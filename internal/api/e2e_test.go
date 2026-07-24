// End-to-end tests for the JSON REST API, driven through the fully wired router
// (server.New) against a throwaway Postgres (internal/testsupport). They mirror
// the HTML e2e suite in internal/handlers, but speak JSON + bearer tokens: every
// request carries "Authorization: Bearer <token>" instead of a session cookie,
// and every assertion is on a status code + JSON body rather than rendered HTML.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

func init() { gin.SetMode(gin.TestMode) }

// harness is a fully wired app (real router + real Postgres) with the same
// seeded org the HTML suite uses:
//
//	admin ─┬─ manager ─┬─ alice   (employee, has an Annual allocation)
//	       │           └─ bob     (employee)
//	       └─ olivia   (employee, reports to admin, NOT to manager)
//
// A bearer token per principal is pre-issued so tests can drive any route.
type harness struct {
	t      *testing.T
	store  *store.Store
	router *gin.Engine

	adminID, managerID, aliceID, bobID, oliviaID int64
	annualID                                      int64

	adminTok, managerTok, aliceTok, oliviaTok string
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
	h.oliviaID = mk("Olivia", "olivia@e2e.test", auth.RoleEmployee, &h.adminID)

	lt, err := st.CreateLeaveType(ctx, "Annual", 25, "#6366f1")
	if err != nil {
		t.Fatalf("leave type: %v", err)
	}
	h.annualID = lt.ID
	if _, err := st.UpsertAllocation(ctx, h.aliceID, h.annualID, int32(time.Now().Year()), 25); err != nil {
		t.Fatalf("allocation: %v", err)
	}

	h.adminTok = h.token(h.adminID)
	h.managerTok = h.token(h.managerID)
	h.aliceTok = h.token(h.aliceID)
	h.oliviaTok = h.token(h.oliviaID)
	return h
}

// token issues a real session row and returns its bearer token — the same
// Postgres-backed session the web app uses, just carried in a header.
func (h *harness) token(empID int64) string {
	h.t.Helper()
	tok, err := auth.NewToken()
	if err != nil {
		h.t.Fatalf("token: %v", err)
	}
	if _, err := h.store.CreateSession(context.Background(), tok, empID, time.Now().Add(time.Hour)); err != nil {
		h.t.Fatalf("create session: %v", err)
	}
	return tok
}

// ─────────────────────────────── helpers ───────────────────────────────

// doJSON sends a JSON request (body marshalled if non-nil) with an optional
// bearer token and returns the recorder. An empty token omits the header.
func (h *harness) doJSON(method, path string, body any, token string) *httptest.ResponseRecorder {
	h.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func mustStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, want, w.Body.String())
	}
}

// decode unmarshals the response body into v, failing the test on error.
func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return v
}

func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	d := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	for d.Weekday() != wd {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// ─────────────────────────────── auth ───────────────────────────────

func TestLoginAndMe(t *testing.T) {
	h := setup(t)

	// Wrong password and unknown email both fail with the same generic 401.
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "alice@e2e.test", "password": "nope"}, ""), http.StatusUnauthorized)
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "ghost@e2e.test", "password": "password"}, ""), http.StatusUnauthorized)

	// Correct credentials return a token + the caller's profile.
	w := h.doJSON(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "alice@e2e.test", "password": "password"}, "")
	mustStatus(t, w, http.StatusOK)
	login := decode[struct {
		Token string      `json:"token"`
		User  api.UserDTO `json:"user"`
	}](t, w)
	if login.Token == "" {
		t.Fatal("login returned an empty token")
	}
	if login.User.ID != h.aliceID || login.User.Email != "alice@e2e.test" {
		t.Fatalf("login user = %+v, want alice (%d)", login.User, h.aliceID)
	}

	// The freshly issued token authorises /me.
	me := decode[api.UserDTO](t, mustOK(t, h.doJSON(http.MethodGet, "/api/v1/me", nil, login.Token)))
	if me.ID != h.aliceID {
		t.Fatalf("/me id = %d, want %d", me.ID, h.aliceID)
	}
}

func TestBearerAuthFailures(t *testing.T) {
	h := setup(t)
	// Missing header, malformed header, and a garbage token all 401.
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/me", nil, ""), http.StatusUnauthorized)
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/me", nil, "not-a-real-token"), http.StatusUnauthorized)

	// A raw (non-Bearer) Authorization header is rejected too.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", h.aliceTok) // no "Bearer " prefix
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	mustStatus(t, w, http.StatusUnauthorized)
}

