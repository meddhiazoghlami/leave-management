package cli

import (
	"strings"
	"testing"

	"github.com/meddhiazoghlami/leave-management/internal/testsupport"
)

// setArgs points the shared rootCmd at the given args for one test and restores
// the default (os.Args) afterwards.
func setArgs(t *testing.T, args ...string) {
	t.Helper()
	rootCmd.SetArgs(args)
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
}

func TestExecute_Help(t *testing.T) {
	setArgs(t, "--help")
	if err := Execute(); err != nil {
		t.Fatalf("--help should succeed, got %v", err)
	}
}

func TestExecute_Version(t *testing.T) {
	setArgs(t, "--version")
	if err := Execute(); err != nil {
		t.Fatalf("--version should succeed, got %v", err)
	}
}

func TestServe_MissingDatabaseURL(t *testing.T) {
	t.Chdir(t.TempDir()) // no .env to supply a DATABASE_URL
	t.Setenv("DATABASE_URL", "")
	setArgs(t, "serve")
	if err := Execute(); err == nil {
		t.Fatal("serve without DATABASE_URL should error")
	}
}

func TestSeed_MissingDatabaseURL(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "")
	setArgs(t, "seed")
	if err := Execute(); err == nil {
		t.Fatal("seed without DATABASE_URL should error")
	}
}

// unreachableDSN parses fine but points at a port nothing listens on, so the
// pool's Ping fails fast — exercising provideStore's connect-error branch.
const unreachableDSN = "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"

func TestServe_UnreachableDB(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", unreachableDSN)
	setArgs(t, "serve")
	if err := Execute(); err == nil {
		t.Fatal("serve should error when the database is unreachable")
	}
}

func TestSeed_UnreachableDB(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", unreachableDSN)
	setArgs(t, "seed")
	if err := Execute(); err == nil {
		t.Fatal("seed should error when the database is unreachable")
	}
}

// TestSeed_Success runs the real seed command against the container DB, covering
// app.InitializeStore, provideStore, and seed.Run end to end.
func TestSeed_Success(t *testing.T) {
	dsn := testsupport.DSN(t) // skips when Docker is unavailable
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", dsn)
	setArgs(t, "seed")
	if err := Execute(); err != nil {
		t.Fatalf("seed command: %v", err)
	}
}

// TestServe_AssetsMissing drives serve far enough to build the app and connect
// to the DB, then fails at asset-manifest loading (there's no build/ in the temp
// cwd). This covers everything in serve's RunE except the blocking Run call.
func TestServe_AssetsMissing(t *testing.T) {
	dsn := testsupport.DSN(t)
	t.Chdir(t.TempDir()) // no public/build/.vite/manifest.json here
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("VITE_DEV", "") // force prod mode so it reads the (absent) manifest
	setArgs(t, "serve")

	err := Execute()
	if err == nil {
		t.Fatal("serve should fail when the asset manifest is missing")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("error = %v, want it to mention the manifest", err)
	}
}
