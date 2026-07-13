-- Reverse M1. Fractional day values are rounded to the nearest integer on the
-- way back to INT (lossy, but this is only for rolling the migration back).
ALTER TABLE leave_requests    ALTER COLUMN working_days TYPE INT USING ROUND(working_days)::int;
ALTER TABLE leave_allocations ALTER COLUMN days         TYPE INT USING ROUND(days)::int;
ALTER TABLE leave_types       ALTER COLUMN default_days TYPE INT USING ROUND(default_days)::int;

DROP TABLE IF EXISTS company_settings;
