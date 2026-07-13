// Package migrations embeds the SQL migration files so the binary can apply
// pending migrations on startup (see config.AutoMigrate) without needing the
// files on disk. The same *.sql files are also read by the `migrate` CLI
// (`make migrate-up`) and Docker Compose — one source of truth.
package migrations

import "embed"

// FS holds every migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
