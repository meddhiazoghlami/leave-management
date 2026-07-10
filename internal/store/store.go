// Package store is the app-facing data layer. It owns the pgx pool and exposes
// domain methods to the handlers. Since Phase 7 the SQL + Scan all live in the
// sqlc-generated `db` package (now under internal/db); this file is pool
// lifecycle plus thin, well-named delegations.
//
// One job it does own: translating between the plain Go types handlers like to
// pass (int64, time.Time) and the pgtype wrappers sqlc emits for NULL-able
// columns — so the pgtype.Int8 dance stays here and never leaks into handlers.
package store

import (
	"context"
	"time"

	"github.com/dzovi/leave-management/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

func (s *Store) Close() { s.pool.Close() }

// managerRef wraps an employee id as the NULL-able bigint sqlc expects for the
// manager_id comparisons.
func managerRef(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: true} }

// ─────────────────────────────── employees ───────────────────────────────

func (s *Store) GetEmployeeByEmail(ctx context.Context, email string) (db.Employee, error) {
	return s.q.GetEmployeeByEmail(ctx, email)
}

func (s *Store) GetEmployee(ctx context.Context, id int64) (db.Employee, error) {
	return s.q.GetEmployee(ctx, id)
}

// CreateEmployee inserts an employee. managerID is optional (nil for the top of
// the org tree) and is translated to the NULL-able bigint sqlc expects.
func (s *Store) CreateEmployee(ctx context.Context, name, email, passwordHash, role string, managerID *int64) (db.Employee, error) {
	var mgr pgtype.Int8
	if managerID != nil {
		mgr = pgtype.Int8{Int64: *managerID, Valid: true}
	}
	return s.q.CreateEmployee(ctx, db.CreateEmployeeParams{
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
		ManagerID:    mgr,
	})
}

// ListEmployees returns everyone when managerID == 0, else just that manager's
// direct reports.
func (s *Store) ListEmployees(ctx context.Context, managerID int64) ([]db.ListEmployeesRow, error) {
	return s.q.ListEmployees(ctx, managerID)
}

// ─────────────────────────────── sessions ────────────────────────────────

func (s *Store) CreateSession(ctx context.Context, token string, employeeID int64, expiresAt time.Time) (db.Session, error) {
	return s.q.CreateSession(ctx, db.CreateSessionParams{Token: token, EmployeeID: employeeID, ExpiresAt: expiresAt})
}

// GetSessionEmployee resolves a session token to its (unexpired) employee. The
// generated query wraps the row in a struct; we unwrap it here.
func (s *Store) GetSessionEmployee(ctx context.Context, token string) (db.Employee, error) {
	row, err := s.q.GetSessionEmployee(ctx, token)
	return row.Employee, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.q.DeleteSession(ctx, token)
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	return s.q.DeleteExpiredSessions(ctx)
}

// ────────────────────────────── leave_types ──────────────────────────────

func (s *Store) ListLeaveTypes(ctx context.Context) ([]db.LeaveType, error) {
	return s.q.ListLeaveTypes(ctx)
}

func (s *Store) CreateLeaveType(ctx context.Context, name string, defaultDays int32, color string) (db.LeaveType, error) {
	return s.q.CreateLeaveType(ctx, db.CreateLeaveTypeParams{Name: name, DefaultDays: defaultDays, Color: color})
}

// ─────────────────────────── leave_allocations ───────────────────────────

func (s *Store) UpsertAllocation(ctx context.Context, employeeID, leaveTypeID int64, year, days int32) (db.LeaveAllocation, error) {
	return s.q.UpsertAllocation(ctx, db.UpsertAllocationParams{
		EmployeeID:  employeeID,
		LeaveTypeID: leaveTypeID,
		Year:        year,
		Days:        days,
	})
}

// ──────────────────────────── leave_requests ─────────────────────────────

func (s *Store) CreateLeaveRequest(ctx context.Context, employeeID, leaveTypeID int64, start, end time.Time, workingDays int32, reason string) (db.CreateLeaveRequestRow, error) {
	return s.q.CreateLeaveRequest(ctx, db.CreateLeaveRequestParams{
		EmployeeID:  employeeID,
		LeaveTypeID: leaveTypeID,
		StartDate:   start,
		EndDate:     end,
		WorkingDays: workingDays,
		Reason:      reason,
	})
}

func (s *Store) GetLeaveRequest(ctx context.Context, id int64) (db.GetLeaveRequestRow, error) {
	return s.q.GetLeaveRequest(ctx, id)
}

func (s *Store) ListRequestsByEmployee(ctx context.Context, employeeID int64) ([]db.ListRequestsByEmployeeRow, error) {
	return s.q.ListRequestsByEmployee(ctx, employeeID)
}

func (s *Store) ListPendingForManager(ctx context.Context, managerID int64) ([]db.ListPendingForManagerRow, error) {
	return s.q.ListPendingForManager(ctx, managerRef(managerID))
}

func (s *Store) CountPendingForManager(ctx context.Context, managerID int64) (int64, error) {
	return s.q.CountPendingForManager(ctx, managerRef(managerID))
}

// SetRequestStatus decides a pending request (approve/reject). No-op if the
// request is no longer pending — the guard is in the SQL.
func (s *Store) SetRequestStatus(ctx context.Context, id int64, status string, decidedBy int64) error {
	return s.q.SetRequestStatus(ctx, db.SetRequestStatusParams{ID: id, Status: status, DecidedBy: decidedBy})
}

func (s *Store) CancelOwnRequest(ctx context.Context, id, employeeID int64) error {
	return s.q.CancelOwnRequest(ctx, db.CancelOwnRequestParams{ID: id, EmployeeID: employeeID})
}

// ──────────────────────────────── balances ───────────────────────────────

func (s *Store) ListBalances(ctx context.Context, employeeID int64, year int32) ([]db.ListBalancesRow, error) {
	return s.q.ListBalances(ctx, db.ListBalancesParams{EmployeeID: employeeID, Year: year})
}

// ──────────────────────────────── calendar ───────────────────────────────

func (s *Store) ListApprovedInRange(ctx context.Context, start, end time.Time) ([]db.ListApprovedInRangeRow, error) {
	return s.q.ListApprovedInRange(ctx, db.ListApprovedInRangeParams{RangeStart: start, RangeEnd: end})
}

// ─────────────────────────────── holidays ────────────────────────────────

func (s *Store) ListHolidays(ctx context.Context) ([]db.PublicHoliday, error) {
	return s.q.ListHolidays(ctx)
}

func (s *Store) ListHolidaysInRange(ctx context.Context, start, end time.Time) ([]db.PublicHoliday, error) {
	return s.q.ListHolidaysInRange(ctx, db.ListHolidaysInRangeParams{RangeStart: start, RangeEnd: end})
}

func (s *Store) CreateHoliday(ctx context.Context, name string, date time.Time) (db.PublicHoliday, error) {
	return s.q.CreateHoliday(ctx, db.CreateHolidayParams{Name: name, HolidayDate: date})
}

func (s *Store) DeleteHoliday(ctx context.Context, id int64) error {
	return s.q.DeleteHoliday(ctx, id)
}
