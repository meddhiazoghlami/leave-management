package handlers_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/api"
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/handlers"
	"github.com/meddhiazoghlami/leave-management/internal/server"
	"github.com/meddhiazoghlami/leave-management/internal/store"
	"github.com/meddhiazoghlami/leave-management/internal/testsupport"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace/noop"
)

// testLogger discards output; the obs middleware just needs a non-nil logger.
func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// harness is a fully wired app (real router + real Postgres) with a seeded org:
//
//	admin ─┬─ manager ─┬─ alice   (employee, has an Annual allocation)
//	       │           └─ bob     (employee)
//	       └─ outsider (employee, reports to admin, NOT to manager)
//
// Cookies for each principal are pre-created so tests can drive any route.
type harness struct {
	t      *testing.T
	store  *store.Store
	router *gin.Engine

	adminID, managerID, aliceID, bobID, outsiderID int64
	annualID                                       int64

	admin, manager, alice, outsider *http.Cookie
}

func setup(t *testing.T) *harness {
	st := testsupport.NewStore(t)
	cfg := config.Config{SessionTTL: time.Hour}
	r := server.New(handlers.New(st, cfg), api.New(st, cfg), st, cfg, testLogger(), noop.NewTracerProvider())
	h := &harness{t: t, store: st, router: r}

	ctx := context.Background()
	hash, err := auth.HashPassword("password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	mk := func(name, email, role string, mgr *int64) int64 {
		emp, err := st.CreateEmployee(ctx, name, email, hash, role, mgr)
		if err != nil {
			t.Fatalf("create %s: %v", email, err)
		}
		return emp.ID
	}

	h.adminID = mk("Admin", "admin@e2e.test", auth.RoleAdmin, nil)
	h.managerID = mk("Manager", "mgr@e2e.test", auth.RoleManager, &h.adminID)
	h.aliceID = mk("Alice", "alice@e2e.test", auth.RoleEmployee, &h.managerID)
	h.bobID = mk("Bob", "bob@e2e.test", auth.RoleEmployee, &h.managerID)
	h.outsiderID = mk("Olivia", "olivia@e2e.test", auth.RoleEmployee, &h.adminID)

	lt, err := st.CreateLeaveType(ctx, "Annual", 25, "#6366f1")
	if err != nil {
		t.Fatalf("leave type: %v", err)
	}
	h.annualID = lt.ID
	if _, err := st.UpsertAllocation(ctx, h.aliceID, h.annualID, int32(time.Now().Year()), 25); err != nil {
		t.Fatalf("allocation: %v", err)
	}

	h.admin = h.login(h.adminID)
	h.manager = h.login(h.managerID)
	h.alice = h.login(h.aliceID)
	h.outsider = h.login(h.outsiderID)
	return h
}

func (h *harness) login(empID int64) *http.Cookie {
	h.t.Helper()
	tok, err := auth.NewToken()
	if err != nil {
		h.t.Fatalf("token: %v", err)
	}
	if _, err := h.store.CreateSession(context.Background(), tok, empID, time.Now().Add(time.Hour)); err != nil {
		h.t.Fatalf("create session: %v", err)
	}
	return &http.Cookie{Name: auth.CookieName, Value: tok}
}

