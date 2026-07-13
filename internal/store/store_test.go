package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/internal/leave"
	"github.com/meddhiazoghlami/leave-management/internal/store"
	"github.com/meddhiazoghlami/leave-management/internal/testsupport"
)

// These are integration tests: they run against a real Postgres started by
// testcontainers (see internal/testsupport) with the actual migrations applied,
// so they exercise the sqlc-generated queries and the pgtype translations in
// package store together. They are skipped automatically when Docker isn't
// available.

func ctx() context.Context { return context.Background() }

// mkEmployee is a small helper that fails the test on error.
func mkEmployee(t *testing.T, st *store.Store, name, email, role string, mgr *int64) int64 {
	t.Helper()
	emp, err := st.CreateEmployee(ctx(), name, email, "hash", role, mgr)
	if err != nil {
		t.Fatalf("CreateEmployee(%s): %v", email, err)
	}
	return emp.ID
}

func mkLeaveType(t *testing.T, st *store.Store, name string, days int32) int64 {
	t.Helper()
	lt, err := st.CreateLeaveType(ctx(), name, days, "#123456")
	if err != nil {
		t.Fatalf("CreateLeaveType(%s): %v", name, err)
	}
	return lt.ID
}

func TestNew_BadURL(t *testing.T) {
	// New should fail fast on an unreachable/invalid DSN (Ping fails).
	if _, err := store.New(ctx(), "postgres://user:pass@127.0.0.1:1/nope?sslmode=disable&connect_timeout=1"); err == nil {
		t.Fatal("expected error connecting to a bogus database")
	}
	// A DSN that doesn't even parse should error too.
	if _, err := store.New(ctx(), "://not-a-dsn"); err == nil {
		t.Fatal("expected error parsing a malformed DSN")
	}
}

