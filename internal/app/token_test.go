package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fromforgesoftware/go-kit/search"
	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/vault"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func testVault(t *testing.T) *vault.Vault {
	t.Helper()
	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = byte(i)
	}
	p, err := vault.NewLocalKeyProvider(kek)
	require.NoError(t, err)
	return vault.New(p)
}

func sealedCredential(t *testing.T, v *vault.Vault, connID string, kind domain.CredentialKind, s app.Secret, exp *time.Time) domain.Credential {
	t.Helper()
	pt, err := json.Marshal(s)
	require.NoError(t, err)
	ct, wk, err := v.Seal(context.Background(), pt)
	require.NoError(t, err)
	opts := []domain.CredentialOption{domain.WithCredentialID("cred-1"), domain.WithCredentialKeyID(v.KeyID())}
	if exp != nil {
		opts = append(opts, domain.WithCredentialExpiresAt(exp))
	}
	return domain.NewCredential(connID, kind, ct, wk, opts...)
}

// --- fakes ---

type memConns struct {
	conn         domain.Connection
	statusSet    domain.ConnectionStatus
	statusCalled bool
}

func (m *memConns) Get(context.Context, ...search.Option) (domain.Connection, error) {
	return m.conn, nil
}
func (m *memConns) SetStatus(_ context.Context, _ string, s domain.ConnectionStatus) error {
	m.statusSet, m.statusCalled = s, true
	return nil
}

type memCreds struct {
	cred         domain.Credential
	due          []domain.Credential
	replaced     bool
	replacedCT   []byte
	replacedWK   []byte
	v            *vault.Vault
	connID       string
	replacedKind domain.CredentialKind
}

func (m *memCreds) Get(context.Context, ...search.Option) (domain.Credential, error) {
	return m.cred, nil
}
func (m *memCreds) Create(_ context.Context, c domain.Credential) (domain.Credential, error) {
	m.cred = c
	return c, nil
}
func (m *memCreds) Replace(_ context.Context, id string, ct, wk []byte, keyID string, exp *time.Time) error {
	m.replaced, m.replacedCT, m.replacedWK = true, ct, wk
	opts := []domain.CredentialOption{domain.WithCredentialID(id), domain.WithCredentialKeyID(keyID)}
	if exp != nil {
		opts = append(opts, domain.WithCredentialExpiresAt(exp))
	}
	kind := m.replacedKind
	if kind == "" {
		kind = domain.CredentialKindOAuthTokens
	}
	m.cred = domain.NewCredential(m.connID, kind, ct, wk, opts...)
	return nil
}
func (m *memCreds) ListDueForRefresh(context.Context, time.Time, int) ([]domain.Credential, error) {
	return m.due, nil
}

type fakeRegistry struct {
	connector domain.Connector
	ok        bool
}

func (f fakeRegistry) Lookup(string) (domain.Connector, bool) { return f.connector, f.ok }
func (f fakeRegistry) List() []domain.Connector {
	if !f.ok {
		return nil
	}
	return []domain.Connector{f.connector}
}

type fakeExchanger struct {
	fresh app.Secret
	err   error
	calls int
}

func (f *fakeExchanger) Refresh(context.Context, domain.Connector, string) (app.Secret, error) {
	f.calls++
	return f.fresh, f.err
}

type fakeNotifier struct{ called bool }

func (f *fakeNotifier) NeedsReauth(context.Context, domain.Connection) error {
	f.called = true
	return nil
}

const testOwner = "owner-1"

func conn(id string, status domain.ConnectionStatus) domain.Connection {
	return domain.NewConnection(testOwner, "binance",
		domain.WithConnectionID(id), domain.WithConnectionStatus(status))
}

// --- tests ---

func TestVend_WarmTokenNoRefresh(t *testing.T) {
	v := testVault(t)
	exp := fixedNow.Add(time.Hour)
	creds := &memCreds{connID: "c1", v: v,
		cred: sealedCredential(t, v, "c1", domain.CredentialKindOAuthTokens,
			app.Secret{AccessToken: "AT", RefreshToken: "RT"}, &exp)}
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusActive)}
	ex := &fakeExchanger{}
	u := app.NewTokenUsecase(conns, creds, v, fakeRegistry{ok: true}, ex, &fakeNotifier{},
		app.WithClock(func() time.Time { return fixedNow }), app.WithRefreshLead(5*time.Minute))

	got, err := u.Vend(context.Background(), testOwner, "c1")
	require.NoError(t, err)
	assert.Equal(t, "AT", got.AccessToken)
	assert.Zero(t, ex.calls, "warm token must not hit the provider")
}

func TestVend_RefreshOnReadWhenDue(t *testing.T) {
	v := testVault(t)
	exp := fixedNow.Add(2 * time.Minute) // within 5m lead → due
	creds := &memCreds{connID: "c1", v: v,
		cred: sealedCredential(t, v, "c1", domain.CredentialKindOAuthTokens,
			app.Secret{AccessToken: "old", RefreshToken: "RT"}, &exp)}
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusActive)}
	newExp := fixedNow.Add(time.Hour)
	ex := &fakeExchanger{fresh: app.Secret{AccessToken: "new", RefreshToken: "RT2", ExpiresAt: &newExp}}
	u := newUsecaseWith(t, conns, creds, v, ex, &fakeNotifier{})

	got, err := u.Vend(context.Background(), testOwner, "c1")
	require.NoError(t, err)
	assert.Equal(t, "new", got.AccessToken)
	assert.Equal(t, 1, ex.calls)
	assert.True(t, creds.replaced, "fresh secret must be re-sealed")
}

