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
	// AutoMigrate, when true, makes the app apply any pending DB migrations on
	// startup (before opening the store). Handy for local dev so `serve`/`seed`
	// "just work"; left false in production, where migrations run as a
	// deliberate, separate step (the Docker Compose `migrate` service).
	AutoMigrate bool
	// BaseURL is the externally-reachable root of the app, used to build links
	// in outbound email (e.g. the login page). Not the listen Addr.
	BaseURL string

	// Bootstrap accounts. On `serve` startup the app ensures an admin and an HR
	// account exist for these emails, mailing each a freshly-generated password
	// (see internal/bootstrap). Empty means "don't bootstrap that role".
	BootstrapAdminEmail string
	BootstrapHREmail    string

	// SMTP is the outbound mail transport used to deliver bootstrap credentials.
	// Required only when a bootstrap account actually needs creating.
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
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
		DatabaseURL:         dbURL,
		Addr:                env("ADDR", ":8080"),
		SessionTTL:          7 * 24 * time.Hour,
		ViteDev:             os.Getenv("VITE_DEV") == "true",
		AutoMigrate:         os.Getenv("AUTO_MIGRATE") == "true",
		BaseURL:             env("BASE_URL", "http://localhost:8080"),
		BootstrapAdminEmail: os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapHREmail:    os.Getenv("BOOTSTRAP_HR_EMAIL"),
		SMTPHost:            os.Getenv("SMTP_HOST"),
		SMTPPort:            env("SMTP_PORT", "587"),
		SMTPUsername:        os.Getenv("SMTP_USERNAME"),
		SMTPPassword:        os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:            os.Getenv("SMTP_FROM"),
	}, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
