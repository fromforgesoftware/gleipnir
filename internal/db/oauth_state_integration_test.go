//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fromforgesoftware/gleipnir/internal/db"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/internaltest"
)

func TestOAuthStateCreateAndConsume(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	conns, err := db.NewConnectionRepository(client)
	require.NoError(t, err)
	states, err := db.NewOAuthStateRepository(client)
	require.NoError(t, err)

	conn, err := conns.Create(ctx, domain.NewConnection(ownerA, "coinbase"))
	require.NoError(t, err)

	now := time.Now().UTC()

	created, err := states.Create(ctx, domain.NewOAuthState(
		"state-ok", conn.ID(), "coinbase", "https://app/cb", now.Add(10*time.Minute),
		domain.WithOAuthStateCodeVerifier("verifier-x")))
	require.NoError(t, err)
	require.NotEmpty(t, created.ID())

	t.Run("consume returns the pinned redirect and verifier", func(t *testing.T) {
		got, err := states.Consume(ctx, "state-ok", now)
		require.NoError(t, err)
		assert.Equal(t, conn.ID(), got.Connection().ID())
		assert.Equal(t, "coinbase", got.Connector())
		assert.Equal(t, "https://app/cb", got.RedirectURI())
		assert.Equal(t, "verifier-x", got.CodeVerifier())
		require.NotNil(t, got.ConsumedAt())
	})

	t.Run("replay is rejected (single-use)", func(t *testing.T) {
		_, err := states.Consume(ctx, "state-ok", now)
		require.Error(t, err, "an already-consumed state cannot be consumed again")
	})
}

func TestOAuthStateConsumeUnknown(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	states, err := db.NewOAuthStateRepository(client)
	require.NoError(t, err)

	_, err = states.Consume(context.Background(), "no-such-state", time.Now())
	require.Error(t, err, "an unknown state is rejected")
}

func TestOAuthStateConsumeExpired(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	conns, err := db.NewConnectionRepository(client)
	require.NoError(t, err)
	states, err := db.NewOAuthStateRepository(client)
	require.NoError(t, err)

	conn, err := conns.Create(ctx, domain.NewConnection(ownerA, "coinbase"))
	require.NoError(t, err)

	now := time.Now().UTC()
	_, err = states.Create(ctx, domain.NewOAuthState(
		"state-expired", conn.ID(), "coinbase", "https://app/cb", now.Add(-time.Minute)))
	require.NoError(t, err)

	_, err = states.Consume(ctx, "state-expired", now)
	require.Error(t, err, "an expired state is rejected")
}

func TestOAuthStateDeleteExpired(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	conns, err := db.NewConnectionRepository(client)
	require.NoError(t, err)
	states, err := db.NewOAuthStateRepository(client)
	require.NoError(t, err)

	conn, err := conns.Create(ctx, domain.NewConnection(ownerA, "coinbase"))
	require.NoError(t, err)

	now := time.Now().UTC()
	_, err = states.Create(ctx, domain.NewOAuthState("old", conn.ID(), "coinbase", "https://app/cb", now.Add(-time.Hour)))
	require.NoError(t, err)
	_, err = states.Create(ctx, domain.NewOAuthState("fresh", conn.ID(), "coinbase", "https://app/cb", now.Add(time.Hour)))
	require.NoError(t, err)

	n, err := states.DeleteExpired(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// The fresh one survives and is still consumable.
	_, err = states.Consume(ctx, "fresh", now)
	require.NoError(t, err)
}
