package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/config"
)

// clearEnv unsets keys for the duration of the test, restoring any prior value
// afterwards. Complements t.Setenv, which can only set.
func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if old, had := os.LookupEnv(k); had {
			t.Cleanup(func() { os.Setenv(k, old) })
		}
		os.Unsetenv(k)
	}
}

func TestLoad_FromEnvironment(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate from any ambient .env
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
	t.Setenv("ADDR", ":9999")
	t.Setenv("VITE_DEV", "true")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://u:p@localhost/db" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999", cfg.Addr)
	}
	if !cfg.ViteDev {
		t.Error("ViteDev = false, want true")
	}
	if cfg.SessionTTL != 7*24*time.Hour {
		t.Errorf("SessionTTL = %v, want 168h", cfg.SessionTTL)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://x")
	clearEnv(t, "ADDR", "VITE_DEV")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want default :8080", cfg.Addr)
	}
	if cfg.ViteDev {
		t.Error("ViteDev = true, want default false")
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Chdir(t.TempDir())
	clearEnv(t, "DATABASE_URL")

	if _, err := config.Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is unset")
	}
}

func TestLoad_ReadsDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// The value must be absent from the environment, or godotenv won't override.
	clearEnv(t, "DATABASE_URL", "ADDR")

	content := "DATABASE_URL=postgres://from-dotenv/db\nADDR=:7000\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://from-dotenv/db" {
		t.Errorf("DatabaseURL = %q, want value from .env", cfg.DatabaseURL)
	}
	if cfg.Addr != ":7000" {
		t.Errorf("Addr = %q, want :7000 from .env", cfg.Addr)
	}
}

func TestLoad_MalformedDotEnvIsAnError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t, "DATABASE_URL")

	// A directory named ".env" makes godotenv's read fail with a non-"not
	// exist" error, exercising the branch that surfaces it rather than ignoring.
	if err := os.Mkdir(filepath.Join(dir, ".env"), 0o755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	if _, err := config.Load(); err == nil {
		t.Fatal("expected error when .env cannot be read")
	}
}
