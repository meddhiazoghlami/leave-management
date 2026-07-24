// Package api is the JSON REST API that a mobile (or any non-browser) client
// integrates against. It is deliberately separate from package handlers, which
// serves server-rendered HTML: the two share the data layer (store) and the pure
// business rules (internal/leave), but nothing of the transport. HTML handlers
// render templ components and fire HTMX toasts; API handlers marshal DTOs and
// return REST-style status codes with a JSON error envelope.
//
// Auth is the same Postgres-backed session token the web app issues, carried in
// an "Authorization: Bearer <token>" header instead of an HttpOnly cookie (see
// auth.RequireAPIAuth) — so a mobile login and a browser login are the same row
// in the sessions table.
package api

import (
	"context"
	"strconv"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/internal/leave"

	"github.com/gin-gonic/gin"
)

// Store is the data layer as the API consumes it — the same surface the HTML
// handlers use. Declared consumer-side (rather than importing *store.Store) so
// API handlers can be unit-tested with a fake; the concrete *store.Store
// satisfies it and Wire binds the two (see internal/app).
type Store interface {
	// employees
	GetEmployeeByEmail(ctx context.Context, email string) (db.Employee, error)
	GetEmployee(ctx context.Context, id int64) (db.Employee, error)
	ListEmployees(ctx context.Context, managerID int64) ([]db.ListEmployeesRow, error)
	// sessions
	CreateSession(ctx context.Context, token string, employeeID int64, expiresAt time.Time) (db.Session, error)
	DeleteSession(ctx context.Context, token string) error
	// leave types
	ListLeaveTypes(ctx context.Context) ([]db.LeaveType, error)
	CreateLeaveType(ctx context.Context, name string, defaultDays float64, color string) (db.LeaveType, error)
	// allocations
	UpsertAllocation(ctx context.Context, employeeID, leaveTypeID int64, year int32, days float64) (db.LeaveAllocation, error)
	// requests
	CreateLeaveRequest(ctx context.Context, employeeID, leaveTypeID int64, start, end time.Time, workingDays float64, reason string) (db.CreateLeaveRequestRow, error)
	GetLeaveRequest(ctx context.Context, id int64) (db.GetLeaveRequestRow, error)
	ListRequestsByEmployee(ctx context.Context, employeeID int64) ([]db.ListRequestsByEmployeeRow, error)
	ListPendingForManager(ctx context.Context, managerID int64) ([]db.ListPendingForManagerRow, error)
	SetRequestStatus(ctx context.Context, id int64, status string, decidedBy int64) error
	CancelOwnRequest(ctx context.Context, id, employeeID int64) error
	// balances
	ListBalances(ctx context.Context, employeeID int64, year int32, windowStart, windowEnd time.Time) ([]db.ListBalancesRow, error)
	// calendar
	ListApprovedInRange(ctx context.Context, start, end time.Time) ([]db.ListApprovedInRangeRow, error)
	// holidays
	ListHolidays(ctx context.Context) ([]db.PublicHoliday, error)
	ListHolidaysInRange(ctx context.Context, start, end time.Time) ([]db.PublicHoliday, error)
	CreateHoliday(ctx context.Context, name string, date time.Time) (db.PublicHoliday, error)
	DeleteHoliday(ctx context.Context, id int64) error
	// settings
	GetSettings(ctx context.Context) (db.CompanySetting, error)
	UpdateSettings(ctx context.Context, name string, leaveYearStartMonth int32, mon, tue, wed, thu, fri, sat, sun bool) error
}

// Handlers bundles the dependencies every API route needs.
type Handlers struct {
	Store Store
	Cfg   config.Config
}

func New(s Store, cfg config.Config) *Handlers {
	return &Handlers{Store: s, Cfg: cfg}
}

// fail writes the standard error envelope: {"error": "<message>"}. Every
// non-2xx API response goes through here so clients can parse failures uniformly.
func fail(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}

// balanceScope loads company settings and derives, from today, the current
// leave-year window used to sum approved usage plus the integer label that keys
// per-year allocation rows — identical to the web handler's scoping, so the API
// and the HTML dashboard report the same balances.
func (h *Handlers) balanceScope(ctx context.Context) (year int32, windowStart, windowEnd time.Time, err error) {
	s, err := h.Store.GetSettings(ctx)
	if err != nil {
		return 0, time.Time{}, time.Time{}, err
	}
	start, end, label := leave.LeaveYearWindow(time.Now(), int(s.LeaveYearStartMonth))
	return int32(label), start, end, nil
}

// idParam parses a numeric ":id" route param.
func idParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	return id, err == nil
}
