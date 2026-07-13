package migrate

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPgx5URL(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@h/db?sslmode=disable": "pgx5://u:p@h/db?sslmode=disable",
		"postgresql://u:p@h/db":               "pgx5://u:p@h/db",
		"pgx5://already":                      "pgx5://already",
		"mysql://other":                       "mysql://other",
	}
	for in, want := range cases {
		if got := pgx5URL(in); got != want {
			t.Errorf("pgx5URL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUp_AppliesAndIsIdempotent runs the embedded migrations against a fresh
// container DB (no init scripts), proving the auto-migrate path creates the full
// schema — including the M1 company_settings table — and that a second call is a
// clean no-op rather than an error.
func TestUp_AppliesAndIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB migrate test in -short mode")
	}
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("migtest"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Skipf("testcontainers Postgres unavailable (is Docker running?): %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// Fresh DB: Up must create the schema from nothing.
	if err := Up(dsn); err != nil {
		t.Fatalf("Up (fresh): %v", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM company_settings").Scan(&n); err != nil {
		t.Fatalf("company_settings should exist after Up: %v", err)
	}
	if n != 1 {
		t.Fatalf("company_settings rows = %d, want 1 (the seeded row)", n)
	}

	// Second run: already current -> nil (ErrNoChange swallowed).
	if err := Up(dsn); err != nil {
		t.Fatalf("Up (idempotent second run): %v", err)
	}
}
