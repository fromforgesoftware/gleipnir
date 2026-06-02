package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fromforgesoftware/go-kit/httpclient"
	"github.com/fromforgesoftware/go-kit/ratelimit"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

// ClientSecrets supplies a connector's OAuth app credentials (client id and
// secret) — deployment secrets, not part of the in-code connector catalog.
type ClientSecrets interface {
	ClientCredentials(slug string) (id, secret string, ok bool)
}

// HTTPExchanger performs the OAuth2 refresh_token and authorization_code grants
// against a connector's TokenURL. It is rate-limited per connector (the
// connector's RateProfile) so Gleipnir stays under each provider's published
// API ceiling, and maps a dead grant (invalid_grant) to ErrNeedsReauth.
type HTTPExchanger struct {
	http    *httpclient.Client
	limiter ratelimit.Limiter
	secrets ClientSecrets
	now     func() time.Time
}

type ExchangerOption func(*HTTPExchanger)

// WithExchangerClock overrides the time source (tests).
func WithExchangerClock(now func() time.Time) ExchangerOption {
	return func(e *HTTPExchanger) { e.now = now }
}

func NewHTTPExchanger(client *httpclient.Client, limiter ratelimit.Limiter, secrets ClientSecrets, opts ...ExchangerOption) *HTTPExchanger {
	e := &HTTPExchanger{http: client, limiter: limiter, secrets: secrets, now: time.Now}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (e *HTTPExchanger) Refresh(ctx context.Context, connector domain.Connector, refreshToken string) (Secret, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return e.post(ctx, connector, form)
}

// ExchangeCode performs the OAuth2 authorization_code grant against the
// connector's TokenURL: it trades the provider-issued code for access/refresh
// tokens. redirectURI must be the exact value used at the authorize step (the
// provider rejects a mismatch), and codeVerifier carries the PKCE secret for
// flows that used one (empty otherwise). It shares the connector rate limiter
// and the invalid_grant→ErrNeedsReauth mapping with Refresh.
func (e *HTTPExchanger) ExchangeCode(ctx context.Context, connector domain.Connector, code, redirectURI, codeVerifier string) (Secret, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}
	return e.post(ctx, connector, form)
}

// post is the shared token-endpoint round-trip: it enforces the per-connector
// rate limit, attaches client credentials, POSTs the form, and decodes the
// token response. Neither the request form (refresh token / code / verifier)
// nor the response body is ever logged.
func (e *HTTPExchanger) post(ctx context.Context, connector domain.Connector, form url.Values) (Secret, error) {
	if connector.TokenURL == "" {
		return Secret{}, fmt.Errorf("gleipnir: connector %s has no token URL", connector.Slug)
	}

	res, err := e.limiter.Allow(ctx, "connector:"+connector.Slug, policyFromRate(connector.Rate))
	if err != nil {
		return Secret{}, err
	}
	if !res.Allowed {
		return Secret{}, fmt.Errorf("gleipnir: rate limited for %s, retry after %s", connector.Slug, res.RetryAfter)
	}

	if id, secret, ok := e.secrets.ClientCredentials(connector.Slug); ok {
		form.Set("client_id", id)
		form.Set("client_secret", secret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, connector.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Secret{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := e.http.Do(ctx, req)
	if err != nil {
		return Secret{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		if bytes.Contains(body, []byte("invalid_grant")) {
			return Secret{}, ErrNeedsReauth
		}
		return Secret{}, fmt.Errorf("gleipnir: token exchange failed (%d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return Secret{}, fmt.Errorf("gleipnir: token exchange failed (%d)", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Secret{}, fmt.Errorf("gleipnir: decode token response: %w", err)
	}
	secret := Secret{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken}
	if tr.ExpiresIn > 0 {
		exp := e.now().Add(time.Duration(tr.ExpiresIn) * time.Second)
		secret.ExpiresAt = &exp
	}
	return secret, nil
}

func policyFromRate(r domain.RateProfile) ratelimit.Policy {
	return ratelimit.Policy{Limit: r.Limit, Window: r.Window}
}