// submitRequest is a helper that creates a pending Annual request owned by
// `owner` (via `cookie`) and returns its id by reading it back from the store.
func (h *harness) submitAnnual(owner int64, cookie *http.Cookie) int64 {
	h.t.Helper()
	mon := nextWeekday(time.Now().AddDate(0, 0, 7), time.Monday)
	f := form(
		"leave_type_id", itoa(h.annualID),
		"start_date", ymd(mon),
		"end_date", ymd(mon.AddDate(0, 0, 2)),
		"reason", "trip",
	)
	w := do(h.router, http.MethodPost, "/requests", f, cookie)
	mustStatus(h.t, w, http.StatusOK)

	reqs, err := h.store.ListRequestsByEmployee(context.Background(), owner)
	if err != nil || len(reqs) == 0 {
		h.t.Fatalf("expected a request for %d, err %v", owner, err)
	}
	return reqs[0].ID
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// ─────────────────────────────── public routes ───────────────────────────────

func TestHealthz(t *testing.T) {
	h := setup(t)
	w := do(h.router, http.MethodGet, "/healthz", nil)
	mustStatus(t, w, http.StatusOK)
	if w.Body.String() != "ok" {
		t.Fatalf("body = %q, want ok", w.Body.String())
	}
}

func TestLoginFlow(t *testing.T) {
	h := setup(t)

	// GET login page.
	mustStatus(t, do(h.router, http.MethodGet, "/login", nil), http.StatusOK)

	// Wrong password: 200 re-render, no session cookie.
	bad := do(h.router, http.MethodPost, "/login",
		form("email", "alice@e2e.test", "password", "nope"))
	mustStatus(t, bad, http.StatusOK)
	if hasSessionCookie(bad) {
		t.Fatal("wrong password should not set a session cookie")
	}

	// Unknown email: same generic failure.
	mustStatus(t, do(h.router, http.MethodPost, "/login",
		form("email", "ghost@e2e.test", "password", "password")), http.StatusOK)

	// Correct credentials: 303 + a session cookie that then authorises "/".
	ok := do(h.router, http.MethodPost, "/login",
		form("email", "alice@e2e.test", "password", "password"))
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", ok.Code)
	}
	if !hasSessionCookie(ok) {
		t.Fatal("successful login should set a session cookie")
	}
	var session *http.Cookie
	for _, ck := range ok.Result().Cookies() {
		if ck.Name == auth.CookieName {
			session = ck
		}
	}
	mustStatus(t, do(h.router, http.MethodGet, "/", nil, session), http.StatusOK)
}

func TestLogout(t *testing.T) {
	h := setup(t)
	w := do(h.router, http.MethodPost, "/logout", nil, h.alice)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want 303", w.Code)
	}
	// Cookie is cleared.
	var cleared bool
	for _, ck := range w.Result().Cookies() {
		if ck.Name == auth.CookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logout should clear the session cookie")
	}
	// /logout is auth-gated: without a session cookie RequireAuth bounces to
	// /login (302) before the handler runs.
	if w2 := do(h.router, http.MethodPost, "/logout", nil); w2.Code != http.StatusFound {
		t.Fatalf("logout without cookie = %d, want 302 (bounced by RequireAuth)", w2.Code)
	}
}

// ──────────────────────────── authenticated pages ────────────────────────────

func TestAuthenticatedPages(t *testing.T) {
	h := setup(t)
	pages := []struct {
		path   string
		cookie *http.Cookie
	}{
		{"/", h.alice},
		{"/requests", h.alice},
		{"/calendar", h.alice},
		{"/calendar/month?year=2026&month=7", h.alice},
		{"/calendar/month?month=99", h.alice}, // invalid month clamps
		{"/calendar/month", h.alice},          // no params -> current month
		{"/approvals", h.manager},
		{"/employees", h.manager},
		{"/employees", h.admin}, // admin sees everyone
		{"/admin", h.admin},
	}
	for _, p := range pages {
		t.Run(p.path, func(t *testing.T) {
			mustStatus(t, do(h.router, http.MethodGet, p.path, nil, p.cookie), http.StatusOK)
		})
	}
}

func TestDashboardTrimsRecentToFive(t *testing.T) {
	h := setup(t)
	// Six requests -> dashboard should still render (and trim to 5 internally).
	ctx := context.Background()
	start := nextWeekday(time.Now().AddDate(0, 0, 30), time.Monday)
	for i := range 6 {
		d := start.AddDate(0, 0, i*7)
		if _, err := h.store.CreateLeaveRequest(ctx, h.aliceID, h.annualID, d, d, 1, "x"); err != nil {
			t.Fatalf("seed request %d: %v", i, err)
		}
	}
	mustStatus(t, do(h.router, http.MethodGet, "/", nil, h.alice), http.StatusOK)
}

