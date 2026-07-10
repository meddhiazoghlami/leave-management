package store_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/dzovi/leave-management/internal/leave"
	"github.com/dzovi/leave-management/internal/store"
)

// TestBalancesReflectApprovedRequest is an integration test for the sqlc query
// layer: it creates an employee, a leave type, an allocation, and an approved
// request, then checks ListBalances aggregates the used/remaining days
// correctly. It requires a migrated database and is skipped unless
// TEST_DATABASE_URL is set:
//
//	TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable go test ./internal/store/
func TestBalancesReflectApprovedRequest(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run store integration tests")
	}

	ctx := context.Background()
	st, err := store.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer st.Close()

	// Unique-per-run rows so the test is isolated and re-runnable.
	uniq := strconv.FormatInt(time.Now().UnixNano(), 10)
	email := "test+" + uniq + "@example.test"

	emp, err := st.CreateEmployee(ctx, "Test Person", email, "x", "employee", nil)
	if err != nil {
		t.Fatalf("create employee: %v", err)
	}
	lt, err := st.CreateLeaveType(ctx, "Test Type "+uniq, 20, "#000000")
	if err != nil {
		t.Fatalf("create leave type: %v", err)
	}

	year := int32(time.Now().Year())
	if _, err := st.UpsertAllocation(ctx, emp.ID, lt.ID, year, 20); err != nil {
		t.Fatalf("upsert allocation: %v", err)
	}

	// A Mon–Wed request this year -> 3 working days (no holidays factored here).
	start := nextMonday(time.Now())
	end := start.AddDate(0, 0, 2)
	wantDays := int32(leave.WorkingDays(start, end, nil))

	req, err := st.CreateLeaveRequest(ctx, emp.ID, lt.ID, start, end, wantDays, "integration test")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if err := st.SetRequestStatus(ctx, req.ID, "approved", emp.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}

	balances, err := st.ListBalances(ctx, emp.ID, year)
	if err != nil {
		t.Fatalf("list balances: %v", err)
	}

	var found bool
	for _, b := range balances {
		if b.LeaveTypeID != lt.ID {
			continue
		}
		found = true
		if b.Allocated != 20 {
			t.Errorf("Allocated = %d, want 20", b.Allocated)
		}
		if b.Used != wantDays {
			t.Errorf("Used = %d, want %d", b.Used, wantDays)
		}
		if b.Remaining != 20-wantDays {
			t.Errorf("Remaining = %d, want %d", b.Remaining, 20-wantDays)
		}
	}
	if !found {
		t.Fatalf("leave type %d not present in balances", lt.ID)
	}
}

func nextMonday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
