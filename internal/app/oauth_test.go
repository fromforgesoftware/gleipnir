package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apierrors "github.com/fromforgesoftware/go-kit/errors"

	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

const (
	allowedRedirect = "https://app.example.com/oauth/callback"
	deniedRedirect  = "https://evil.example.com/cb"
)

// --- fakes ---

type fakeStates struct {
	created   domain.OAuthState
	stored    domain.OAuthState // what Consume returns when found
	consume   func(state string, now time.Time) (domain.OAuthState, error)
	createErr error
}

func (f *fakeStates) Create(_ context.Context, s domain.OAuthState) (domain.OAuthState, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = s
	return s, nil
}

func (f *fakeStates) Consume(_ context.Context, state string, now time.Time) (domain.OAuthState, error) {
	if f.consume != nil {
		return f.consume(state, now)
	}
	return f.stored, nil
}

type fakeCodeExchanger struct {
	secret app.Secret
	err    error
	calls  int
	// captured
	code, redirectURI, verifier string
}

func (f *fakeCodeExchanger) ExchangeCode(_ context.Context, _ domain.Connector, code, redirectURI, verifier string) (app.Secret, error) {
	f.calls++
	f.code, f.redirectURI, f.verifier = code, redirectURI, verifier
	return f.secret, f.err
}

type fakeSecrets struct{ id, secret string }

func (f fakeSecrets) ClientCredentials(string) (string, string, bool) {
	if f.id == "" && f.secret == "" {
		return "", "", false
	}
	return f.id, f.secret, true
}

func oauthConnector(pkce bool) domain.Connector {
	return domain.Connector{
		Slug:     "coinbase",
		Name:     "Coinbase",
		AuthType: domain.AuthTypeOAuth2,
		AuthURL:  "https://login.coinbase.com/oauth2/auth",
		TokenURL: "https://login.coinbase.com/oauth2/token",
		Scopes:   []string{"wallet:accounts:read", "wallet:trades:create"},
		PKCE:     pkce,
		Rate:     domain.RateProfile{Limit: 100, Window: time.Minute},
	}
}

func newOAuth(t *testing.T, conns *memConns, states app.OAuthStateStore, reg app.ConnectorRegistry, ex app.CodeExchanger, opts ...app.OAuthOption) *app.OAuthUsecase {
	t.Helper()
	v := testVault(t)
	creds := &memCreds{connID: "c1", v: v}
	tokens := app.NewTokenUsecase(conns, creds, v, reg, &fakeExchanger{}, &fakeNotifier{},
		app.WithClock(func() time.Time { return fixedNow }))
	allow := app.NewRedirectAllowlist(allowedRedirect)
	base := []app.OAuthOption{app.WithOAuthClock(func() time.Time { return fixedNow })}
	return app.NewOAuthUsecase(conns, states, reg, ex, tokens, fakeSecrets{id: "cid"}, allow, append(base, opts...)...)
}

// --- AuthorizeStart ---

func TestAuthorizeStart_BuildsURLWithStateAndPKCE(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "coinbase", domain.WithConnectionID("c1"))}
	states := &fakeStates{}
	reg := fakeRegistry{connector: oauthConnector(true), ok: true}
	u := newOAuth(t, conns, states, reg, &fakeCodeExchanger{},
		app.WithTokenGenerator(func() (string, error) { return "fixed-verifier-or-state", nil }))

	raw, err := u.AuthorizeStart(context.Background(), testOwner, "c1", allowedRedirect)
	require.NoError(t, err)

	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "login.coinbase.com", parsed.Host)
	q := parsed.Query()
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "cid", q.Get("client_id"))
	assert.Equal(t, allowedRedirect, q.Get("redirect_uri"))
	assert.Equal(t, "fixed-verifier-or-state", q.Get("state"))
	assert.Equal(t, "wallet:accounts:read wallet:trades:create", q.Get("scope"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))

	// challenge = BASE64URL(SHA256(verifier))
	sum := sha256.Sum256([]byte("fixed-verifier-or-state"))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), q.Get("code_challenge"))

	// State persisted with the verifier server-side and a future expiry.
	require.NotNil(t, states.created)
	assert.Equal(t, "fixed-verifier-or-state", states.created.State())
	assert.Equal(t, "fixed-verifier-or-state", states.created.CodeVerifier())
	assert.Equal(t, allowedRedirect, states.created.RedirectURI())
	assert.Equal(t, "coinbase", states.created.Connector())
	assert.True(t, states.created.ExpiresAt().After(fixedNow))
}

func TestAuthorizeStart_NoPKCEOmitsChallenge(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "alpaca", domain.WithConnectionID("c1"))}
	states := &fakeStates{}
	reg := fakeRegistry{connector: domain.Connector{Slug: "alpaca", AuthType: domain.AuthTypeOAuth2,
		AuthURL: "https://app.alpaca.markets/oauth/authorize", PKCE: false}, ok: true}
	u := newOAuth(t, conns, states, reg, &fakeCodeExchanger{})

	raw, err := u.AuthorizeStart(context.Background(), testOwner, "c1", allowedRedirect)
	require.NoError(t, err)
	q := mustQuery(t, raw)
	assert.Empty(t, q.Get("code_challenge"))
	assert.Empty(t, states.created.CodeVerifier(), "non-PKCE flow stores no verifier")
}

