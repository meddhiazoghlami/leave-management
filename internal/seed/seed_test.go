package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/seed"
	"github.com/meddhiazoghlami/leave-management/internal/testsupport"
)

// TestRun_SeedsDemoOrg checks that seeding builds the expected demo org and that
// re-running is idempotent (no duplicate rows). Runs against a real Postgres via
// testcontainers; skipped when Docker is unavailable.
func TestRun_SeedsDemoOrg(t *testing.T) {
	st := testsupport.NewStore(t)
	ctx := context.Background()

	if err := seed.Run(ctx, st); err != nil {
		t.Fatalf("seed.Run: %v", err)
	}

	// 5 employees: admin, manager, and three reports.
	emps, err := st.ListEmployees(ctx, 0)
	if err != nil {
		t.Fatalf("ListEmployees: %v", err)
	}
	if len(emps) != 5 {
		t.Fatalf("employees = %d, want 5", len(emps))
	}

	// 3 leave types.
	types, err := st.ListLeaveTypes(ctx)
	if err != nil {
		t.Fatalf("ListLeaveTypes: %v", err)
	}
	if len(types) != 3 {
		t.Fatalf("leave types = %d, want 3", len(types))
	}

	// 5 seeded holidays.
	holidays, err := st.ListHolidays(ctx)
	if err != nil {
		t.Fatalf("ListHolidays: %v", err)
	}
	if len(holidays) != 5 {
		t.Fatalf("holidays = %d, want 5", len(holidays))
	}

	// The manager has two pending sample requests to approve.
	manager, err := st.GetEmployeeByEmail(ctx, "manager@acme.test")
	if err != nil {
		t.Fatalf("GetEmployeeByEmail(manager): %v", err)
	}
	pending, err := st.CountPendingForManager(ctx, manager.ID)
	if err != nil {
		t.Fatalf("CountPendingForManager: %v", err)
	}
	if pending != 2 {
		t.Fatalf("pending for manager = %d, want 2", pending)
	}

	// Allocations exist for a report this year (default_days applied).
	sam, err := st.GetEmployeeByEmail(ctx, "sam@acme.test")
	if err != nil {
		t.Fatalf("GetEmployeeByEmail(sam): %v", err)
	}
	balances, err := st.ListBalances(ctx, sam.ID, int32(time.Now().Year()))
	if err != nil {
		t.Fatalf("ListBalances: %v", err)
	}
	var anyAllocated bool
	for _, b := range balances {
		if b.Allocated > 0 {
			anyAllocated = true
		}
	}
	if !anyAllocated {
		t.Fatal("expected at least one allocated leave type for a seeded employee")
	}

	// Idempotent: a second run must not add rows.
	if err := seed.Run(ctx, st); err != nil {
		t.Fatalf("seed.Run (2nd): %v", err)
	}
	emps2, _ := st.ListEmployees(ctx, 0)
	types2, _ := st.ListLeaveTypes(ctx)
	holidays2, _ := st.ListHolidays(ctx)
	pending2, _ := st.CountPendingForManager(ctx, manager.ID)
	if len(emps2) != 5 || len(types2) != 3 || len(holidays2) != 5 || pending2 != 2 {
		t.Fatalf("second run changed counts: emps %d types %d holidays %d pending %d",
			len(emps2), len(types2), len(holidays2), pending2)
	}
}