func TestCalendarShowsApprovedLeaveAndHoliday(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	now := time.Now()

	// An approved 3-day request mid-month, so day cells carry entries and the
	// inRange overlap check fires.
	d := time.Date(now.Year(), now.Month(), 15, 0, 0, 0, 0, time.UTC)
	req, err := h.store.CreateLeaveRequest(ctx, h.aliceID, h.annualID, d, d.AddDate(0, 0, 2), 3, "x")
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}
	if err := h.store.SetRequestStatus(ctx, req.ID, "approved", h.managerID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// A holiday in the same month so the holiday annotation branch renders.
	if _, err := h.store.CreateHoliday(ctx, "Mid-Month", time.Date(now.Year(), now.Month(), 10, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("seed holiday: %v", err)
	}

	mustStatus(t, do(h.router, http.MethodGet, "/calendar", nil, h.alice), http.StatusOK)
	frag := "/calendar/month?year=" + itoa(int64(now.Year())) + "&month=" + itoa(int64(now.Month()))
	mustStatus(t, do(h.router, http.MethodGet, frag, nil, h.alice), http.StatusOK)
}

// ───────────────────────────── request lifecycle ─────────────────────────────

func TestCreateRequest_Validation(t *testing.T) {
	h := setup(t)
	mon := nextWeekday(time.Now().AddDate(0, 0, 7), time.Monday)
	sat := nextWeekday(time.Now().AddDate(0, 0, 7), time.Saturday)
	sun := nextWeekday(sat, time.Sunday)

	cases := []struct {
		name string
		f    map[string]string
		code int
	}{
		{"ok", map[string]string{"leave_type_id": itoa(h.annualID), "start_date": ymd(mon), "end_date": ymd(mon.AddDate(0, 0, 2)), "reason": "trip"}, http.StatusOK},
		{"bad type", map[string]string{"leave_type_id": "abc", "start_date": ymd(mon), "end_date": ymd(mon)}, http.StatusBadRequest},
		{"bad dates", map[string]string{"leave_type_id": itoa(h.annualID), "start_date": "nope", "end_date": "nope"}, http.StatusBadRequest},
		{"end before start", map[string]string{"leave_type_id": itoa(h.annualID), "start_date": ymd(mon.AddDate(0, 0, 2)), "end_date": ymd(mon)}, http.StatusBadRequest},
		{"weekend only", map[string]string{"leave_type_id": itoa(h.annualID), "start_date": ymd(sat), "end_date": ymd(sun)}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := form()
			for k, v := range tc.f {
				f.Set(k, v)
			}
			mustStatus(t, do(h.router, http.MethodPost, "/requests", f, h.alice), tc.code)
		})
	}
}

func TestCreateRequest_WithHolidayInRange(t *testing.T) {
	h := setup(t)
	mon := nextWeekday(time.Now().AddDate(0, 0, 7), time.Monday)
	// A holiday on the Tuesday of the range exercises the holiday-subtraction
	// loop; the request still has working days (Mon + Wed) so it succeeds.
	if _, err := h.store.CreateHoliday(context.Background(), "Mid", mon.AddDate(0, 0, 1)); err != nil {
		t.Fatalf("seed holiday: %v", err)
	}
	f := form(
		"leave_type_id", itoa(h.annualID),
		"start_date", ymd(mon),
		"end_date", ymd(mon.AddDate(0, 0, 2)),
		"reason", "x",
	)
	mustStatus(t, do(h.router, http.MethodPost, "/requests", f, h.alice), http.StatusOK)

	reqs, _ := h.store.ListRequestsByEmployee(context.Background(), h.aliceID)
	if len(reqs) != 1 || reqs[0].WorkingDays != 2 {
		t.Fatalf("expected 1 request with 2 working days (holiday excluded), got %+v", reqs)
	}
}

// TestSettings_FriSatWeekend is the M1 acceptance test: after the admin switches
// the working week to Sun–Thu (a Fri/Sat weekend), a Sun–Thu leave request must
// count 5 working days rather than the Mon–Fri default's 4.
func TestSettings_FriSatWeekend(t *testing.T) {
	h := setup(t)

	// Unchecked weekday boxes are simply absent from the POST, so we send only
	// the five working days (Sun–Thu); Fri/Sat are the weekend.
	settings := form(
		"name", "Acme",
		"leave_year_start_month", "1",
		"work_sunday", "on",
		"work_monday", "on",
		"work_tuesday", "on",
		"work_wednesday", "on",
		"work_thursday", "on",
	)
	mustStatus(t, do(h.router, http.MethodPost, "/admin/settings", settings, h.admin), http.StatusOK)

	sun := nextWeekday(time.Now().AddDate(0, 0, 7), time.Sunday)
	req := form(
		"leave_type_id", itoa(h.annualID),
		"start_date", ymd(sun),
		"end_date", ymd(sun.AddDate(0, 0, 4)), // the following Thursday
		"reason", "x",
	)
	mustStatus(t, do(h.router, http.MethodPost, "/requests", req, h.alice), http.StatusOK)

	reqs, _ := h.store.ListRequestsByEmployee(context.Background(), h.aliceID)
	if len(reqs) != 1 || reqs[0].WorkingDays != 5 {
		t.Fatalf("expected 1 request with 5 working days (Sun–Thu, Fri/Sat weekend), got %+v", reqs)
	}
}

