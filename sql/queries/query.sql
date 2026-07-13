-- Phase 9: the real domain's queries. sqlc turns each annotated statement into
-- a typed Go method on *Queries. Conventions worth noting:
--   :one/:many/:exec  -> single row / slice / no rows
--   @name / ::type    -> named params with an explicit cast, so sqlc infers the
--                        Go type we want (and NULL-able columns don't leak
--                        pgtype wrappers into params).
--   sqlc.embed(x)     -> return the whole `x` table struct as a nested field.
--
-- Deliberately, no query SELECTs the nullable timestamp `decided_at` (nor
-- `decided_by`): the global timestamptz->time.Time override would force it to a
-- non-null time.Time and a NULL would fail to scan. Those columns are written
-- (SetRequestStatus) but never read back into Go.

-- ─────────────────────────────── employees ───────────────────────────────

-- name: GetEmployeeByEmail :one
SELECT id, name, email, password_hash, role, manager_id, created_at
FROM employees
WHERE email = $1;

-- name: GetEmployee :one
SELECT id, name, email, password_hash, role, manager_id, created_at
FROM employees
WHERE id = $1;

-- Employees are created by the seed program (no self-registration in scope).
-- name: CreateEmployee :one
INSERT INTO employees (name, email, password_hash, role, manager_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, name, email, password_hash, role, manager_id, created_at;

-- ListEmployees doubles as the manager-scoped list: pass manager_id = 0 to get
-- everyone (admin view), or a real id to get just that manager's reports.
-- name: ListEmployees :many
SELECT e.id, e.name, e.email, e.role, COALESCE(m.name, '') AS manager_name
FROM employees e
LEFT JOIN employees m ON m.id = e.manager_id
WHERE (@manager_id::bigint = 0 OR e.manager_id = @manager_id::bigint)
ORDER BY e.name;

-- ──────────────────────────────── sessions ───────────────────────────────

-- name: CreateSession :one
INSERT INTO sessions (token, employee_id, expires_at)
VALUES ($1, $2, $3)
RETURNING token, employee_id, created_at, expires_at;

-- One round-trip auth lookup: valid (unexpired) token -> the full employee row.
-- name: GetSessionEmployee :one
SELECT sqlc.embed(e)
FROM sessions s
JOIN employees e ON e.id = s.employee_id
WHERE s.token = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();

-- ────────────────────────────── leave_types ──────────────────────────────

-- name: ListLeaveTypes :many
SELECT id, name, default_days, color, created_at
FROM leave_types
ORDER BY name;

-- name: CreateLeaveType :one
INSERT INTO leave_types (name, default_days, color)
VALUES ($1, $2, $3)
RETURNING id, name, default_days, color, created_at;

-- ─────────────────────────── leave_allocations ───────────────────────────

-- name: UpsertAllocation :one
INSERT INTO leave_allocations (employee_id, leave_type_id, year, days)
VALUES ($1, $2, $3, $4)
ON CONFLICT (employee_id, leave_type_id, year)
DO UPDATE SET days = EXCLUDED.days
RETURNING id, employee_id, leave_type_id, year, days;

-- ──────────────────────────── leave_requests ─────────────────────────────

-- name: CreateLeaveRequest :one
INSERT INTO leave_requests (employee_id, leave_type_id, start_date, end_date, working_days, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, employee_id, leave_type_id, start_date, end_date, working_days, reason, status, created_at;

-- name: GetLeaveRequest :one
SELECT id, employee_id, leave_type_id, start_date, end_date, working_days, reason, status, created_at
FROM leave_requests
WHERE id = $1;

-- name: ListRequestsByEmployee :many
SELECT r.id, r.leave_type_id, lt.name AS leave_type_name, lt.color AS leave_type_color,
       r.start_date, r.end_date, r.working_days, r.reason, r.status, r.created_at
FROM leave_requests r
JOIN leave_types lt ON lt.id = r.leave_type_id
WHERE r.employee_id = $1
ORDER BY r.created_at DESC;

-- name: ListPendingForManager :many
SELECT r.id, r.employee_id, e.name AS employee_name,
       lt.name AS leave_type_name, lt.color AS leave_type_color,
       r.start_date, r.end_date, r.working_days, r.reason, r.created_at
FROM leave_requests r
JOIN employees e ON e.id = r.employee_id
JOIN leave_types lt ON lt.id = r.leave_type_id
WHERE r.status = 'pending' AND e.manager_id = $1
ORDER BY r.created_at;

-- name: CountPendingForManager :one
SELECT COUNT(*)
FROM leave_requests r
JOIN employees e ON e.id = r.employee_id
WHERE r.status = 'pending' AND e.manager_id = $1;

-- Only a pending request can be decided; the guard is in the WHERE clause.
-- name: SetRequestStatus :exec
UPDATE leave_requests
SET status = @status, decided_by = @decided_by::bigint, decided_at = now()
WHERE id = @id AND status = 'pending';

-- name: CancelOwnRequest :exec
UPDATE leave_requests
SET status = 'cancelled'
WHERE id = @id AND employee_id = @employee_id AND status = 'pending';

-- ──────────────────────────────── balances ───────────────────────────────

-- Per leave type: allocated days for the year minus days already used by
-- APPROVED requests in that year. LEFT JOINs so a type with no allocation and
-- no usage still shows up (as 0 / 0).
-- Per leave type: allocated days for the leave year minus days already used by
-- APPROVED requests whose start_date falls in that leave-year window. The
-- window (and the @year label that keys allocations) is computed in Go from
-- company_settings.leave_year_start_month, so a non-January leave year works.
-- name: ListBalances :many
SELECT lt.id AS leave_type_id, lt.name AS leave_type_name, lt.color AS leave_type_color,
       COALESCE(a.days, 0)::numeric AS allocated,
       COALESCE(u.used, 0)::numeric AS used,
       (COALESCE(a.days, 0) - COALESCE(u.used, 0))::numeric AS remaining
FROM leave_types lt
LEFT JOIN leave_allocations a
       ON a.leave_type_id = lt.id
      AND a.employee_id = @employee_id
      AND a.year = @year::int
LEFT JOIN (
    SELECT leave_type_id, SUM(working_days) AS used
    FROM leave_requests
    WHERE employee_id = @employee_id
      AND status = 'approved'
      AND start_date BETWEEN @window_start::date AND @window_end::date
    GROUP BY leave_type_id
) u ON u.leave_type_id = lt.id
ORDER BY lt.name;

-- ──────────────────────────────── calendar ───────────────────────────────

-- Approved requests overlapping [range_start, range_end]. Overlap = starts on
-- or before the range end AND ends on or after the range start.
-- name: ListApprovedInRange :many
SELECT r.id, e.name AS employee_name,
       lt.name AS leave_type_name, lt.color AS leave_type_color,
       r.start_date, r.end_date
FROM leave_requests r
JOIN employees e ON e.id = r.employee_id
JOIN leave_types lt ON lt.id = r.leave_type_id
WHERE r.status = 'approved'
  AND r.start_date <= @range_end::date
  AND r.end_date >= @range_start::date
ORDER BY r.start_date;

-- ─────────────────────────────── holidays ────────────────────────────────

-- name: ListHolidays :many
SELECT id, name, holiday_date, created_at
FROM public_holidays
ORDER BY holiday_date;

-- name: ListHolidaysInRange :many
SELECT id, name, holiday_date, created_at
FROM public_holidays
WHERE holiday_date BETWEEN @range_start::date AND @range_end::date
ORDER BY holiday_date;

-- name: CreateHoliday :one
INSERT INTO public_holidays (name, holiday_date)
VALUES ($1, $2)
RETURNING id, name, holiday_date, created_at;

-- name: DeleteHoliday :exec
DELETE FROM public_holidays WHERE id = $1;

-- ─────────────────────────── company settings ────────────────────────────

-- The settings row is pinned to id = 1 (see migration 000003), so both queries
-- target it directly.
-- name: GetSettings :one
SELECT id, name, leave_year_start_month,
       work_monday, work_tuesday, work_wednesday, work_thursday,
       work_friday, work_saturday, work_sunday, updated_at
FROM company_settings
WHERE id = 1;

-- name: UpdateSettings :exec
UPDATE company_settings
SET name                   = @name,
    leave_year_start_month = @leave_year_start_month::int,
    work_monday            = @work_monday,
    work_tuesday           = @work_tuesday,
    work_wednesday         = @work_wednesday,
    work_thursday          = @work_thursday,
    work_friday            = @work_friday,
    work_saturday          = @work_saturday,
    work_sunday            = @work_sunday,
    updated_at             = now()
WHERE id = 1;
