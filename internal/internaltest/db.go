//go:build integration

// Package internaltest holds test helpers shared by Gleipnir's integration tests.
package internaltest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fromforgesoftware/go-kit/migrator"
	"github.com/fromforgesoftware/go-kit/persistence/gormdb"
	"github.com/fromforgesoftware/go-kit/persistence/gormdb/gormdbtest"
	"github.com/stretchr/testify/require"
)

// GetDB returns a per-process singleton Postgres with Gleipnir's migrations
// applied by the real kit migrator. DB_SCHEMA=gleipnir mirrors prod; the
// common-pre-migration bootstrap creates the gleipnir schema before
// golang-migrate's tracking table needs it.
func GetDB(t *testing.T) *gormdb.DBClient {
	t.Helper()

	tdb := gormdbtest.GetDB(t, "gleipnir")
	if tdb == nil {
		t.Skip("test database unavailable (docker/gnomock); skipping integration test")
	}

	t.Setenv("DB_HOST", tdb.Host)
	t.Setenv("DB_PORT", fmt.Sprintf("%d", tdb.Port))
	t.Setenv("DB_USER", tdb.User)
	t.Setenv("DB_PASSWORD", tdb.Password)
	t.Setenv("DB_NAME", tdb.DBName)
	t.Setenv("DB_SSL", "disable")
	t.Setenv("DB_SCHEMA", "gleipnir")

	require.NoError(t, migrator.Up(context.Background(), os.DirFS(migratorDir()), migrator.WithServiceName("gleipnir")))
	return tdb.DBClient
}

// TruncateTables wipes Gleipnir's tables between tests sharing the singleton
// container.
func TruncateTables(t *testing.T, db *gormdb.DBClient) {
	t.Helper()
	require.NoError(t, db.Exec(`TRUNCATE TABLE gleipnir.connection, gleipnir.credential RESTART IDENTITY CASCADE;`).Error)
}

func migratorDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "cmd", "migrator")
}