func TestCancelRequest(t *testing.T) {
	h := setup(t)
	id := h.submitAnnual(h.aliceID, h.alice)

	// Bad id.
	mustStatus(t, do(h.router, http.MethodPost, "/requests/abc/cancel", nil, h.alice), http.StatusBadRequest)

	// Owner cancels -> 200; the request becomes cancelled.
	mustStatus(t, do(h.router, http.MethodPost, "/requests/"+itoa(id)+"/cancel", nil, h.alice), http.StatusOK)
	got, _ := h.store.GetLeaveRequest(context.Background(), id)
	if got.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
}

func TestRequestsRenderAllStatuses(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	base := nextWeekday(time.Now().AddDate(0, 0, 40), time.Monday)

	// Four requests, one driven to each status, so the list renders every
	// status badge (pending/approved/rejected/cancelled).
	mk := func(week int) int64 {
		d := base.AddDate(0, 0, week*7)
		r, err := h.store.CreateLeaveRequest(ctx, h.aliceID, h.annualID, d, d, 1, "x")
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return r.ID
	}
	_ = mk(0) // stays pending
	if err := h.store.SetRequestStatus(ctx, mk(1), "approved", h.managerID); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetRequestStatus(ctx, mk(2), "rejected", h.managerID); err != nil {
		t.Fatal(err)
	}
	if err := h.store.CancelOwnRequest(ctx, mk(3), h.aliceID); err != nil {
		t.Fatal(err)
	}

	mustStatus(t, do(h.router, http.MethodGet, "/requests", nil, h.alice), http.StatusOK)
	// The dashboard renders the balance cards + recent list too.
	mustStatus(t, do(h.router, http.MethodGet, "/", nil, h.alice), http.StatusOK)
}

// ─────────────────────────────── approvals ───────────────────────────────────

func TestApprovalDecisions(t *testing.T) {
	h := setup(t)
	id := h.submitAnnual(h.aliceID, h.alice)

	// Manager's approvals page now renders a pending card for the report.
	mustStatus(t, do(h.router, http.MethodGet, "/approvals", nil, h.manager), http.StatusOK)

	// Employee cannot reach approvals at all (role gate).
	mustStatus(t, do(h.router, http.MethodGet, "/approvals", nil, h.alice), http.StatusForbidden)

	// Bad id -> 400.
	mustStatus(t, do(h.router, http.MethodPost, "/approvals/abc/approve", nil, h.manager), http.StatusBadRequest)
	// Unknown id -> 404.
	mustStatus(t, do(h.router, http.MethodPost, "/approvals/99999/approve", nil, h.manager), http.StatusNotFound)

	// Manager approves their report -> 200, status flips to approved.
	mustStatus(t, do(h.router, http.MethodPost, "/approvals/"+itoa(id)+"/approve", nil, h.manager), http.StatusOK)
	got, _ := h.store.GetLeaveRequest(context.Background(), id)
	if got.Status != "approved" {
		t.Fatalf("status = %q, want approved", got.Status)
	}
}

func TestApproval_NotYourReport(t *testing.T) {
	h := setup(t)
	// Olivia (reports to admin) submits; the manager is NOT her manager.
	id := h.submitAnnual(h.outsiderID, h.outsider)
	mustStatus(t, do(h.router, http.MethodPost, "/approvals/"+itoa(id)+"/reject", nil, h.manager), http.StatusForbidden)

	// Admin may decide anyone.
	mustStatus(t, do(h.router, http.MethodPost, "/approvals/"+itoa(id)+"/reject", nil, h.admin), http.StatusOK)
}

// ─────────────────────────────── employees ───────────────────────────────────

func TestEmployeeProfile(t *testing.T) {
	h := setup(t)

	// Manager opens their report -> 200.
	mustStatus(t, do(h.router, http.MethodGet, "/employees/"+itoa(h.aliceID), nil, h.manager), http.StatusOK)
	// Admin opens anyone -> 200.
	mustStatus(t, do(h.router, http.MethodGet, "/employees/"+itoa(h.outsiderID), nil, h.admin), http.StatusOK)
	// Manager opens a non-report -> 403.
	mustStatus(t, do(h.router, http.MethodGet, "/employees/"+itoa(h.outsiderID), nil, h.manager), http.StatusForbidden)
	// Bad id -> 400.
	mustStatus(t, do(h.router, http.MethodGet, "/employees/abc", nil, h.manager), http.StatusBadRequest)
	// Unknown id -> 404.
	mustStatus(t, do(h.router, http.MethodGet, "/employees/99999", nil, h.admin), http.StatusNotFound)
}

