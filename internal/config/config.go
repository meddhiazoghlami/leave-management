// Package config centralises the app's environment-driven settings. Phase 9
// pulls the ad-hoc os.Getenv calls that lived in main.go into one typed struct
// so the rest of the app takes a *config.Config instead of reading env itself.
package config

import (
	"os"
	"time"
)

type Config struct {
	// DatabaseURL is the pgx connection string.
	DatabaseURL string
	// Addr is the host:port Gin listens on.
	Addr string
	// SessionTTL is how long a login session stays valid.
	SessionTTL time.Duration
	// ViteDev points the asset layer at the Vite dev server (HMR) instead of
	// the built manifest.
	ViteDev bool
}

// Load reads configuration from the environment, applying sensible local
// defaults so `go run .` works out of the box against the project's Docker
// Postgres.
func Load() Config {
	return Config{
		DatabaseURL: env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable"),
		Addr:        env("ADDR", ":8080"),
		SessionTTL:  7 * 24 * time.Hour,
		ViteDev:     os.Getenv("VITE_DEV") == "true",
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