func TestPing(t *testing.T) {
	st := testsupport.NewStore(t)
	if err := st.Ping(ctx()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClose(t *testing.T) {
	// Open a dedicated store (not the shared one) so closing it is safe, and
	// confirm Close tears the pool down: a subsequent Ping must fail.
	st, err := store.New(ctx(), testsupport.DSN(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	st.Close()
	if err := st.Ping(ctx()); err == nil {
		t.Fatal("expected Ping to fail after Close")
	}
}

func TestEmployees_CRUDAndScoping(t *testing.T) {
	st := testsupport.NewStore(t)

	adminID := mkEmployee(t, st, "Admin", "admin@x.test", "admin", nil)
	mgrID := mkEmployee(t, st, "Manager", "mgr@x.test", "manager", &adminID)
	empA := mkEmployee(t, st, "Alice", "alice@x.test", "employee", &mgrID)
	_ = mkEmployee(t, st, "Bob", "bob@x.test", "employee", &mgrID)

	// GetEmployee / GetEmployeeByEmail round-trip.
	got, err := st.GetEmployee(ctx(), empA)
	if err != nil || got.Email != "alice@x.test" {
		t.Fatalf("GetEmployee = %+v, err %v", got, err)
	}
	if got.ManagerID.Int64 != mgrID || !got.ManagerID.Valid {
		t.Fatalf("Alice's manager = %+v, want %d", got.ManagerID, mgrID)
	}
	byEmail, err := st.GetEmployeeByEmail(ctx(), "mgr@x.test")
	if err != nil || byEmail.ID != mgrID {
		t.Fatalf("GetEmployeeByEmail = %+v, err %v", byEmail, err)
	}
	if _, err := st.GetEmployeeByEmail(ctx(), "ghost@x.test"); err == nil {
		t.Fatal("expected error for unknown email")
	}

	// admin (top of tree) has NULL manager.
	admin, _ := st.GetEmployee(ctx(), adminID)
	if admin.ManagerID.Valid {
		t.Fatalf("admin should have NULL manager, got %+v", admin.ManagerID)
	}

	// ListEmployees(0) = everyone; ListEmployees(mgr) = only reports.
	all, err := st.ListEmployees(ctx(), 0)
	if err != nil || len(all) != 4 {
		t.Fatalf("ListEmployees(0) len = %d, err %v", len(all), err)
	}
	reports, err := st.ListEmployees(ctx(), mgrID)
	if err != nil || len(reports) != 2 {
		t.Fatalf("ListEmployees(mgr) len = %d, err %v", len(reports), err)
	}
	// manager_name is populated from the LEFT JOIN.
	for _, r := range reports {
		if r.ManagerName != "Manager" {
			t.Errorf("report %s manager_name = %q, want Manager", r.Name, r.ManagerName)
		}
	}
}

func TestSessions_Lifecycle(t *testing.T) {
	st := testsupport.NewStore(t)
	empID := mkEmployee(t, st, "Sess", "sess@x.test", "employee", nil)

	// Valid session resolves to its employee.
	if _, err := st.CreateSession(ctx(), "tok-valid", empID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	emp, err := st.GetSessionEmployee(ctx(), "tok-valid")
	if err != nil || emp.ID != empID {
		t.Fatalf("GetSessionEmployee = %+v, err %v", emp, err)
	}

	// Expired session does not resolve (WHERE expires_at > now()).
	if _, err := st.CreateSession(ctx(), "tok-expired", empID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}
	if _, err := st.GetSessionEmployee(ctx(), "tok-expired"); err == nil {
		t.Fatal("expired token should not resolve")
	}

	// DeleteSession removes the valid one.
	if err := st.DeleteSession(ctx(), "tok-valid"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := st.GetSessionEmployee(ctx(), "tok-valid"); err == nil {
		t.Fatal("deleted token should not resolve")
	}

	// DeleteExpiredSessions sweeps the expired one; unknown token delete is a no-op.
	if err := st.DeleteExpiredSessions(ctx()); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	if err := st.DeleteSession(ctx(), "does-not-exist"); err != nil {
		t.Fatalf("DeleteSession(unknown) should be a no-op, got %v", err)
	}
}

func TestLeaveTypes_ListAndCreate(t *testing.T) {
	st := testsupport.NewStore(t)
	mkLeaveType(t, st, "Sick", 12)
	mkLeaveType(t, st, "Annual", 25)

	types, err := st.ListLeaveTypes(ctx())
	if err != nil || len(types) != 2 {
		t.Fatalf("ListLeaveTypes len = %d, err %v", len(types), err)
	}
	// Ordered by name: Annual before Sick.
	if types[0].Name != "Annual" || types[1].Name != "Sick" {
		t.Fatalf("unexpected order: %s, %s", types[0].Name, types[1].Name)
	}
	// UNIQUE(name) violation surfaces as an error.
	if _, err := st.CreateLeaveType(ctx(), "Sick", 5, "#000000"); err == nil {
		t.Fatal("expected unique-violation error for duplicate leave type name")
	}
}

func TestUpsertAllocation_InsertThenUpdate(t *testing.T) {
	st := testsupport.NewStore(t)
	empID := mkEmployee(t, st, "Al", "al@x.test", "employee", nil)
	ltID := mkLeaveType(t, st, "Annual", 25)
	year := int32(2026)

	a, err := st.UpsertAllocation(ctx(), empID, ltID, year, 20)
	if err != nil || a.Days != 20 {
		t.Fatalf("insert allocation = %+v, err %v", a, err)
	}
	// Same (employee, type, year) updates rather than inserts a second row.
	a2, err := st.UpsertAllocation(ctx(), empID, ltID, year, 30)
	if err != nil || a2.Days != 30 || a2.ID != a.ID {
		t.Fatalf("update allocation = %+v (orig id %d), err %v", a2, a.ID, err)
	}
}

func TestRequests_LifecycleAndDecisions(t *testing.T) {
	st := testsupport.NewStore(t)
	mgrID := mkEmployee(t, st, "Mgr", "mgr2@x.test", "manager", nil)
	empID := mkEmployee(t, st, "Emp", "emp2@x.test", "employee", &mgrID)
	ltID := mkLeaveType(t, st, "Annual", 25)

	start := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) // Monday
	end := start.AddDate(0, 0, 2)                        // Wednesday -> 3 working days
	days := int32(leave.WorkingDays(start, end, nil))

	req, err := st.CreateLeaveRequest(ctx(), empID, ltID, start, end, days, "trip")
	if err != nil {
		t.Fatalf("CreateLeaveRequest: %v", err)
	}
	if req.Status != "pending" {
		t.Fatalf("new request status = %q, want pending", req.Status)
	}

	// Manager sees it pending; count matches.
	pending, err := st.ListPendingForManager(ctx(), mgrID)
	if err != nil || len(pending) != 1 || pending[0].EmployeeName != "Emp" {
		t.Fatalf("ListPendingForManager = %+v, err %v", pending, err)
	}
	n, err := st.CountPendingForManager(ctx(), mgrID)
	if err != nil || n != 1 {
		t.Fatalf("CountPendingForManager = %d, err %v", n, err)
	}

	// Get + list by employee.
	got, err := st.GetLeaveRequest(ctx(), req.ID)
	if err != nil || got.WorkingDays != days {
		t.Fatalf("GetLeaveRequest = %+v, err %v", got, err)
	}
	list, err := st.ListRequestsByEmployee(ctx(), empID)
	if err != nil || len(list) != 1 || list[0].LeaveTypeName != "Annual" {
		t.Fatalf("ListRequestsByEmployee = %+v, err %v", list, err)
	}

	// Approve it; it leaves the pending queue.
	if err := st.SetRequestStatus(ctx(), req.ID, "approved", mgrID); err != nil {
		t.Fatalf("SetRequestStatus approve: %v", err)
	}
	if n, _ := st.CountPendingForManager(ctx(), mgrID); n != 0 {
		t.Fatalf("pending after approve = %d, want 0", n)
	}
	// Second decision is a no-op (guard: status = 'pending').
	if err := st.SetRequestStatus(ctx(), req.ID, "rejected", mgrID); err != nil {
		t.Fatalf("SetRequestStatus second: %v", err)
	}
	if g, _ := st.GetLeaveRequest(ctx(), req.ID); g.Status != "approved" {
		t.Fatalf("status after redundant decision = %q, want approved", g.Status)
	}
}

func TestCancelOwnRequest_OwnershipAndPendingGuard(t *testing.T) {
	st := testsupport.NewStore(t)
	empID := mkEmployee(t, st, "Own", "own@x.test", "employee", nil)
	otherID := mkEmployee(t, st, "Other", "other@x.test", "employee", nil)
	ltID := mkLeaveType(t, st, "Annual", 25)
	start := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	req, err := st.CreateLeaveRequest(ctx(), empID, ltID, start, start, 1, "day off")
	if err != nil {
		t.Fatalf("CreateLeaveRequest: %v", err)
	}

	// Someone else cannot cancel it (ownership guard) — no error, but no change.
	if err := st.CancelOwnRequest(ctx(), req.ID, otherID); err != nil {
		t.Fatalf("CancelOwnRequest(other): %v", err)
	}
	if g, _ := st.GetLeaveRequest(ctx(), req.ID); g.Status != "pending" {
		t.Fatalf("status after other's cancel = %q, want pending", g.Status)
	}

	// Owner cancels it.
	if err := st.CancelOwnRequest(ctx(), req.ID, empID); err != nil {
		t.Fatalf("CancelOwnRequest(owner): %v", err)
	}
	if g, _ := st.GetLeaveRequest(ctx(), req.ID); g.Status != "cancelled" {
		t.Fatalf("status after owner cancel = %q, want cancelled", g.Status)
	}
}

func TestListBalances_AllocatedUsedRemaining(t *testing.T) {
	st := testsupport.NewStore(t)
	empID := mkEmployee(t, st, "Bal", "bal@x.test", "employee", nil)
	ltID := mkLeaveType(t, st, "Annual", 25)
	year := int32(2026)

	if _, err := st.UpsertAllocation(ctx(), empID, ltID, year, 20); err != nil {
		t.Fatalf("UpsertAllocation: %v", err)
	}

	// A 3-working-day approved request this year is counted as "used".
	start := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) // Monday
	end := start.AddDate(0, 0, 2)                        // Wednesday
	days := int32(leave.WorkingDays(start, end, nil))
	req, err := st.CreateLeaveRequest(ctx(), empID, ltID, start, end, days, "")
	if err != nil {
		t.Fatalf("CreateLeaveRequest: %v", err)
	}
	// Pending doesn't count yet.
	balances, _ := st.ListBalances(ctx(), empID, year)
	if b := findBalance(balances, ltID); b == nil || b.Used != 0 || b.Allocated != 20 || b.Remaining != 20 {
		t.Fatalf("pending balance = %+v, want allocated 20 used 0 remaining 20", b)
	}

	// Approve -> now counted.
	if err := st.SetRequestStatus(ctx(), req.ID, "approved", empID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	balances, err = st.ListBalances(ctx(), empID, year)
	if err != nil {
		t.Fatalf("ListBalances: %v", err)
	}
	b := findBalance(balances, ltID)
	if b == nil {
		t.Fatalf("leave type %d missing from balances", ltID)
	}
	if b.Allocated != 20 || b.Used != days || b.Remaining != 20-days {
		t.Fatalf("balance = {alloc %d used %d remaining %d}, want {20 %d %d}",
			b.Allocated, b.Used, b.Remaining, days, 20-days)
	}
}

func TestCalendarAndHolidays(t *testing.T) {
	st := testsupport.NewStore(t)
	empID := mkEmployee(t, st, "Cal", "cal@x.test", "employee", nil)
	ltID := mkLeaveType(t, st, "Annual", 25)

	// An approved request inside a window shows up in ListApprovedInRange.
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 4)
	req, _ := st.CreateLeaveRequest(ctx(), empID, ltID, start, end, 5, "")
	_ = st.SetRequestStatus(ctx(), req.ID, "approved", empID)

	rangeStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	approved, err := st.ListApprovedInRange(ctx(), rangeStart, rangeEnd)
	if err != nil || len(approved) != 1 || approved[0].EmployeeName != "Cal" {
		t.Fatalf("ListApprovedInRange = %+v, err %v", approved, err)
	}
	// A window before the request finds nothing.
	none, _ := st.ListApprovedInRange(ctx(),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))
	if len(none) != 0 {
		t.Fatalf("expected no approved leave in January, got %d", len(none))
	}

	// Holidays: create, list, list-in-range, delete.
	h1, err := st.CreateHoliday(ctx(), "Republic Day", time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateHoliday: %v", err)
	}
	if _, err := st.CreateHoliday(ctx(), "New Year", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CreateHoliday 2: %v", err)
	}
	all, _ := st.ListHolidays(ctx())
	if len(all) != 2 {
		t.Fatalf("ListHolidays len = %d, want 2", len(all))
	}
	inRange, _ := st.ListHolidaysInRange(ctx(),
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	if len(inRange) != 1 || inRange[0].Name != "Republic Day" {
		t.Fatalf("ListHolidaysInRange = %+v, want [Republic Day]", inRange)
	}
	if err := st.DeleteHoliday(ctx(), h1.ID); err != nil {
		t.Fatalf("DeleteHoliday: %v", err)
	}
	if all, _ := st.ListHolidays(ctx()); len(all) != 1 {
		t.Fatalf("ListHolidays after delete = %d, want 1", len(all))
	}
	// Duplicate holiday_date (UNIQUE) errors.
	if _, err := st.CreateHoliday(ctx(), "Dup", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected unique-violation for duplicate holiday_date")
	}
}

// findBalance returns the balance row for a leave type, or nil.
func findBalance(balances []db.ListBalancesRow, leaveTypeID int64) *db.ListBalancesRow {
	for i := range balances {
		if balances[i].LeaveTypeID == leaveTypeID {
			return &balances[i]
		}
	}
	return nil
}