func TestLogoutRevokesToken(t *testing.T) {
	h := setup(t)
	tok := h.token(h.aliceID)

	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/me", nil, tok), http.StatusOK)
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/auth/logout", nil, tok), http.StatusNoContent)
	// After logout the session row is gone, so the token no longer resolves.
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/me", nil, tok), http.StatusUnauthorized)
}

// ─────────────────────────── requests lifecycle ───────────────────────────

func TestRequestLifecycle(t *testing.T) {
	h := setup(t)
	mon := nextWeekday(time.Now().AddDate(0, 0, 7), time.Monday)

	// Create: Mon–Wed is 3 working days on a Mon–Fri week.
	w := h.doJSON(http.MethodPost, "/api/v1/requests", map[string]any{
		"leave_type_id": h.annualID,
		"start_date":    ymd(mon),
		"end_date":      ymd(mon.AddDate(0, 0, 2)),
		"reason":        "trip",
	}, h.aliceTok)
	mustStatus(t, w, http.StatusCreated)
	created := decode[struct {
		ID          int64   `json:"id"`
		WorkingDays float64 `json:"working_days"`
		Status      string  `json:"status"`
	}](t, w)
	if created.WorkingDays != 3 {
		t.Fatalf("working_days = %v, want 3", created.WorkingDays)
	}
	if created.Status != "pending" {
		t.Fatalf("status = %q, want pending", created.Status)
	}

	// List: the new request shows up for its owner.
	list := decode[[]api.RequestDTO](t, mustOK(t, h.doJSON(http.MethodGet, "/api/v1/requests", nil, h.aliceTok)))
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("my requests = %+v, want the one just created", list)
	}

	// Cancel: idempotent 204, and the request drops out of the pending set.
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/requests/"+itoa(created.ID)+"/cancel", nil, h.aliceTok), http.StatusNoContent)
	after := decode[[]api.RequestDTO](t, mustOK(t, h.doJSON(http.MethodGet, "/api/v1/requests", nil, h.aliceTok)))
	if len(after) != 1 || after[0].Status != "cancelled" {
		t.Fatalf("after cancel = %+v, want a single cancelled request", after)
	}
}

func TestCreateRequestValidation(t *testing.T) {
	h := setup(t)
	mon := nextWeekday(time.Now().AddDate(0, 0, 7), time.Monday)

	// end before start.
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/requests", map[string]any{
		"leave_type_id": h.annualID, "start_date": ymd(mon), "end_date": ymd(mon.AddDate(0, 0, -1)),
	}, h.aliceTok), http.StatusBadRequest)

	// Weekend-only range → no working days.
	sat := nextWeekday(time.Now().AddDate(0, 0, 7), time.Saturday)
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/requests", map[string]any{
		"leave_type_id": h.annualID, "start_date": ymd(sat), "end_date": ymd(sat),
	}, h.aliceTok), http.StatusBadRequest)

	// Missing required field.
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/requests", map[string]any{
		"start_date": ymd(mon), "end_date": ymd(mon),
	}, h.aliceTok), http.StatusBadRequest)
}

// ─────────────────────────── approvals + roles ───────────────────────────

func TestApprovalsFlow(t *testing.T) {
	h := setup(t)
	mon := nextWeekday(time.Now().AddDate(0, 0, 7), time.Monday)

	// Alice submits, then her manager approves it.
	w := h.doJSON(http.MethodPost, "/api/v1/requests", map[string]any{
		"leave_type_id": h.annualID, "start_date": ymd(mon), "end_date": ymd(mon),
	}, h.aliceTok)
	mustStatus(t, w, http.StatusCreated)
	reqID := decode[struct {
		ID int64 `json:"id"`
	}](t, w).ID

	pending := decode[[]api.PendingDTO](t, mustOK(t, h.doJSON(http.MethodGet, "/api/v1/approvals", nil, h.managerTok)))
	if len(pending) != 1 || pending[0].ID != reqID {
		t.Fatalf("manager approvals = %+v, want alice's request %d", pending, reqID)
	}

	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/approvals/"+itoa(reqID)+"/approve", nil, h.managerTok), http.StatusOK)

	// A manager who isn't her manager (admin manages Olivia, not Alice's report
	// chain here — Olivia's request) can't act on someone else's report.
	ow := h.doJSON(http.MethodPost, "/api/v1/requests", map[string]any{
		"leave_type_id": h.annualID, "start_date": ymd(mon), "end_date": ymd(mon),
	}, h.oliviaTok)
	mustStatus(t, ow, http.StatusCreated)
	oReq := decode[struct {
		ID int64 `json:"id"`
	}](t, ow).ID
	// manager is NOT Olivia's manager → 403.
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/approvals/"+itoa(oReq)+"/reject", nil, h.managerTok), http.StatusForbidden)
}

