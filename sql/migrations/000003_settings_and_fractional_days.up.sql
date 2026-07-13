-- M1 (docs/next-steps.md): make the working week and leave year configurable,
-- and allow fractional leave days.
--
-- 1. company_settings is a single pinned row (id = 1) holding the working week
--    (which weekdays count) and the month the leave year starts on. Policy that
--    used to be hardcoded in Go (Mon–Fri weekend, Jan–Dec leave year) now lives
--    in the DB and is editable from /admin — "config over code".
-- 2. Day columns move INT -> NUMERIC(6,2) so allocations, per-request working
--    days, and computed balances can carry fractions (e.g. 22/yr -> 1.83/mo).

CREATE TABLE company_settings (
    -- One row only: the CHECK pins the id, so there is never more than one
    -- settings record to reconcile.
    id                     INT         PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    name                   TEXT        NOT NULL DEFAULT 'My Company',
    leave_year_start_month INT         NOT NULL DEFAULT 1
                               CHECK (leave_year_start_month BETWEEN 1 AND 12),
    -- The working week, one flag per weekday. Default is Mon–Fri (matches the
    -- old hardcoded behaviour); set work_friday/saturday for a Fri/Sat weekend.
    work_monday            BOOLEAN     NOT NULL DEFAULT true,
    work_tuesday           BOOLEAN     NOT NULL DEFAULT true,
    work_wednesday         BOOLEAN     NOT NULL DEFAULT true,
    work_thursday          BOOLEAN     NOT NULL DEFAULT true,
    work_friday            BOOLEAN     NOT NULL DEFAULT true,
    work_saturday          BOOLEAN     NOT NULL DEFAULT false,
    work_sunday            BOOLEAN     NOT NULL DEFAULT false,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the single row with the defaults above.
INSERT INTO company_settings (id) VALUES (1);

-- Fractional days everywhere a day count is stored.
ALTER TABLE leave_types       ALTER COLUMN default_days TYPE NUMERIC(6,2);
ALTER TABLE leave_allocations ALTER COLUMN days         TYPE NUMERIC(6,2);
ALTER TABLE leave_requests    ALTER COLUMN working_days TYPE NUMERIC(6,2);
