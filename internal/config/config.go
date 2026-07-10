// Package config centralises the app's environment-driven settings. It loads a
// local .env file (if present) so `go run` works without exporting vars by hand;
// real deployments inject the environment directly and need no .env.
package config

import (
	"errors"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// DatabaseURL is the pgx connection string. Required — there is no default.
	DatabaseURL string
	// Addr is the host:port Gin listens on.
	Addr string
	// SessionTTL is how long a login session stays valid.
	SessionTTL time.Duration
	// ViteDev points the asset layer at the Vite dev server (HMR) instead of
	// the built manifest.
	ViteDev bool
}

// Load reads configuration from the environment. A .env file in the working
// directory is loaded first if present (absent in containers/prod, which is not
// an error). DATABASE_URL is mandatory and has no fallback: a missing value is a
// hard error rather than a silent connection to some default localhost database.
func Load() (Config, error) {
	// Optional local .env. Missing file is fine; a malformed one is not.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return Config{}, err
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, errors.New("DATABASE_URL is required (set it in the environment or a .env file — see .env.example)")
	}

	return Config{
		DatabaseURL: dbURL,
		Addr:        env("ADDR", ":8080"),
		SessionTTL:  7 * 24 * time.Hour,
		ViteDev:     os.Getenv("VITE_DEV") == "true",
	}, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