func TestRoleGates(t *testing.T) {
	h := setup(t)

	// A plain employee is denied the manager-scoped and admin-scoped trees.
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/approvals", nil, h.aliceTok), http.StatusForbidden)
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/employees", nil, h.aliceTok), http.StatusForbidden)
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/admin/settings", nil, h.aliceTok), http.StatusForbidden)

	// A manager reaches approvals/employees but not admin.
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/employees", nil, h.managerTok), http.StatusOK)
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/admin/settings", nil, h.managerTok), http.StatusForbidden)

	// Admin reaches everything.
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/admin/settings", nil, h.adminTok), http.StatusOK)
}

func TestEmployeeProfileScope(t *testing.T) {
	h := setup(t)

	// Manager may open a direct report (Alice)…
	prof := decode[struct {
		Employee api.UserDTO `json:"employee"`
	}](t, mustOK(t, h.doJSON(http.MethodGet, "/api/v1/employees/"+itoa(h.aliceID), nil, h.managerTok)))
	if prof.Employee.ID != h.aliceID {
		t.Fatalf("profile id = %d, want %d", prof.Employee.ID, h.aliceID)
	}
	// …but not someone who isn't their report (Olivia reports to admin).
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/employees/"+itoa(h.oliviaID), nil, h.managerTok), http.StatusForbidden)
	// Admin may open anyone.
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/employees/"+itoa(h.oliviaID), nil, h.adminTok), http.StatusOK)
}

// ─────────────────────────── admin mutations ───────────────────────────

func TestAdminMutations(t *testing.T) {
	h := setup(t)

	// Balances read cleanly for the current leave year.
	mustStatus(t, h.doJSON(http.MethodGet, "/api/v1/me/balances", nil, h.aliceTok), http.StatusOK)

	// Create a leave type.
	w := h.doJSON(http.MethodPost, "/api/v1/admin/leave-types", map[string]any{
		"name": "Sick", "default_days": 10, "color": "#ef4444",
	}, h.adminTok)
	mustStatus(t, w, http.StatusCreated)
	lt := decode[api.LeaveTypeDTO](t, w)
	if lt.Name != "Sick" || lt.ID == 0 {
		t.Fatalf("created leave type = %+v", lt)
	}

	// Create then delete a holiday.
	hw := h.doJSON(http.MethodPost, "/api/v1/admin/holidays", map[string]any{
		"name": "New Year", "date": "2026-01-01",
	}, h.adminTok)
	mustStatus(t, hw, http.StatusCreated)
	hol := decode[api.HolidayDTO](t, hw)
	mustStatus(t, h.doJSON(http.MethodDelete, "/api/v1/admin/holidays/"+itoa(hol.ID), nil, h.adminTok), http.StatusNoContent)

	// Upsert an allocation for Bob.
	mustStatus(t, h.doJSON(http.MethodPost, "/api/v1/admin/allocations", map[string]any{
		"employee_id": h.bobID, "leave_type_id": h.annualID, "days": 20,
	}, h.adminTok), http.StatusNoContent)

	// Replace settings via PUT, then read them back.
	mustStatus(t, h.doJSON(http.MethodPut, "/api/v1/admin/settings", api.SettingsDTO{
		Name: "Acme", LeaveYearStartMonth: 4,
		WorkMonday: true, WorkTuesday: true, WorkWednesday: true, WorkThursday: true, WorkFriday: true,
	}, h.adminTok), http.StatusNoContent)
	got := decode[api.SettingsDTO](t, mustOK(t, h.doJSON(http.MethodGet, "/api/v1/admin/settings", nil, h.adminTok)))
	if got.Name != "Acme" || got.LeaveYearStartMonth != 4 {
		t.Fatalf("settings after PUT = %+v", got)
	}

	// Out-of-range month is rejected.
	mustStatus(t, h.doJSON(http.MethodPut, "/api/v1/admin/settings", api.SettingsDTO{
		Name: "Acme", LeaveYearStartMonth: 13,
	}, h.adminTok), http.StatusBadRequest)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// mustOK asserts a 200 and returns the recorder so it can be chained into decode.
func mustOK(t *testing.T, w *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	mustStatus(t, w, http.StatusOK)
	return w
}
