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

// httpExchanger performs the OAuth2 refresh_token grant against a connector's
// TokenURL. It is rate-limited per connector (the connector's RateProfile) so
// Gleipnir stays under each provider's published API ceiling, and maps a dead
// refresh token (invalid_grant) to ErrNeedsReauth.
type httpExchanger struct {
	http    *httpclient.Client
	limiter ratelimit.Limiter
	secrets ClientSecrets
	now     func() time.Time
}

type ExchangerOption func(*httpExchanger)

// WithExchangerClock overrides the time source (tests).
func WithExchangerClock(now func() time.Time) ExchangerOption {
	return func(e *httpExchanger) { e.now = now }
}

func NewHTTPExchanger(client *httpclient.Client, limiter ratelimit.Limiter, secrets ClientSecrets, opts ...ExchangerOption) *httpExchanger {
	e := &httpExchanger{http: client, limiter: limiter, secrets: secrets, now: time.Now}
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

func (e *httpExchanger) Refresh(ctx context.Context, connector domain.Connector, refreshToken string) (Secret, error) {
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

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
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
		return Secret{}, fmt.Errorf("gleipnir: token refresh failed (%d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return Secret{}, fmt.Errorf("gleipnir: token refresh failed (%d)", resp.StatusCode)
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
