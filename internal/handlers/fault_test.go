package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/api"
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/internal/handlers"
	"github.com/meddhiazoghlami/leave-management/internal/server"

	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/trace/noop"
)

// boom is the injected failure the fake store returns for selected methods.
var boom = errors.New("boom")

// fakeStore implements both handlers.Store and auth.SessionStore. It returns
// canned values for the "happy" methods and `boom` for any method named in
// fail, letting each test drive a specific error (500/503/404/403) branch that
// a real database would rarely produce.
type fakeStore struct {
	emp    db.Employee           // GetSessionEmployee + GetEmployeeByEmail (the principal)
	target db.Employee           // GetEmployee (e.g. a profile target / requester)
	req    db.GetLeaveRequestRow // GetLeaveRequest
	fail   map[string]bool
}

func (f *fakeStore) e(name string) error {
	if f.fail[name] {
		return boom
	}
	return nil
}

// auth.SessionStore
func (f *fakeStore) GetSessionEmployee(context.Context, string) (db.Employee, error) {
	return f.emp, f.e("GetSessionEmployee")
}

// handlers.Store
func (f *fakeStore) GetEmployeeByEmail(context.Context, string) (db.Employee, error) {
	return f.emp, f.e("GetEmployeeByEmail")
}
func (f *fakeStore) GetEmployee(context.Context, int64) (db.Employee, error) {
	return f.target, f.e("GetEmployee")
}
func (f *fakeStore) ListEmployees(context.Context, int64) ([]db.ListEmployeesRow, error) {
	return nil, f.e("ListEmployees")
}
func (f *fakeStore) CreateSession(context.Context, string, int64, time.Time) (db.Session, error) {
	return db.Session{}, f.e("CreateSession")
}
func (f *fakeStore) DeleteSession(context.Context, string) error { return f.e("DeleteSession") }
func (f *fakeStore) ListLeaveTypes(context.Context) ([]db.LeaveType, error) {
	return nil, f.e("ListLeaveTypes")
}
func (f *fakeStore) CreateLeaveType(context.Context, string, float64, string) (db.LeaveType, error) {
	return db.LeaveType{}, f.e("CreateLeaveType")
}
func (f *fakeStore) UpsertAllocation(context.Context, int64, int64, int32, float64) (db.LeaveAllocation, error) {
	return db.LeaveAllocation{}, f.e("UpsertAllocation")
}
func (f *fakeStore) CreateLeaveRequest(context.Context, int64, int64, time.Time, time.Time, float64, string) (db.CreateLeaveRequestRow, error) {
	return db.CreateLeaveRequestRow{}, f.e("CreateLeaveRequest")
}
func (f *fakeStore) GetLeaveRequest(context.Context, int64) (db.GetLeaveRequestRow, error) {
	return f.req, f.e("GetLeaveRequest")
}
func (f *fakeStore) ListRequestsByEmployee(context.Context, int64) ([]db.ListRequestsByEmployeeRow, error) {
	return nil, f.e("ListRequestsByEmployee")
}
func (f *fakeStore) ListPendingForManager(context.Context, int64) ([]db.ListPendingForManagerRow, error) {
	return nil, f.e("ListPendingForManager")
}
func (f *fakeStore) CountPendingForManager(context.Context, int64) (int64, error) {
	return 0, f.e("CountPendingForManager")
}
func (f *fakeStore) SetRequestStatus(context.Context, int64, string, int64) error {
	return f.e("SetRequestStatus")
}
func (f *fakeStore) CancelOwnRequest(context.Context, int64, int64) error {
	return f.e("CancelOwnRequest")
}
func (f *fakeStore) ListBalances(context.Context, int64, int32, time.Time, time.Time) ([]db.ListBalancesRow, error) {
	return nil, f.e("ListBalances")
}
func (f *fakeStore) ListApprovedInRange(context.Context, time.Time, time.Time) ([]db.ListApprovedInRangeRow, error) {
	return nil, f.e("ListApprovedInRange")
}
func (f *fakeStore) ListHolidays(context.Context) ([]db.PublicHoliday, error) {
	return nil, f.e("ListHolidays")
}
func (f *fakeStore) ListHolidaysInRange(context.Context, time.Time, time.Time) ([]db.PublicHoliday, error) {
	return nil, f.e("ListHolidaysInRange")
}
func (f *fakeStore) CreateHoliday(context.Context, string, time.Time) (db.PublicHoliday, error) {
	return db.PublicHoliday{}, f.e("CreateHoliday")
}
func (f *fakeStore) DeleteHoliday(context.Context, int64) error { return f.e("DeleteHoliday") }
func (f *fakeStore) GetSettings(context.Context) (db.CompanySetting, error) {
	// A Mon–Fri working week so the submit path computes > 0 working days and
	// reaches the store call each fault case actually targets.
	return db.CompanySetting{
		LeaveYearStartMonth: 1,
		WorkMonday:          true,
		WorkTuesday:         true,
		WorkWednesday:       true,
		WorkThursday:        true,
		WorkFriday:          true,
	}, f.e("GetSettings")
}
func (f *fakeStore) UpdateSettings(context.Context, string, int32, bool, bool, bool, bool, bool, bool, bool) error {
	return f.e("UpdateSettings")
}
func (f *fakeStore) Ping(context.Context) error { return f.e("Ping") }

