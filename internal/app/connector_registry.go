package app

import (
	"context"
	"time"

	"github.com/fromforgesoftware/go-kit/factory"
	"github.com/fromforgesoftware/go-kit/resource"
	"github.com/fromforgesoftware/go-kit/search"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

// connectorRegistry is an immutable, lock-free connector catalog backed by
// kit/factory. It is frozen at construction so Lookup on the vend hot path is
// allocation-free.
type connectorRegistry struct {
	reg *factory.Registry[domain.Connector]
}

// NewConnectorRegistry builds a frozen registry from the given connectors.
func NewConnectorRegistry(connectors ...domain.Connector) ConnectorRegistry {
	reg := factory.New[domain.Connector]()
	for _, c := range connectors {
		reg.MustRegister(c.Slug, c)
	}
	reg.Freeze()
	return &connectorRegistry{reg: reg}
}

// NewDefaultConnectorRegistry builds the registry from the built-in catalog.
func NewDefaultConnectorRegistry() ConnectorRegistry {
	return NewConnectorRegistry(DefaultConnectors()...)
}

func (r *connectorRegistry) Lookup(slug string) (domain.Connector, bool) {
	return r.reg.Get(slug)
}

func (r *connectorRegistry) List() []domain.Connector {
	keys := r.reg.Keys()
	out := make([]domain.Connector, 0, len(keys))
	for _, k := range keys {
		if c, ok := r.reg.Get(k); ok {
			out = append(out, c)
		}
	}
	return out
}

// ConnectorCatalog adapts the registry to a kit Lister so /api/connectors can
// reuse the generic JSON:API list handler.
type ConnectorCatalog struct {
	registry ConnectorRegistry
}

func NewConnectorCatalog(registry ConnectorRegistry) *ConnectorCatalog {
	return &ConnectorCatalog{registry: registry}
}

// List returns the full catalog; the static set ignores search options.
func (c *ConnectorCatalog) List(_ context.Context, _ ...search.Option) (resource.ListResponse[domain.Connector], error) {
	cs := c.registry.List()
	return resource.NewListResponse(cs, len(cs)), nil
}

// DefaultConnectors is the built-in provider catalog. Rate profiles default to
// each provider's published per-account API ceiling so all orgs' calls stay
// under it (enforced via kit/ratelimit).
func DefaultConnectors() []domain.Connector {
	return []domain.Connector{
		{
			Slug:     "alpaca",
			Name:     "Alpaca",
			AuthType: domain.AuthTypeOAuth2,
			AuthURL:  "https://app.alpaca.markets/oauth/authorize",
			TokenURL: "https://api.alpaca.markets/oauth/token",
			Scopes:   []string{"account:write", "trading", "data"},
			Rate:     domain.RateProfile{Limit: 200, Window: time.Minute},
		},
		{
			Slug:     "binance",
			Name:     "Binance",
			AuthType: domain.AuthTypeAPIKey,
			Rate:     domain.RateProfile{Limit: 1200, Window: time.Minute},
		},
		{
			Slug:     "coinbase",
			Name:     "Coinbase",
			AuthType: domain.AuthTypeOAuth2,
			AuthURL:  "https://login.coinbase.com/oauth2/auth",
			TokenURL: "https://login.coinbase.com/oauth2/token",
			Scopes:   []string{"wallet:accounts:read", "wallet:trades:create"},
			Rate:     domain.RateProfile{Limit: 10, Window: time.Second},
		},
	}
}
