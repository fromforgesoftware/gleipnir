package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	apierrors "github.com/fromforgesoftware/go-kit/errors"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

// stateTTL is how long an authorize "start" stays valid before its state token
// expires. Kept short: the user is expected to complete the provider consent
// immediately, and a short window shrinks the replay/CSRF surface.
const stateTTL = 10 * time.Minute

// CodeExchanger trades a provider authorization code for tokens. It is the
// authorization_code counterpart to TokenExchanger.Refresh and is satisfied by
// the same *HTTPExchanger, so the code path inherits the per-connector rate
// limit and the invalid_grant→ErrNeedsReauth mapping.
type CodeExchanger interface {
	ExchangeCode(ctx context.Context, connector domain.Connector, code, redirectURI, codeVerifier string) (Secret, error)
}

// OAuthStateStore persists in-flight authorization-code transactions and
// consumes them exactly once on callback.
type OAuthStateStore interface {
	Create(ctx context.Context, s domain.OAuthState) (domain.OAuthState, error)
	// Consume atomically claims a usable (unexpired, unconsumed) state by its
	// token, returning NotFound for unknown / expired / already-consumed.
	Consume(ctx context.Context, state string, now time.Time) (domain.OAuthState, error)
}

// OAuthUsecase drives the OAuth2 authorization_code grant: it builds the
// provider authorize URL with a single-use CSRF state (and PKCE challenge where
// supported) on start, and on callback validates the state, exchanges the code,
// and seals the resulting tokens via the same vault path used for any stored
// credential.
type OAuthUsecase struct {
	conns     ConnectionStore
	states    OAuthStateStore
	registry  ConnectorRegistry
	exchanger CodeExchanger
	tokens    *TokenUsecase
	secrets   ClientSecrets
	redirects RedirectAllowlist
	now       func() time.Time
	newToken  func() (string, error)
}

type OAuthOption func(*OAuthUsecase)

// WithOAuthClock overrides the time source (tests).
func WithOAuthClock(now func() time.Time) OAuthOption {
	return func(u *OAuthUsecase) { u.now = now }
}

// WithTokenGenerator overrides the random-token source (tests) used for the
// state token and the PKCE verifier.
func WithTokenGenerator(gen func() (string, error)) OAuthOption {
	return func(u *OAuthUsecase) { u.newToken = gen }
}

