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

func TestCredentialCreateGetReplace(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	conns, err := db.NewConnectionRepository(client)
	require.NoError(t, err)
	creds, err := db.NewCredentialRepository(client)
	require.NoError(t, err)

	conn, err := conns.Create(ctx, domain.NewConnection(ownerA, "binance"))
	require.NoError(t, err)

	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	created, err := creds.Create(ctx, domain.NewCredential(conn.ID(), domain.CredentialKindOAuthTokens,
		[]byte("ciphertext-bytes"), []byte("wrapped-dek"),
		domain.WithCredentialKeyID("local"),
		domain.WithCredentialExpiresAt(&exp),
	))
	require.NoError(t, err)
	require.NotEmpty(t, created.ID())

	t.Run("get by connection", func(t *testing.T) {
		got, err := creds.Get(ctx, internaltest.GetByConnection(conn.ID()))
		require.NoError(t, err)
		assert.Equal(t, conn.ID(), got.Connection().ID())
		assert.Equal(t, domain.CredentialKindOAuthTokens, got.Kind())
		assert.Equal(t, []byte("ciphertext-bytes"), got.Ciphertext())
		assert.Equal(t, []byte("wrapped-dek"), got.WrappedKey())
		assert.Equal(t, "local", got.KeyID())
	})

	t.Run("replace re-seals in place keeping the id", func(t *testing.T) {
		newExp := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Microsecond)
		require.NoError(t, creds.Replace(ctx, created.ID(),
			[]byte("new-ciphertext"), []byte("new-wrapped"), "local", &newExp))

		got, err := creds.Get(ctx, internaltest.GetByID(created.ID()))
		require.NoError(t, err)
		assert.Equal(t, []byte("new-ciphertext"), got.Ciphertext())
		assert.Equal(t, []byte("new-wrapped"), got.WrappedKey())
		require.NotNil(t, got.ExpiresAt())
		assert.WithinDuration(t, newExp, *got.ExpiresAt(), time.Second)
	})
}

func TestCredentialListDueForRefresh(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	conns, err := db.NewConnectionRepository(client)
	require.NoError(t, err)
	creds, err := db.NewCredentialRepository(client)
	require.NoError(t, err)

	conn, err := conns.Create(ctx, domain.NewConnection(ownerA, "binance"))
	require.NoError(t, err)

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(24 * time.Hour)

	_, err = creds.Create(ctx, domain.NewCredential(conn.ID(), domain.CredentialKindAPIKey,
		[]byte("c1"), []byte("w1"), domain.WithCredentialExpiresAt(&past)))
	require.NoError(t, err)
	_, err = creds.Create(ctx, domain.NewCredential(conn.ID(), domain.CredentialKindAPIKey,
		[]byte("c2"), []byte("w2"), domain.WithCredentialExpiresAt(&future)))
	require.NoError(t, err)
	_, err = creds.Create(ctx, domain.NewCredential(conn.ID(), domain.CredentialKindAPIKey,
		[]byte("c3"), []byte("w3"))) // no expiry → never due
	require.NoError(t, err)

	due, err := creds.ListDueForRefresh(ctx, now.Add(time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, []byte("c1"), due[0].Ciphertext())
}
