-- Reverse of 000002: drop the leave-management schema and restore the Phase 6
-- demo `users` table so migrating down lands exactly where 000001 left off.
-- Order matters — drop children before parents (FKs).

DROP TABLE IF EXISTS public_holidays;
DROP TABLE IF EXISTS leave_requests;
DROP TABLE IF EXISTS leave_allocations;
DROP TABLE IF EXISTS leave_types;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS employees;

CREATE TABLE users (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
