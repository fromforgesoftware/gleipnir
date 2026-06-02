package domain

import (
	"time"

	"github.com/fromforgesoftware/go-kit/resource"
)

// ResourceTypeOAuthState is the JSON:API type for an in-flight OAuth2
// authorization-code transaction. It is server-internal: never returned with
// its code_verifier, and consumed (single-use) on callback.
const ResourceTypeOAuthState resource.Type = "oauthStates"

// OAuthState is the short-lived, single-use record that ties an authorization
// "start" to its "callback". It carries the CSRF state token, the connection
// being authorized, the exact redirect_uri pinned at start (which the callback
// must match), and — for PKCE flows — the server-side code_verifier that never
// leaves Gleipnir. It expires quickly and is consumed exactly once: a replayed
// or expired state is rejected.
type OAuthState interface {
	resource.Resource
	// State is the opaque CSRF token echoed by the provider on callback.
	State() string
	Connection() resource.Identifier
	// Connector is the connector slug, captured at start so the callback need
	// not re-derive it.
	Connector() string
	// RedirectURI is the exact redirect_uri sent to the provider at start; the
	// token exchange must replay the identical value (OAuth2 requires it).
	RedirectURI() string
	// CodeVerifier is the PKCE verifier (RFC 7636), empty for non-PKCE flows.
	// It is held server-side and never exposed over the API.
	CodeVerifier() string
	ExpiresAt() time.Time
	// ConsumedAt is set when the state is spent; a non-nil value means a second
	// use must be rejected as a replay.
	ConsumedAt() *time.Time
}

type oauthState struct {
	resource.Resource

	state        string
	connectionID string
	connector    string
	redirectURI  string
	codeVerifier string
	expiresAt    time.Time
	consumedAt   *time.Time
}

type OAuthStateOption func(*oauthState)

func WithOAuthStateID(id string) OAuthStateOption {
	return func(s *oauthState) { s.Resource = resource.Update(s.Resource, resource.WithID(id)) }
}
func WithOAuthStateCodeVerifier(v string) OAuthStateOption {
	return func(s *oauthState) { s.codeVerifier = v }
}
func WithOAuthStateConsumedAt(t *time.Time) OAuthStateOption {
	return func(s *oauthState) { s.consumedAt = t }
}

// NewOAuthState builds an in-flight authorization-code transaction. state,
// connectionID, connector, redirectURI and expiry are mandatory; the PKCE
// verifier is optional.
func NewOAuthState(state, connectionID, connector, redirectURI string, expiresAt time.Time, opts ...OAuthStateOption) OAuthState {
	s := &oauthState{
		Resource:     resource.New(resource.WithType(ResourceTypeOAuthState)),
		state:        state,
		connectionID: connectionID,
		connector:    connector,
		redirectURI:  redirectURI,
		expiresAt:    expiresAt,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *oauthState) State() string { return s.state }
func (s *oauthState) Connection() resource.Identifier {
	return resource.NewIdentifier(s.connectionID, ResourceTypeConnection)
}
func (s *oauthState) Connector() string      { return s.connector }
func (s *oauthState) RedirectURI() string    { return s.redirectURI }
func (s *oauthState) CodeVerifier() string   { return s.codeVerifier }
func (s *oauthState) ExpiresAt() time.Time   { return s.expiresAt }
func (s *oauthState) ConsumedAt() *time.Time { return s.consumedAt }
