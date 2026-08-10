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
	"github.com/fromforgesoftware/go-kit/application/repository"
)

const (
	ownerA = "owner-a"
	ownerB = "owner-b"
)

func TestConnectionCreateGetSetStatusDelete(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	repo, err := db.NewConnectionRepository(client)
	require.NoError(t, err)

	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	created, err := repo.Create(ctx, domain.NewConnection(ownerA, "binance",
		domain.WithConnectionScopes([]string{"read", "trade"}),
		domain.WithConnectionExpiresAt(&exp),
	))
	require.NoError(t, err)
	require.NotEmpty(t, created.ID())
	assert.Equal(t, domain.ConnectionStatusActive, created.Status())

	t.Run("get by id maps owner and scopes", func(t *testing.T) {
		got, err := repo.Get(ctx, internaltest.GetByID(created.ID()))
		require.NoError(t, err)
		assert.Equal(t, "binance", got.Connector())
		assert.Equal(t, ownerA, got.Owner())
		assert.Equal(t, []string{"read", "trade"}, got.Scopes())
		require.NotNil(t, got.ExpiresAt())
		assert.WithinDuration(t, exp, *got.ExpiresAt(), time.Second)
	})

	t.Run("set status", func(t *testing.T) {
		require.NoError(t, repo.SetStatus(ctx, created.ID(), domain.ConnectionStatusNeedsReauth))
		got, err := repo.Get(ctx, internaltest.GetByID(created.ID()))
		require.NoError(t, err)
		assert.Equal(t, domain.ConnectionStatusNeedsReauth, got.Status())
	})

	t.Run("set status missing returns not found", func(t *testing.T) {
		err := repo.SetStatus(ctx, "00000000-0000-0000-0000-000000000000", domain.ConnectionStatusActive)
		require.Error(t, err)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.Delete(ctx, repository.DeleteTypeHard, internaltest.GetByID(created.ID())))
		_, err := repo.Get(ctx, internaltest.GetByID(created.ID()))
		require.Error(t, err)
	})
}

func TestConnectionListScopedByOwner(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	repo, err := db.NewConnectionRepository(client)
	require.NoError(t, err)

	_, err = repo.Create(ctx, domain.NewConnection(ownerA, "binance"))
	require.NoError(t, err)
	_, err = repo.Create(ctx, domain.NewConnection(ownerA, "alpaca"))
	require.NoError(t, err)
	_, err = repo.Create(ctx, domain.NewConnection(ownerB, "ibkr"))
	require.NoError(t, err)

	got, err := repo.List(ctx, internaltest.FilterByOwner(ownerA))
	require.NoError(t, err)
	assert.Equal(t, 2, got.TotalCount())
	assert.Len(t, got.Results(), 2)
}

// TestConnectionDeleteAcceptsAnOwnerFilter pins the query the usecase actually sends.
//
// The usecase scopes the DELETE by owner as well as id, so the statement itself cannot touch another
// workspace's row — there is no window between "is this mine?" and "delete it". The kit's query
// validator rejects any filter it has not been told to expect, so the repository has to declare
// owner as permitted.
//
// This is an integration test because the failure lives in the validator, and the unit tests use a
// fake repository that does not validate anything: everything passed locally and the delete returned
// 400 "filter by owner is not allowed" against the running service.
func TestConnectionDeleteAcceptsAnOwnerFilter(t *testing.T) {
	client := internaltest.GetDB(t)
	t.Cleanup(func() { internaltest.TruncateTables(t, client) })

	ctx := context.Background()
	repo, err := db.NewConnectionRepository(client)
	require.NoError(t, err)

	mine, err := repo.Create(ctx, domain.NewConnection(ownerA, "kraken"))
	require.NoError(t, err)
	theirs, err := repo.Create(ctx, domain.NewConnection(ownerB, "kraken"))
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, repository.DeleteTypeHard,
		internaltest.GetByID(mine.ID()), internaltest.FilterByOwner(ownerA)))

	gone, err := repo.Get(ctx, internaltest.GetByID(mine.ID()))
	assert.Nil(t, gone, "the owner's own connection should be deleted")
	assert.Error(t, err)

	// The other workspace's row is untouched, which is the point of scoping the statement.
	survivor, err := repo.Get(ctx, internaltest.GetByID(theirs.ID()))
	require.NoError(t, err)
	require.NotNil(t, survivor)

	// And an id+owner pair that do not match deletes nothing rather than falling back to id alone.
	require.NoError(t, repo.Delete(ctx, repository.DeleteTypeHard,
		internaltest.GetByID(theirs.ID()), internaltest.FilterByOwner(ownerA)))
	stillThere, err := repo.Get(ctx, internaltest.GetByID(theirs.ID()))
	require.NoError(t, err)
	assert.NotNil(t, stillThere, "a mismatched owner must not delete the row")
}
