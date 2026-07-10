// Package store is the app-facing data layer. It owns the pgx connection pool
// and exposes domain methods to the handlers.
//
// Phase 7 change: the hand-written SQL + rows.Scan from Phase 6 is gone. The
// User struct and every query/Scan now live in the sqlc-generated package `db`.
// This file shrank to pool lifecycle + one-line delegations. Same public API as
// Phase 6 (CreateUser / ListUsers), so main.go and the views barely change —
// only the *guts* moved from hand-written to generated.
package store

import (
	"context"

	"github.com/dzovi/leave-management/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store owns the pool and the generated query set. *pgxpool.Pool satisfies the
// generated db.DBTX interface, so db.New(pool) just works.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New opens a pgx connection pool, verifies it, and wires up the sqlc queries.
func New(ctx context.Context, dbURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, q: db.New(pool)}, nil
}

// Close releases the pool. Call it on shutdown.
func (s *Store) Close() { s.pool.Close() }

// CreateUser inserts one user. The SQL, the struct, and the Scan all live in
// the generated code now — this is a straight delegation.
func (s *Store) CreateUser(ctx context.Context, name string) (db.User, error) {
	return s.q.CreateUser(ctx, name)
}

// ListUsers returns all users, newest first. Also fully generated.
func (s *Store) ListUsers(ctx context.Context) ([]db.User, error) {
	return s.q.ListUsers(ctx)
}
