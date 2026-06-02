// Package app holds Gleipnir's usecases. The token lifecycle is the core: it
// seals provider secrets into the vault, vends them (refreshing on read when
// near expiry), and refreshes ahead of expiry from a cron, flipping a
// connection to NEEDS_REAUTH and notifying when its refresh token dies.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/fromforgesoftware/go-kit/filter"
	"github.com/fromforgesoftware/go-kit/search"
	"github.com/fromforgesoftware/go-kit/search/query"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/fields"
)

// ErrNeedsReauth means a connection's refresh token is dead; the owner must
// re-authorize the connector. The connection is flipped to NEEDS_REAUTH.
var ErrNeedsReauth = errors.New("gleipnir: connection needs re-authorization")

// ErrRevoked means the connection was revoked and can no longer vend secrets.
var ErrRevoked = errors.New("gleipnir: connection revoked")

// Secret is the plaintext credential material sealed in the vault. Only the
// fields relevant to a connector's AuthType are populated.
type Secret struct {
	AccessToken  string     `json:"accessToken,omitempty"`
	RefreshToken string     `json:"refreshToken,omitempty"`
	APIKey       string     `json:"apiKey,omitempty"`
	APISecret    string     `json:"apiSecret,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
}

// ConnectionStore reads connections and flips their lifecycle status.
type ConnectionStore interface {
	Get(ctx context.Context, opts ...search.Option) (domain.Connection, error)
	SetStatus(ctx context.Context, id string, status domain.ConnectionStatus) error
}

// CredentialStore persists sealed credentials and finds those due for refresh.
type CredentialStore interface {
	Get(ctx context.Context, opts ...search.Option) (domain.Credential, error)
	Create(ctx context.Context, c domain.Credential) (domain.Credential, error)
	Replace(ctx context.Context, id string, ciphertext, wrappedKey []byte, keyID string, expiresAt *time.Time) error
	ListDueForRefresh(ctx context.Context, before time.Time, limit int) ([]domain.Credential, error)
}

// Sealer is the envelope vault; implemented by *vault.Vault.
type Sealer interface {
	Seal(ctx context.Context, plaintext []byte) (ciphertext, wrappedKey []byte, err error)
	Open(ctx context.Context, ciphertext, wrappedKey []byte) ([]byte, error)
	KeyID() string
}

// ConnectorRegistry resolves connector definitions; Lookup serves the token
// lifecycle, List serves the read-only /api/connectors catalog.
type ConnectorRegistry interface {
	Lookup(slug string) (domain.Connector, bool)
	List() []domain.Connector
}

// TokenExchanger performs the provider-side OAuth refresh. It returns
// ErrNeedsReauth when the refresh token itself is rejected.
type TokenExchanger interface {
	Refresh(ctx context.Context, connector domain.Connector, refreshToken string) (Secret, error)
}

// ReauthNotifier is told when a connection needs re-authorization (→ Herald).
type ReauthNotifier interface {
	NeedsReauth(ctx context.Context, conn domain.Connection) error
}

// TokenUsecase orchestrates the token lifecycle.
type TokenUsecase struct {
	conns       ConnectionStore
	creds       CredentialStore
	vault       Sealer
	registry    ConnectorRegistry
	exchanger   TokenExchanger
	notifier    ReauthNotifier
	now         func() time.Time
	refreshLead time.Duration
}

type TokenOption func(*TokenUsecase)

// WithClock overrides the time source (tests).
func WithClock(now func() time.Time) TokenOption {
	return func(u *TokenUsecase) { u.now = now }
}

// WithRefreshLead sets how far before expiry a token is eagerly refreshed.
func WithRefreshLead(d time.Duration) TokenOption {
	return func(u *TokenUsecase) { u.refreshLead = d }
}

func NewTokenUsecase(
	conns ConnectionStore,
	creds CredentialStore,
	vault Sealer,
	registry ConnectorRegistry,
	exchanger TokenExchanger,
	notifier ReauthNotifier,
	opts ...TokenOption,
) *TokenUsecase {
	u := &TokenUsecase{
		conns:       conns,
		creds:       creds,
		vault:       vault,
		registry:    registry,
		exchanger:   exchanger,
		notifier:    notifier,
		now:         time.Now,
		refreshLead: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// StoreCredential seals a secret and persists it as the connection's credential.
func (u *TokenUsecase) StoreCredential(ctx context.Context, connectionID string, kind domain.CredentialKind, secret Secret) (domain.Credential, error) {
	ct, wk, err := u.seal(ctx, secret)
	if err != nil {
		return nil, err
	}
	opts := []domain.CredentialOption{domain.WithCredentialKeyID(u.vault.KeyID())}
	if secret.ExpiresAt != nil {
		opts = append(opts, domain.WithCredentialExpiresAt(secret.ExpiresAt))
	}
	return u.creds.Create(ctx, domain.NewCredential(connectionID, kind, ct, wk, opts...))
}

// Vend returns the connection's current secret, refreshing on read if it is
// within the refresh lead of expiry. This is the hot path; a warm credential
// returns after a single decrypt with no provider round-trip. The owner must
// match the connection's owner — a foreign owner gets a not-found, never a
// secret.
func (u *TokenUsecase) Vend(ctx context.Context, owner, connectionID string) (Secret, error) {
	conn, err := u.conns.Get(ctx, byID(connectionID), byOwner(owner))
	if err != nil {
		return Secret{}, err
	}
	if conn.Status() == domain.ConnectionStatusRevoked {
		return Secret{}, ErrRevoked
	}
	cred, err := u.creds.Get(ctx, byConnection(connectionID))
	if err != nil {
		return Secret{}, err
	}
	secret, err := u.open(ctx, cred)
	if err != nil {
		return Secret{}, err
	}
	if u.dueForRefresh(cred) {
		return u.refresh(ctx, conn, cred, secret)
	}
	return secret, nil
}

// RefreshDue refreshes up to limit credentials that fall within the refresh
// lead, skipping non-ACTIVE connections. Intended to be driven by a cron so
// the vend hot path almost always finds a warm token. Returns the count
// refreshed.
func (u *TokenUsecase) RefreshDue(ctx context.Context, limit int) (int, error) {
	before := u.now().Add(u.refreshLead)
	due, err := u.creds.ListDueForRefresh(ctx, before, limit)
	if err != nil {
		return 0, err
	}
	refreshed := 0
	for _, cred := range due {
		conn, err := u.conns.Get(ctx, byID(cred.Connection().ID()))
		if err != nil || conn.Status() != domain.ConnectionStatusActive {
			continue
		}
		current, err := u.open(ctx, cred)
		if err != nil {
			continue
		}
		if _, err := u.refresh(ctx, conn, cred, current); err == nil {
			refreshed++
		}
	}
	return refreshed, nil
}

func (u *TokenUsecase) refresh(ctx context.Context, conn domain.Connection, cred domain.Credential, current Secret) (Secret, error) {
	connector, ok := u.registry.Lookup(conn.Connector())
	if !ok {
		return Secret{}, errors.New("gleipnir: unknown connector " + conn.Connector())
	}
	fresh, err := u.exchanger.Refresh(ctx, connector, current.RefreshToken)
	if errors.Is(err, ErrNeedsReauth) {
		_ = u.conns.SetStatus(ctx, conn.ID(), domain.ConnectionStatusNeedsReauth)
		_ = u.notifier.NeedsReauth(ctx, conn)
		return Secret{}, ErrNeedsReauth
	}
	if err != nil {
		return Secret{}, err
	}
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = current.RefreshToken
	}
	ct, wk, err := u.seal(ctx, fresh)
	if err != nil {
		return Secret{}, err
	}
	if err := u.creds.Replace(ctx, cred.ID(), ct, wk, u.vault.KeyID(), fresh.ExpiresAt); err != nil {
		return Secret{}, err
	}
	return fresh, nil
}

func (u *TokenUsecase) dueForRefresh(cred domain.Credential) bool {
	if cred.Kind() != domain.CredentialKindOAuthTokens || cred.ExpiresAt() == nil {
		return false
	}
	return !cred.ExpiresAt().After(u.now().Add(u.refreshLead))
}

func (u *TokenUsecase) seal(ctx context.Context, s Secret) (ciphertext, wrappedKey []byte, err error) {
	pt, err := json.Marshal(s)
	if err != nil {
		return nil, nil, err
	}
	return u.vault.Seal(ctx, pt)
}

func (u *TokenUsecase) open(ctx context.Context, cred domain.Credential) (Secret, error) {
	pt, err := u.vault.Open(ctx, cred.Ciphertext(), cred.WrappedKey())
	if err != nil {
		return Secret{}, err
	}
	var s Secret
	if err := json.Unmarshal(pt, &s); err != nil {
		return Secret{}, err
	}
	return s, nil
}

func byID(id string) search.Option {
	return search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.ID, id))
}

func byConnection(id string) search.Option {
	return search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.ConnectionID, id))
}

func byOwner(owner string) search.Option {
	return search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.Owner, owner))
}