func TestAuthorizeStart_DeniesRedirectNotOnAllowlist(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "coinbase", domain.WithConnectionID("c1"))}
	states := &fakeStates{}
	reg := fakeRegistry{connector: oauthConnector(true), ok: true}
	u := newOAuth(t, conns, states, reg, &fakeCodeExchanger{})

	_, err := u.AuthorizeStart(context.Background(), testOwner, "c1", deniedRedirect)
	require.Error(t, err)
	assert.True(t, apierrors.Is(err, apierrors.CodeInvalidArgument))
	assert.Nil(t, states.created, "no state persisted when redirect_uri is denied")
}

func TestAuthorizeStart_RejectsNonOAuthConnector(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "binance", domain.WithConnectionID("c1"))}
	states := &fakeStates{}
	reg := fakeRegistry{connector: domain.Connector{Slug: "binance", AuthType: domain.AuthTypeAPIKey}, ok: true}
	u := newOAuth(t, conns, states, reg, &fakeCodeExchanger{})

	_, err := u.AuthorizeStart(context.Background(), testOwner, "c1", allowedRedirect)
	require.Error(t, err)
	assert.True(t, apierrors.Is(err, apierrors.CodeInvalidArgument))
}

// --- Callback ---

func usableState() domain.OAuthState {
	return domain.NewOAuthState("st", "c1", "coinbase", allowedRedirect, fixedNow.Add(time.Minute),
		domain.WithOAuthStateCodeVerifier("verifier-x"))
}

func TestCallback_SuccessSealsTokensAndActivates(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "coinbase", domain.WithConnectionID("c1"))}
	states := &fakeStates{stored: usableState()}
	reg := fakeRegistry{connector: oauthConnector(true), ok: true}
	exp := fixedNow.Add(time.Hour)
	ex := &fakeCodeExchanger{secret: app.Secret{AccessToken: "AT", RefreshToken: "RT", ExpiresAt: &exp}}
	u := newOAuth(t, conns, states, reg, ex)

	res, err := u.Callback(context.Background(), "st", "the-code")
	require.NoError(t, err)
	assert.Equal(t, 1, ex.calls)
	assert.Equal(t, "the-code", ex.code)
	assert.Equal(t, allowedRedirect, ex.redirectURI, "callback must replay the pinned redirect_uri")
	assert.Equal(t, "verifier-x", ex.verifier, "callback must replay the PKCE verifier")
	require.NotNil(t, res.Credential)
	assert.Equal(t, domain.CredentialKindOAuthTokens, res.Credential.Kind())
	assert.True(t, conns.statusCalled)
	assert.Equal(t, domain.ConnectionStatusActive, conns.statusSet)
}

func TestCallback_RejectsUnknownExpiredOrReplayedState(t *testing.T) {
	cases := map[string]error{
		"unknown":  apierrors.NotFound("oauthState", ""),
		"expired":  apierrors.NotFound("oauthState", ""),
		"replayed": apierrors.NotFound("oauthState", ""),
	}
	for name, consumeErr := range cases {
		t.Run(name, func(t *testing.T) {
			conns := &memConns{conn: domain.NewConnection(testOwner, "coinbase", domain.WithConnectionID("c1"))}
			states := &fakeStates{consume: func(string, time.Time) (domain.OAuthState, error) {
				return nil, consumeErr
			}}
			reg := fakeRegistry{connector: oauthConnector(true), ok: true}
			ex := &fakeCodeExchanger{}
			u := newOAuth(t, conns, states, reg, ex)

			_, err := u.Callback(context.Background(), "st", "code")
			require.Error(t, err)
			assert.True(t, apierrors.Is(err, apierrors.CodeInvalidArgument), "miss maps to opaque invalid argument")
			assert.Zero(t, ex.calls, "no code exchange when state is rejected")
			assert.False(t, conns.statusCalled)
		})
	}
}

func TestCallback_InvalidGrantPropagates(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "coinbase", domain.WithConnectionID("c1"))}
	states := &fakeStates{stored: usableState()}
	reg := fakeRegistry{connector: oauthConnector(true), ok: true}
	ex := &fakeCodeExchanger{err: app.ErrNeedsReauth}
	u := newOAuth(t, conns, states, reg, ex)

	_, err := u.Callback(context.Background(), "st", "bad-code")
	assert.ErrorIs(t, err, app.ErrNeedsReauth)
	assert.False(t, conns.statusCalled, "no activation on a failed exchange")
}

func TestCallback_DeniesStateWithDisallowedRedirect(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "coinbase", domain.WithConnectionID("c1"))}
	// State pins a redirect that is no longer on the allowlist.
	states := &fakeStates{stored: domain.NewOAuthState("st", "c1", "coinbase", deniedRedirect, fixedNow.Add(time.Minute))}
	reg := fakeRegistry{connector: oauthConnector(false), ok: true}
	ex := &fakeCodeExchanger{}
	u := newOAuth(t, conns, states, reg, ex)

	_, err := u.Callback(context.Background(), "st", "code")
	require.Error(t, err)
	assert.True(t, apierrors.Is(err, apierrors.CodeInvalidArgument))
	assert.Zero(t, ex.calls)
}

func TestCallback_RequiresStateAndCode(t *testing.T) {
	conns := &memConns{conn: domain.NewConnection(testOwner, "coinbase", domain.WithConnectionID("c1"))}
	u := newOAuth(t, conns, &fakeStates{}, fakeRegistry{connector: oauthConnector(true), ok: true}, &fakeCodeExchanger{})

	_, err := u.Callback(context.Background(), "", "code")
	assert.True(t, apierrors.Is(err, apierrors.CodeInvalidArgument))
	_, err = u.Callback(context.Background(), "st", "")
	assert.True(t, apierrors.Is(err, apierrors.CodeInvalidArgument))
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	p, err := url.Parse(raw)
	require.NoError(t, err)
	return p.Query()
}