func TestVend_CarriesRefreshTokenWhenProviderOmitsIt(t *testing.T) {
	v := testVault(t)
	exp := fixedNow.Add(time.Minute)
	creds := &memCreds{connID: "c1", v: v,
		cred: sealedCredential(t, v, "c1", domain.CredentialKindOAuthTokens,
			app.Secret{AccessToken: "old", RefreshToken: "keep-me"}, &exp)}
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusActive)}
	newExp := fixedNow.Add(time.Hour)
	ex := &fakeExchanger{fresh: app.Secret{AccessToken: "new", ExpiresAt: &newExp}} // no refresh token
	u := newUsecaseWith(t, conns, creds, v, ex, &fakeNotifier{})

	got, err := u.Vend(context.Background(), testOwner, "c1")
	require.NoError(t, err)
	assert.Equal(t, "keep-me", got.RefreshToken)
}

func TestVend_APIKeyNeverRefreshes(t *testing.T) {
	v := testVault(t)
	creds := &memCreds{connID: "c1", v: v,
		cred: sealedCredential(t, v, "c1", domain.CredentialKindAPIKey,
			app.Secret{APIKey: "K", APISecret: "S"}, nil)}
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusActive)}
	ex := &fakeExchanger{}
	u := newUsecaseWith(t, conns, creds, v, ex, &fakeNotifier{})

	got, err := u.Vend(context.Background(), testOwner, "c1")
	require.NoError(t, err)
	assert.Equal(t, "K", got.APIKey)
	assert.Equal(t, "S", got.APISecret)
	assert.Zero(t, ex.calls)
}

func TestVend_RevokedRejected(t *testing.T) {
	v := testVault(t)
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusRevoked)}
	creds := &memCreds{connID: "c1", v: v}
	u := newUsecaseWith(t, conns, creds, v, &fakeExchanger{}, &fakeNotifier{})

	_, err := u.Vend(context.Background(), testOwner, "c1")
	assert.ErrorIs(t, err, app.ErrRevoked)
}

func TestVend_DeadRefreshTokenFlipsNeedsReauth(t *testing.T) {
	v := testVault(t)
	exp := fixedNow.Add(time.Minute)
	creds := &memCreds{connID: "c1", v: v,
		cred: sealedCredential(t, v, "c1", domain.CredentialKindOAuthTokens,
			app.Secret{AccessToken: "old", RefreshToken: "dead"}, &exp)}
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusActive)}
	no := &fakeNotifier{}
	ex := &fakeExchanger{err: app.ErrNeedsReauth}
	u := newUsecaseWith(t, conns, creds, v, ex, no)

	_, err := u.Vend(context.Background(), testOwner, "c1")
	assert.ErrorIs(t, err, app.ErrNeedsReauth)
	assert.True(t, conns.statusCalled)
	assert.Equal(t, domain.ConnectionStatusNeedsReauth, conns.statusSet)
	assert.True(t, no.called, "owner must be notified")
}

func TestRefreshDue_RefreshesActiveSkipsNonActive(t *testing.T) {
	v := testVault(t)
	exp := fixedNow.Add(time.Minute)
	dueCred := sealedCredential(t, v, "c1", domain.CredentialKindOAuthTokens,
		app.Secret{AccessToken: "old", RefreshToken: "RT"}, &exp)
	creds := &memCreds{connID: "c1", v: v, cred: dueCred, due: []domain.Credential{dueCred}}
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusActive)}
	newExp := fixedNow.Add(time.Hour)
	ex := &fakeExchanger{fresh: app.Secret{AccessToken: "new", RefreshToken: "RT2", ExpiresAt: &newExp}}
	u := newUsecaseWith(t, conns, creds, v, ex, &fakeNotifier{})

	n, err := u.RefreshDue(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, ex.calls)

	// non-active connection is skipped
	conns.conn = conn("c1", domain.ConnectionStatusNeedsReauth)
	ex.calls = 0
	n, err = u.RefreshDue(context.Background(), 10)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Zero(t, ex.calls)
}

func TestStoreCredentialThenVend_RoundTrip(t *testing.T) {
	v := testVault(t)
	creds := &memCreds{connID: "c1", v: v}
	conns := &memConns{conn: conn("c1", domain.ConnectionStatusActive)}
	u := newUsecaseWith(t, conns, creds, v, &fakeExchanger{}, &fakeNotifier{})

	exp := fixedNow.Add(time.Hour)
	_, err := u.StoreCredential(context.Background(), "c1", domain.CredentialKindOAuthTokens,
		app.Secret{AccessToken: "stored", RefreshToken: "RT", ExpiresAt: &exp})
	require.NoError(t, err)

	got, err := u.Vend(context.Background(), testOwner, "c1")
	require.NoError(t, err)
	assert.Equal(t, "stored", got.AccessToken)
}

func newUsecaseWith(t *testing.T, conns *memConns, creds *memCreds, v *vault.Vault, ex *fakeExchanger, no *fakeNotifier) *app.TokenUsecase {
	t.Helper()
	reg := fakeRegistry{connector: domain.Connector{Slug: "binance", AuthType: domain.AuthTypeOAuth2}, ok: true}
	return app.NewTokenUsecase(conns, creds, v, reg, ex, no,
		app.WithClock(func() time.Time { return fixedNow }), app.WithRefreshLead(5*time.Minute))
}
