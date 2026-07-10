// Command seed populates a fresh database with a small, realistic org so the
// app is usable immediately: an admin, a manager reporting to them, three
// employees reporting to the manager, leave types, this-year allocations, a few
// public holidays, and a couple of pending sample requests.
//
// It is re-runnable: existing rows are detected and left alone.
//
//	go run ./cmd/seed
//
// All seeded accounts share the password "password".
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dzovi/leave-management/internal/auth"
	"github.com/dzovi/leave-management/internal/config"
	"github.com/dzovi/leave-management/internal/db"
	"github.com/dzovi/leave-management/internal/leave"
	"github.com/dzovi/leave-management/internal/store"
)

const seedPassword = "password"

func main() {
	cfg := config.Load()
	ctx := context.Background()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer st.Close()

	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	// Org tree: admin -> manager -> employees.
	admin := ensureEmployee(ctx, st, "Dara Admin", "admin@acme.test", hash, auth.RoleAdmin, nil)
	manager := ensureEmployee(ctx, st, "Mona Manager", "manager@acme.test", hash, auth.RoleManager, &admin.ID)
	sam := ensureEmployee(ctx, st, "Sam Employee", "sam@acme.test", hash, auth.RoleEmployee, &manager.ID)
	nadia := ensureEmployee(ctx, st, "Nadia Employee", "nadia@acme.test", hash, auth.RoleEmployee, &manager.ID)
	youssef := ensureEmployee(ctx, st, "Youssef Employee", "youssef@acme.test", hash, auth.RoleEmployee, &manager.ID)

	types := ensureLeaveTypes(ctx, st)

	// Allocations for the current year (managers get them too so they have a
	// dashboard). Each type's default becomes the allocation.
	year := int32(time.Now().Year())
	for _, emp := range []db.Employee{manager, sam, nadia, youssef} {
		for _, t := range types {
			if _, err := st.UpsertAllocation(ctx, emp.ID, t.ID, year, t.DefaultDays); err != nil {
				log.Fatalf("allocation for %s: %v", emp.Email, err)
			}
		}
	}

	ensureHolidays(ctx, st, int(year))

	// Sample pending requests so the manager has something to approve.
	annual := typeByName(types, "Annual")
	sick := typeByName(types, "Sick")
	mon := nextMonday(time.Now())
	ensureSampleRequest(ctx, st, sam, annual, mon, mon.AddDate(0, 0, 2))              // Mon–Wed
	ensureSampleRequest(ctx, st, nadia, sick, mon.AddDate(0, 0, 7), mon.AddDate(0, 0, 7)) // one day

	log.Printf("✔ seed complete — log in as admin@acme.test / manager@acme.test / sam@acme.test (password %q)", seedPassword)
}

func ensureEmployee(ctx context.Context, st *store.Store, name, email, hash, role string, managerID *int64) db.Employee {
	if emp, err := st.GetEmployeeByEmail(ctx, email); err == nil {
		return emp
	}
	emp, err := st.CreateEmployee(ctx, name, email, hash, role, managerID)
	if err != nil {
		log.Fatalf("create employee %s: %v", email, err)
	}
	log.Printf("  created employee %s (%s)", name, role)
	return emp
}

func ensureLeaveTypes(ctx context.Context, st *store.Store) []db.LeaveType {
	existing, err := st.ListLeaveTypes(ctx)
	if err != nil {
		log.Fatalf("list leave types: %v", err)
	}
	if len(existing) > 0 {
		return existing
	}
	defs := []struct {
		Name  string
		Days  int32
		Color string
	}{
		{"Annual", 25, "#6366f1"},
		{"Sick", 12, "#f43f5e"},
		{"Unpaid", 0, "#64748b"},
	}
	out := make([]db.LeaveType, 0, len(defs))
	for _, d := range defs {
		t, err := st.CreateLeaveType(ctx, d.Name, d.Days, d.Color)
		if err != nil {
			log.Fatalf("create leave type %s: %v", d.Name, err)
		}
		log.Printf("  created leave type %s", d.Name)
		out = append(out, t)
	}
	return out
}

func ensureHolidays(ctx context.Context, st *store.Store, year int) {
	existing, err := st.ListHolidays(ctx)
	if err != nil {
		log.Fatalf("list holidays: %v", err)
	}
	have := make(map[string]bool, len(existing))
	for _, h := range existing {
		have[h.HolidayDate.Format("2006-01-02")] = true
	}
	holidays := []struct {
		Name string
		Date string
	}{
		{"New Year's Day", fmt.Sprintf("%d-01-01", year)},
		{"Independence Day", fmt.Sprintf("%d-03-20", year)},
		{"Labour Day", fmt.Sprintf("%d-05-01", year)},
		{"Republic Day", fmt.Sprintf("%d-07-25", year)},
		{"Christmas Day", fmt.Sprintf("%d-12-25", year)},
	}
	for _, h := range holidays {
		if have[h.Date] {
			continue
		}
		d, err := time.Parse("2006-01-02", h.Date)
		if err != nil {
			log.Fatalf("parse holiday %s: %v", h.Date, err)
		}
		if _, err := st.CreateHoliday(ctx, h.Name, d); err != nil {
			log.Fatalf("create holiday %s: %v", h.Name, err)
		}
	}
}

func ensureSampleRequest(ctx context.Context, st *store.Store, emp db.Employee, t db.LeaveType, start, end time.Time) {
	if reqs, _ := st.ListRequestsByEmployee(ctx, emp.ID); len(reqs) > 0 {
		return // already has requests — don't pile on more each run
	}
	days := leave.WorkingDays(start, end, nil)
	if _, err := st.CreateLeaveRequest(ctx, emp.ID, t.ID, start, end, int32(days), "Sample seeded request"); err != nil {
		log.Fatalf("create sample request for %s: %v", emp.Email, err)
	}
}

func typeByName(types []db.LeaveType, name string) db.LeaveType {
	for _, t := range types {
		if t.Name == name {
			return t
		}
	}
	log.Fatalf("leave type %q not found", name)
	return db.LeaveType{}
}

// nextMonday returns the next Monday on or after t, normalised to midnight UTC.
func nextMonday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