func faultRouter(f *fakeStore) http.Handler {
	cfg := config.Config{SessionTTL: time.Hour}
	return server.New(handlers.New(f, cfg), api.New(f, cfg), f, cfg, testLogger(), noop.NewTracerProvider())
}

var authCookie = &http.Cookie{Name: auth.CookieName, Value: "x"}

func admin() db.Employee   { return db.Employee{ID: 1, Role: auth.RoleAdmin} }
func manager() db.Employee { return db.Employee{ID: 2, Role: auth.RoleManager} }

// validReqForm is a submit form whose dates land on a Mon–Wed so validation
// passes and control reaches the store call under test.
func validReqForm() url.Values {
	mon := nextWeekday(time.Now().AddDate(0, 0, 7), time.Monday)
	return form(
		"leave_type_id", "1",
		"start_date", ymd(mon),
		"end_date", ymd(mon.AddDate(0, 0, 2)),
		"reason", "x",
	)
}

func TestFaultBranches(t *testing.T) {
	// A request row owned by employee #99 (used by the decide/canDecide path).
	reqRow := db.GetLeaveRequestRow{ID: 5, EmployeeID: 99, Status: "pending"}
	// A target employee for the profile handler (a report of the manager #2).
	report := db.Employee{ID: 99, Role: auth.RoleEmployee, ManagerID: pgtype.Int8{Int64: 2, Valid: true}}

	cases := []struct {
		name   string
		emp    db.Employee
		target db.Employee
		req    db.GetLeaveRequestRow
		fail   []string
		method string
		path   string
		form   url.Values
		want   int
	}{
		// health probe
		{"health down", admin(), db.Employee{}, reqRow, []string{"Ping"}, "GET", "/healthz", nil, http.StatusServiceUnavailable},

		// login
		{"login create-session fails", loginEmp(t), db.Employee{}, reqRow, []string{"CreateSession"}, "POST", "/login",
			form("email", "a@b.test", "password", "password"), http.StatusInternalServerError},

		// dashboard
		{"dashboard settings fail", admin(), db.Employee{}, reqRow, []string{"GetSettings"}, "GET", "/", nil, http.StatusInternalServerError},
		{"dashboard balances fail", admin(), db.Employee{}, reqRow, []string{"ListBalances"}, "GET", "/", nil, http.StatusInternalServerError},
		{"dashboard requests fail", admin(), db.Employee{}, reqRow, []string{"ListRequestsByEmployee"}, "GET", "/", nil, http.StatusInternalServerError},

		// requests page
		{"requests list fail", admin(), db.Employee{}, reqRow, []string{"ListRequestsByEmployee"}, "GET", "/requests", nil, http.StatusInternalServerError},
		{"requests types fail", admin(), db.Employee{}, reqRow, []string{"ListLeaveTypes"}, "GET", "/requests", nil, http.StatusInternalServerError},

		// create request
		{"create settings fail", admin(), db.Employee{}, reqRow, []string{"GetSettings"}, "POST", "/requests", validReqForm(), http.StatusInternalServerError},
		{"create holidays fail", admin(), db.Employee{}, reqRow, []string{"ListHolidaysInRange"}, "POST", "/requests", validReqForm(), http.StatusInternalServerError},
		{"create insert fail", admin(), db.Employee{}, reqRow, []string{"CreateLeaveRequest"}, "POST", "/requests", validReqForm(), http.StatusInternalServerError},
		{"create reload fail", admin(), db.Employee{}, reqRow, []string{"ListRequestsByEmployee"}, "POST", "/requests", validReqForm(), http.StatusInternalServerError},

		// cancel request
		{"cancel fail", admin(), db.Employee{}, reqRow, []string{"CancelOwnRequest"}, "POST", "/requests/5/cancel", nil, http.StatusInternalServerError},
		{"cancel reload fail", admin(), db.Employee{}, reqRow, []string{"ListRequestsByEmployee"}, "POST", "/requests/5/cancel", nil, http.StatusInternalServerError},

		// approvals
		{"approvals list fail", manager(), db.Employee{}, reqRow, []string{"ListPendingForManager"}, "GET", "/approvals", nil, http.StatusInternalServerError},
		{"decide not found", manager(), db.Employee{}, reqRow, []string{"GetLeaveRequest"}, "POST", "/approvals/5/approve", nil, http.StatusNotFound},
		{"decide set-status fail (admin)", admin(), db.Employee{}, reqRow, []string{"SetRequestStatus"}, "POST", "/approvals/5/approve", nil, http.StatusInternalServerError},
		{"decide canDecide lookup fails", manager(), report, reqRow, []string{"GetEmployee"}, "POST", "/approvals/5/approve", nil, http.StatusForbidden},

		// employees
		{"employees list fail", manager(), db.Employee{}, reqRow, []string{"ListEmployees"}, "GET", "/employees", nil, http.StatusInternalServerError},
		{"profile not found", admin(), db.Employee{}, reqRow, []string{"GetEmployee"}, "GET", "/employees/99", nil, http.StatusNotFound},
		{"profile settings fail", admin(), report, reqRow, []string{"GetSettings"}, "GET", "/employees/99", nil, http.StatusInternalServerError},
		{"profile balances fail", admin(), report, reqRow, []string{"ListBalances"}, "GET", "/employees/99", nil, http.StatusInternalServerError},
		{"profile requests fail", admin(), report, reqRow, []string{"ListRequestsByEmployee"}, "GET", "/employees/99", nil, http.StatusInternalServerError},

		// admin page
		{"admin types fail", admin(), db.Employee{}, reqRow, []string{"ListLeaveTypes"}, "GET", "/admin", nil, http.StatusInternalServerError},
		{"admin holidays fail", admin(), db.Employee{}, reqRow, []string{"ListHolidays"}, "GET", "/admin", nil, http.StatusInternalServerError},
		{"admin employees fail", admin(), db.Employee{}, reqRow, []string{"ListEmployees"}, "GET", "/admin", nil, http.StatusInternalServerError},
		{"admin settings fail", admin(), db.Employee{}, reqRow, []string{"GetSettings"}, "GET", "/admin", nil, http.StatusInternalServerError},

		// admin mutations
		{"create leave-type fail", admin(), db.Employee{}, reqRow, []string{"CreateLeaveType"}, "POST", "/admin/leave-types", form("name", "Sick"), http.StatusInternalServerError},
		{"create leave-type reload fail", admin(), db.Employee{}, reqRow, []string{"ListLeaveTypes"}, "POST", "/admin/leave-types", form("name", "Sick"), http.StatusInternalServerError},
		{"create holiday fail", admin(), db.Employee{}, reqRow, []string{"CreateHoliday"}, "POST", "/admin/holidays", form("name", "X", "holiday_date", "2026-01-01"), http.StatusInternalServerError},
		{"create holiday reload fail", admin(), db.Employee{}, reqRow, []string{"ListHolidays"}, "POST", "/admin/holidays", form("name", "X", "holiday_date", "2026-01-01"), http.StatusInternalServerError},
		{"delete holiday fail", admin(), db.Employee{}, reqRow, []string{"DeleteHoliday"}, "POST", "/admin/holidays/3/delete", nil, http.StatusInternalServerError},
		{"delete holiday reload fail", admin(), db.Employee{}, reqRow, []string{"ListHolidays"}, "POST", "/admin/holidays/3/delete", nil, http.StatusInternalServerError},
		{"set allocation settings fail", admin(), db.Employee{}, reqRow, []string{"GetSettings"}, "POST", "/admin/allocations", form("employee_id", "1", "leave_type_id", "1", "days", "5"), http.StatusInternalServerError},
		{"set allocation fail", admin(), db.Employee{}, reqRow, []string{"UpsertAllocation"}, "POST", "/admin/allocations", form("employee_id", "1", "leave_type_id", "1", "days", "5"), http.StatusInternalServerError},
		{"save settings fail", admin(), db.Employee{}, reqRow, []string{"UpdateSettings"}, "POST", "/admin/settings", form("name", "Acme", "leave_year_start_month", "4"), http.StatusInternalServerError},
		{"save settings bad month", admin(), db.Employee{}, reqRow, nil, "POST", "/admin/settings", form("name", "Acme", "leave_year_start_month", "13"), http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeStore{emp: tc.emp, target: tc.target, req: tc.req, fail: map[string]bool{}}
			for _, name := range tc.fail {
				f.fail[name] = true
			}
			w := do(faultRouter(f), tc.method, tc.path, tc.form, authCookie)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// loginEmp returns an employee whose password hash matches "password", so the
// Login handler's credential check passes and control reaches CreateSession.
func loginEmp(t *testing.T) db.Employee {
	t.Helper()
	hash, err := auth.HashPassword("password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return db.Employee{ID: 1, Role: auth.RoleEmployee, PasswordHash: hash}
}