// ────────────────────────────── admin section ────────────────────────────────

func TestAdminRoleGate(t *testing.T) {
	h := setup(t)
	// Admin reaches the page and it renders (incl. the new general-settings form).
	mustStatus(t, do(h.router, http.MethodGet, "/admin", nil, h.admin), http.StatusOK)
	// Manager is blocked from /admin (admin-only).
	mustStatus(t, do(h.router, http.MethodGet, "/admin", nil, h.manager), http.StatusForbidden)
	// Employee too.
	mustStatus(t, do(h.router, http.MethodGet, "/admin", nil, h.alice), http.StatusForbidden)
}

// TestSaveSettings_Persists round-trips the general settings through the handler
// and the store, and confirms a bad month is rejected.
func TestSaveSettings_Persists(t *testing.T) {
	h := setup(t)
	mustStatus(t, do(h.router, http.MethodPost, "/admin/settings",
		form("name", "Globex", "leave_year_start_month", "4",
			"work_monday", "on", "work_tuesday", "on", "work_wednesday", "on",
			"work_thursday", "on", "work_friday", "on"), h.admin), http.StatusOK)

	s, err := h.store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.Name != "Globex" || s.LeaveYearStartMonth != 4 {
		t.Fatalf("settings = %+v, want name Globex, month 4", s)
	}
	if !s.WorkFriday || s.WorkSaturday || s.WorkSunday {
		t.Fatalf("working week not saved as Mon–Fri: %+v", s)
	}

	// A bad month is rejected (400), leaving the saved value untouched.
	mustStatus(t, do(h.router, http.MethodPost, "/admin/settings",
		form("name", "Globex", "leave_year_start_month", "0"), h.admin), http.StatusBadRequest)
	if s2, _ := h.store.GetSettings(context.Background()); s2.LeaveYearStartMonth != 4 {
		t.Fatalf("bad month clobbered saved value: month = %d, want 4", s2.LeaveYearStartMonth)
	}
}

func TestAdminMutations(t *testing.T) {
	h := setup(t)
	ctx := context.Background()

	// Create leave type: empty name -> 400; valid -> 200.
	mustStatus(t, do(h.router, http.MethodPost, "/admin/leave-types", form("name", ""), h.admin), http.StatusBadRequest)
	mustStatus(t, do(h.router, http.MethodPost, "/admin/leave-types",
		form("name", "Sick", "default_days", "12", "color", ""), h.admin), http.StatusOK)

	// Create holiday: bad date -> 400; valid -> 200.
	mustStatus(t, do(h.router, http.MethodPost, "/admin/holidays",
		form("name", "Bad", "holiday_date", "not-a-date"), h.admin), http.StatusBadRequest)
	mustStatus(t, do(h.router, http.MethodPost, "/admin/holidays",
		form("name", "New Year", "holiday_date", "2026-01-01"), h.admin), http.StatusOK)

	holidays, _ := h.store.ListHolidays(ctx)
	if len(holidays) != 1 {
		t.Fatalf("holidays = %d, want 1", len(holidays))
	}

	// Delete holiday: bad id -> 400; valid -> 200.
	mustStatus(t, do(h.router, http.MethodPost, "/admin/holidays/abc/delete", nil, h.admin), http.StatusBadRequest)
	mustStatus(t, do(h.router, http.MethodPost, "/admin/holidays/"+itoa(holidays[0].ID)+"/delete", nil, h.admin), http.StatusOK)

	// Set allocation: incomplete -> 400; valid -> 200.
	mustStatus(t, do(h.router, http.MethodPost, "/admin/allocations",
		form("employee_id", "", "leave_type_id", "", "days", ""), h.admin), http.StatusBadRequest)
	mustStatus(t, do(h.router, http.MethodPost, "/admin/allocations",
		form("employee_id", itoa(h.bobID), "leave_type_id", itoa(h.annualID), "days", "10"), h.admin), http.StatusOK)
}

// ───────────────────────── unauthenticated redirect ──────────────────────────

func TestUnauthenticatedRedirect(t *testing.T) {
	// No DB needed: RequireAuth short-circuits before any store call.
	h := handlers.New(nil, config.Config{})
	r := server.New(h, nil, nil, config.Config{}, testLogger(), noop.NewTracerProvider())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login" {
		t.Fatalf("status %d loc %q, want 302 -> /login", w.Code, w.Header().Get("Location"))
	}
}