func NewOAuthUsecase(
	conns ConnectionStore,
	states OAuthStateStore,
	registry ConnectorRegistry,
	exchanger CodeExchanger,
	tokens *TokenUsecase,
	secrets ClientSecrets,
	redirects RedirectAllowlist,
	opts ...OAuthOption,
) *OAuthUsecase {
	u := &OAuthUsecase{
		conns:     conns,
		states:    states,
		registry:  registry,
		exchanger: exchanger,
		tokens:    tokens,
		secrets:   secrets,
		redirects: redirects,
		now:       time.Now,
		newToken:  randomToken,
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// AuthorizeStart begins the authorization_code flow for a connection. It
// resolves the connection (scoped to owner), validates the connector is OAuth2
// with an authorize URL and that redirectURI is on the allowlist, generates a
// single-use state (and a PKCE verifier/challenge for PKCE connectors),
// persists the transaction, and returns the provider authorize URL to redirect
// the user to.
func (u *OAuthUsecase) AuthorizeStart(ctx context.Context, owner, connectionID, redirectURI string) (string, error) {
	if !u.redirects.Allowed(redirectURI) {
		return "", apierrors.InvalidArgument("redirect_uri is not allowed")
	}

	conn, err := u.conns.Get(ctx, byID(connectionID), byOwner(owner))
	if err != nil {
		return "", err
	}
	connector, ok := u.registry.Lookup(conn.Connector())
	if !ok {
		return "", apierrors.InvalidArgument("unknown connector: " + conn.Connector())
	}
	if connector.AuthType != domain.AuthTypeOAuth2 || connector.AuthURL == "" {
		return "", apierrors.InvalidArgument("connector does not support the authorization-code flow")
	}

	state, err := u.newToken()
	if err != nil {
		return "", apierrors.InternalError("could not generate state")
	}

	q := url.Values{}
	q.Set("response_type", "code")
	if id, _, ok := u.secrets.ClientCredentials(connector.Slug); ok {
		q.Set("client_id", id)
	}
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	if len(connector.Scopes) > 0 {
		q.Set("scope", joinScopes(connector.Scopes))
	}

	var verifier string
	if connector.PKCE {
		verifier, err = u.newToken()
		if err != nil {
			return "", apierrors.InternalError("could not generate code verifier")
		}
		q.Set("code_challenge", pkceChallenge(verifier))
		q.Set("code_challenge_method", "S256")
	}

	opts := []domain.OAuthStateOption{}
	if verifier != "" {
		opts = append(opts, domain.WithOAuthStateCodeVerifier(verifier))
	}
	expires := u.now().Add(stateTTL)
	if _, err := u.states.Create(ctx,
		domain.NewOAuthState(state, conn.ID(), connector.Slug, redirectURI, expires, opts...)); err != nil {
		return "", err
	}

	return buildAuthorizeURL(connector.AuthURL, q)
}

// CallbackResult is the outcome of a completed authorization-code exchange. It
// deliberately carries no token material — only the connection that was
// authorized — so a controller can never leak a secret in its response.
type CallbackResult struct {
	Connection domain.Connection
	Credential domain.Credential
}

// Callback completes the flow: it consumes the state (rejecting unknown /
// expired / replayed), exchanges the code at the provider's token endpoint
// replaying the pinned redirect_uri and PKCE verifier, then seals the resulting
// tokens into the vault as the connection's OAuth credential and marks the
// connection ACTIVE. The provider tokens are never returned to the caller.
func (u *OAuthUsecase) Callback(ctx context.Context, state, code string) (CallbackResult, error) {
	if state == "" || code == "" {
		return CallbackResult{}, apierrors.InvalidArgument("state and code are required")
	}

	st, err := u.states.Consume(ctx, state, u.now())
	if err != nil {
		// Map any miss (unknown / expired / already-used) to a single opaque
		// rejection so the caller learns nothing about why.
		if apierrors.Is(err, apierrors.CodeNotFound) {
			return CallbackResult{}, apierrors.InvalidArgument("invalid or expired state")
		}
		return CallbackResult{}, err
	}

	// Defence in depth: the redirect_uri pinned at start must still be on the
	// allowlist (config could have changed) before we replay it.
	if !u.redirects.Allowed(st.RedirectURI()) {
		return CallbackResult{}, apierrors.InvalidArgument("redirect_uri is not allowed")
	}

	connector, ok := u.registry.Lookup(st.Connector())
	if !ok {
		return CallbackResult{}, apierrors.InvalidArgument("unknown connector: " + st.Connector())
	}

	connID := st.Connection().ID()
	conn, err := u.conns.Get(ctx, byID(connID))
	if err != nil {
		return CallbackResult{}, err
	}

	secret, err := u.exchanger.ExchangeCode(ctx, connector, code, st.RedirectURI(), st.CodeVerifier())
	if err != nil {
		return CallbackResult{}, err
	}

	cred, err := u.tokens.StoreCredential(ctx, connID, domain.CredentialKindOAuthTokens, secret)
	if err != nil {
		return CallbackResult{}, err
	}
	if err := u.conns.SetStatus(ctx, connID, domain.ConnectionStatusActive); err != nil {
		return CallbackResult{}, err
	}

	return CallbackResult{Connection: conn, Credential: cred}, nil
}

// randomToken returns 32 bytes of CSPRNG entropy as a URL-safe, unpadded
// base64 string — used for both the CSRF state token and, doubling as the PKCE
// code_verifier, an RFC 7636-compliant high-entropy verifier (43 chars).
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// pkceChallenge derives the S256 code_challenge from a verifier per RFC 7636:
// BASE64URL(SHA256(verifier)), unpadded.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func joinScopes(scopes []string) string {
	out := ""
	for i, s := range scopes {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

// buildAuthorizeURL appends the query parameters to the connector's authorize
// endpoint, preserving any query the endpoint already carries.
func buildAuthorizeURL(authURL string, q url.Values) (string, error) {
	u, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("gleipnir: invalid authorize URL: %w", err)
	}
	existing := u.Query()
	for k, vs := range q {
		for _, v := range vs {
			existing.Set(k, v)
		}
	}
	u.RawQuery = existing.Encode()
	return u.String(), nil
}
