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
			Slug:        "alpaca",
			Name:        "Alpaca",
			Description: "Commission-free US stocks, ETFs and crypto with a REST and streaming API built for algorithmic trading.",
			DocsURL:     "https://docs.alpaca.markets/",
			AuthType:    domain.AuthTypeOAuth2,
			AuthURL:     "https://app.alpaca.markets/oauth/authorize",
			TokenURL:    "https://api.alpaca.markets/oauth/token",
			Scopes:      []string{"account:write", "trading", "data"},
			Rate:        domain.RateProfile{Limit: 200, Window: time.Minute},
		},
		{
			Slug:        "binance",
			Name:        "Binance",
			Description: "The largest crypto exchange by volume, with spot, margin and futures markets over REST and WebSocket.",
			DocsURL:     "https://developers.binance.com/docs",
			AuthType:    domain.AuthTypeAPIKey,
			Rate:        domain.RateProfile{Limit: 1200, Window: time.Minute},
		},
		{
			Slug:        "kraken",
			Name:        "Kraken",
			Description: "Spot and futures crypto trading with deep fiat liquidity and a long-stable REST API.",
			DocsURL:     "https://docs.kraken.com/api/",
			AuthType:    domain.AuthTypeAPIKey,
			Rate:        domain.RateProfile{Limit: 60, Window: time.Minute},
		},
		{
			Slug:        "bybit",
			Name:        "Bybit",
			Description: "Crypto derivatives and spot markets with unified-account REST and WebSocket endpoints.",
			DocsURL:     "https://bybit-exchange.github.io/docs/",
			AuthType:    domain.AuthTypeAPIKey,
			Rate:        domain.RateProfile{Limit: 600, Window: time.Minute},
		},
		{
			Slug:        "bitmex",
			Name:        "BitMEX",
			Description: "Crypto derivatives exchange specialising in perpetual swaps and futures.",
			DocsURL:     "https://www.bitmex.com/app/apiOverview",
			AuthType:    domain.AuthTypeAPIKey,
			Rate:        domain.RateProfile{Limit: 120, Window: time.Minute},
		},
		{
			Slug:        "ibkr-flex",
			Name:        "Interactive Brokers (Flex)",
			Description: "Read-only statement and trade history for Interactive Brokers accounts via Flex queries.",
			DocsURL:     "https://www.ibkrguides.com/clientportal/performanceandstatements/flex-web-service.htm",
			AuthType:    domain.AuthTypeAPIKey,
			Rate:        domain.RateProfile{Limit: 10, Window: time.Minute},
		},
		{
			Slug:        "tradovate",
			Name:        "Tradovate",
			Description: "Futures brokerage for CME micro and full-size contracts, with live and paper environments.",
			DocsURL:     "https://api.tradovate.com/",
			AuthType:    domain.AuthTypeAPIKey,
			Rate:        domain.RateProfile{Limit: 240, Window: time.Minute},
		},
		{
			Slug:        "polygon",
			Name:        "Polygon.io",
			Description: "Market data for US stocks, options, forex and crypto — real-time streams and historical bars.",
			DocsURL:     "https://polygon.io/docs",
			AuthType:    domain.AuthTypeAPIKey,
			Rate:        domain.RateProfile{Limit: 100, Window: time.Minute},
		},
		{
			Slug:        "coinbase",
			Name:        "Coinbase",
			Description: "Regulated US crypto exchange with spot markets and OAuth-based account access.",
			DocsURL:     "https://docs.cdp.coinbase.com/",
			AuthType:    domain.AuthTypeOAuth2,
			AuthURL:     "https://login.coinbase.com/oauth2/auth",
			TokenURL:    "https://login.coinbase.com/oauth2/token",
			Scopes:      []string{"wallet:accounts:read", "wallet:trades:create"},
			PKCE:        true,
			Rate:        domain.RateProfile{Limit: 10, Window: time.Second},
		},
	}
}
