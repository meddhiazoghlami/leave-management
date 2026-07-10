-- Phase 9: the real leave-management domain. This retires the Phase 6 demo
-- `users` table and introduces the actual entities: employees (with a
-- self-referential manager link and a role), sessions for cookie auth, leave
-- types, per-year allocations, leave requests, and public holidays.
--
-- Enum-like columns use TEXT + CHECK rather than native Postgres enums: adding
-- a value to a native enum requires ALTER TYPE (and can't run in a transaction
-- pre-PG12), whereas a CHECK is trivial to change in a migration. sqlc maps
-- both to plain Go strings anyway.

DROP TABLE IF EXISTS users;

CREATE TABLE employees (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT        NOT NULL,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'employee'
                      CHECK (role IN ('employee', 'manager', 'admin')),
    -- Self-reference: an employee's manager is another employee. NULL for the
    -- top of the tree (e.g. the admin). ON DELETE SET NULL so removing a
    -- manager doesn't cascade-delete their reports.
    manager_id    BIGINT      REFERENCES employees(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    -- The token itself is the primary key: it's the random value we hand to the
    -- browser in a cookie and look up on every authenticated request.
    token       TEXT        PRIMARY KEY,
    employee_id BIGINT      NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE leave_types (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT        NOT NULL UNIQUE,
    default_days INT         NOT NULL DEFAULT 0,
    color        TEXT        NOT NULL DEFAULT '#6366f1',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE leave_allocations (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id   BIGINT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    leave_type_id BIGINT NOT NULL REFERENCES leave_types(id) ON DELETE CASCADE,
    year          INT    NOT NULL,
    days          INT    NOT NULL DEFAULT 0,
    -- One allocation per (employee, type, year); enables UPSERT on the trio.
    UNIQUE (employee_id, leave_type_id, year)
);

CREATE TABLE leave_requests (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_id   BIGINT      NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    leave_type_id BIGINT      NOT NULL REFERENCES leave_types(id),
    start_date    DATE        NOT NULL,
    end_date      DATE        NOT NULL,
    -- Working days (weekdays minus public holidays) computed in Go at submit
    -- time and snapshotted here, so balances stay stable even if the holiday
    -- calendar changes later.
    working_days  INT         NOT NULL,
    reason        TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    decided_by    BIGINT      REFERENCES employees(id) ON DELETE SET NULL,
    decided_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_date >= start_date)
);

CREATE TABLE public_holidays (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT        NOT NULL,
    holiday_date DATE        NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_leave_requests_employee ON leave_requests (employee_id);
CREATE INDEX idx_leave_requests_status   ON leave_requests (status);
CREATE INDEX idx_sessions_employee       ON sessions (employee_id);
