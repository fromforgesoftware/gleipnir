package main

import (
	"context"
	"embed"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/lib/pq"

	"github.com/fromforgesoftware/go-kit/migrator"
)

//go:embed migrations
var migrationsFS embed.FS

func main() {
	if err := migrator.Up(context.Background(), migrationsFS, migrator.WithServiceName("gleipnir")); err != nil {
		panic(err)
	}
}
