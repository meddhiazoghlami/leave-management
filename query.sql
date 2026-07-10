-- Phase 7: these annotated queries are the *entire* input to sqlc. From the
-- schema (migrations/) plus the annotations below, sqlc generates a typed Go
-- function for each query and the structs they return. No hand-written Scan.
--
-- :one  -> returns a single row (returns (T, error))
-- :many -> returns a slice   (returns ([]T, error))
-- :exec -> returns no rows   (returns error)

-- name: CreateUser :one
INSERT INTO users (name) VALUES ($1)
RETURNING id, name, created_at;

-- name: ListUsers :many
SELECT id, name, created_at FROM users
ORDER BY created_at DESC;
