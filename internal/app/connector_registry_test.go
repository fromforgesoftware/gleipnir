package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

func TestConnectorRegistry_LookupAndList(t *testing.T) {
	reg := app.NewConnectorRegistry(
		domain.Connector{Slug: "binance", Name: "Binance", AuthType: domain.AuthTypeAPIKey},
		domain.Connector{Slug: "alpaca", Name: "Alpaca", AuthType: domain.AuthTypeOAuth2},
	)

	got, ok := reg.Lookup("binance")
	require.True(t, ok)
	assert.Equal(t, "Binance", got.Name)

	_, ok = reg.Lookup("missing")
	assert.False(t, ok)

	list := reg.List()
	require.Len(t, list, 2)
	assert.Equal(t, "alpaca", list[0].Slug, "List is sorted by slug")
	assert.Equal(t, "binance", list[1].Slug)
}

func TestDefaultConnectorRegistry_HasKnownConnectors(t *testing.T) {
	reg := app.NewDefaultConnectorRegistry()
	for _, slug := range []string{"alpaca", "binance", "coinbase"} {
		_, ok := reg.Lookup(slug)
		assert.True(t, ok, "default catalog must include %q", slug)
	}
	for _, c := range reg.List() {
		assert.True(t, c.AuthType.Valid(), "%s has a valid auth type", c.Slug)
		assert.NotEmpty(t, c.Name)
	}
}
