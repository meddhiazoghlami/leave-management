// Package testsupport spins up a throwaway Postgres for integration and
// end-to-end tests using testcontainers-go. One container is started per test
// binary (lazily, on first use) with the real sql/migrations applied as init
// scripts; each NewStore call truncates every domain table so tests get a clean
// slate without paying for a fresh container each time.
//
// If Docker isn't reachable the helpers call t.Skip rather than failing, so
// `go test ./...` still passes on a machine without a Docker daemon — the DB
// tests just don't contribute coverage there.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// harness is the shared, package-lifetime Postgres. admin is a separate pool
// used only for truncation between tests, kept apart from the store's own pool.
type harness struct {
	container *postgres.PostgresContainer
	dsn       string
	store     *store.Store
	admin     *pgxpool.Pool
}

var (
	once   sync.Once
	shared *harness
	setErr error
)

// domainTables are every table the migrations create (users is dropped by
// 000002). Truncated with RESTART IDENTITY so id sequences reset per test.
var domainTables = []string{
	"employees", "sessions", "leave_types",
	"leave_allocations", "leave_requests", "public_holidays",
}

// NewStore returns a *store.Store backed by the shared container, with all
// domain tables truncated for isolation. Skips the test if Docker is
// unavailable.
func NewStore(t *testing.T) *store.Store {
	t.Helper()
	h := get(t)
	truncate(t, h)
	return h.store
}

// DSN returns the connection string of the shared container (for tests that
// need to open their own pool/connection). Skips if Docker is unavailable.
func DSN(t *testing.T) string {
	t.Helper()
	return get(t).dsn
}

func get(t *testing.T) *harness {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB-backed test in -short mode")
	}
	once.Do(func() { shared, setErr = start(context.Background()) })
	if setErr != nil {
		t.Skipf("testcontainers Postgres unavailable (is Docker running?): %v", setErr)
	}
	return shared
}

func start(ctx context.Context) (*harness, error) {
	scripts, err := migrationUpScripts()
	if err != nil {
		return nil, err
	}

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("leave_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(scripts...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("connection string: %w", err)
	}

	st, err := store.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open admin pool: %w", err)
	}

	return &harness{container: container, dsn: dsn, store: st, admin: admin}, nil
}

func truncate(t *testing.T, h *harness) {
	t.Helper()
	stmt := "TRUNCATE " + strings.Join(domainTables, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := h.admin.Exec(context.Background(), stmt); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

// migrationUpScripts returns the absolute paths of every *.up.sql file in
// sql/migrations, sorted so they apply in version order. The directory is found
// relative to this source file so it works regardless of the test's working dir.
func migrationUpScripts() ([]string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("cannot locate testsupport source file")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "sql", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var scripts []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".sql" && len(name) > len(".up.sql") &&
			name[len(name)-len(".up.sql"):] == ".up.sql" {
			scripts = append(scripts, filepath.Join(migrationsDir, name))
		}
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("no *.up.sql files found in %s", migrationsDir)
	}
	sort.Strings(scripts)
	return scripts, nil
}
