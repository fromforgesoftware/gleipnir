// Package domain holds Gleipnir's aggregates: Connector (a provider
// definition), Connection (an org's authorized instance of one), and
// Credential (its encrypted secret material).
package domain

import (
	"time"

	"github.com/fromforgesoftware/go-kit/resource"
)

// ResourceTypeConnector is the JSON:API type for /api/connectors.
const ResourceTypeConnector resource.Type = "connectors"

// AuthType is how a connector authenticates to its provider.
type AuthType string

const (
	AuthTypeOAuth2 AuthType = "OAUTH2"
	AuthTypeAPIKey AuthType = "API_KEY"
)

func (a AuthType) Valid() bool {
	switch a {
	case AuthTypeOAuth2, AuthTypeAPIKey:
		return true
	}
	return false
}

// RateProfile is a connector's default per-provider throttle, enforced via
// kit/ratelimit so all orgs' calls stay under the provider's API limit.
type RateProfile struct {
	Limit  int
	Window time.Duration
}

// Connector is a global provider definition (Alpaca, Binance, IBKR, …),
// registered in code via kit/factory and surfaced read-only at /api/connectors.
// Its Slug is the identifier; it is not owner-scoped or persisted. It satisfies
// resource.Resource (id = slug) so it flows through the kit JSON:API handlers.
type Connector struct {
	Slug     string
	Name     string
	AuthType AuthType
	// AuthURL/TokenURL drive the OAuth2 authorize + token/refresh exchange
	// (empty for API_KEY connectors).
	AuthURL  string
	TokenURL string
	Scopes   []string
	// PKCE marks providers that support (or require) RFC 7636 Proof Key for
	// Code Exchange. When true the authorize step generates a code_verifier /
	// code_challenge and the token exchange replays the verifier — mandatory
	// for public/SPA-style clients that cannot keep a client secret.
	PKCE bool
	Rate RateProfile
}

func (c Connector) ID() string            { return c.Slug }
func (c Connector) LID() string           { return "" }
func (c Connector) Type() resource.Type   { return ResourceTypeConnector }
func (c Connector) CreatedAt() time.Time  { return time.Time{} }
func (c Connector) UpdatedAt() time.Time  { return time.Time{} }
func (c Connector) DeletedAt() *time.Time { return nil }
