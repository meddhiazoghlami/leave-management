// Package handlers holds the HTTP layer: one method per route, grouped by
// feature across several files. Handlers depend only on the store (data) and
// views (HTML); auth/session concerns live in package auth.
package handlers

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/internal/leave"
	"github.com/meddhiazoghlami/leave-management/views"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
)

// Store is the data layer as the HTTP handlers consume it — every store method a
// handler calls, and nothing more. Declared consumer-side (rather than importing
// the concrete *store.Store) so handlers can be unit-tested with a fake and have
// their error branches exercised via fault injection. The concrete *store.Store
// satisfies this; Wire binds the two (see internal/app).
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
	CountPendingForManager(ctx context.Context, managerID int64) (int64, error)
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
	// health
	Ping(ctx context.Context) error
}

// Handlers bundles the dependencies every route needs.
type Handlers struct {
	Store Store
	Cfg   config.Config
}

func New(s Store, cfg config.Config) *Handlers {
	return &Handlers{Store: s, Cfg: cfg}
}

// render writes a templ component as an HTML response with the given status.
func render(c *gin.Context, status int, comp templ.Component) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = comp.Render(c.Request.Context(), c.Writer)
}

// toast sets the HX-Trigger header so the Alpine toast host (in the layout)
// pops a message. kind is "success" or "error" (controls the colour).
func toast(c *gin.Context, message, kind string) {
	payload := map[string]any{"toast": map[string]string{"message": message, "type": kind}}
	b, _ := json.Marshal(payload)
	c.Header("HX-Trigger", string(b))
}

// navFor builds the header view model for the current (authenticated) user,
// including the manager pending-approval badge count.
func (h *Handlers) navFor(c *gin.Context, active, title string) views.Nav {
	emp := auth.MustEmployee(c)
	nav := views.Nav{Title: title, Name: emp.Name, Role: emp.Role, Active: active}
	if nav.IsManager() {
		if n, err := h.Store.CountPendingForManager(c.Request.Context(), emp.ID); err == nil {
			nav.PendingCount = n
		}
	}
	return nav
}

// balanceScope loads company settings and derives, from today, the current
// leave-year window used to sum approved usage plus the integer label that keys
// per-year allocation rows. Balances, the profile page, and the allocation
// admin all scope to the same window this way.
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
