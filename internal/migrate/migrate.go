// Package migrate applies the embedded SQL migrations to a database, so the app
// can bring a database up to the latest schema on startup (see
// config.AutoMigrate). It uses the same sql/migrations files as the `migrate`
// CLI and Docker Compose — one source of truth, one schema_migrations table.
package migrate

import (
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the "pgx5" driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/meddhiazoghlami/leave-management/sql/migrations"
)

// Up applies all pending migrations, returning nil when the schema is already
// current. dbURL is the same pgx connection string the app uses
// (postgres://…); it's rewritten to the pgx5:// scheme the migrate driver
// registers under, so no separate database driver dependency is needed.
func Up(dbURL string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL(dbURL))
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	defer m.Close() // best-effort: releases the migrator's own db handle

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// pgx5URL swaps a postgres:// | postgresql:// scheme for the pgx5:// scheme the
// golang-migrate pgx/v5 driver registers under. Any other scheme passes through
// unchanged.
func pgx5URL(dbURL string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(dbURL, p); ok {
			return "pgx5://" + rest
		}
	}
	return dbURL
}
