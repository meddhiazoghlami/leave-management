// Package seed populates the database with a small, realistic demo org so the
// app is usable immediately: an admin, a manager reporting to them, three
// employees reporting to the manager, leave types, this-year allocations, a few
// public holidays, and a couple of pending sample requests.
//
// It is re-runnable: existing rows are detected and left alone. Invoked by the
// `leave-management seed` CLI command.
package seed

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/internal/leave"
)

// Store is the subset of the data layer the seeder needs. Declared here
// (consumer-side) so seeding depends on an interface — the concrete *store.Store
// satisfies it — which keeps its error branches testable via a fake.
type Store interface {
	GetEmployeeByEmail(ctx context.Context, email string) (db.Employee, error)
	CreateEmployee(ctx context.Context, name, email, passwordHash, role string, managerID *int64) (db.Employee, error)
	ListLeaveTypes(ctx context.Context) ([]db.LeaveType, error)
	CreateLeaveType(ctx context.Context, name string, defaultDays float64, color string) (db.LeaveType, error)
	UpsertAllocation(ctx context.Context, employeeID, leaveTypeID int64, year int32, days float64) (db.LeaveAllocation, error)
	ListHolidays(ctx context.Context) ([]db.PublicHoliday, error)
	CreateHoliday(ctx context.Context, name string, date time.Time) (db.PublicHoliday, error)
	ListRequestsByEmployee(ctx context.Context, employeeID int64) ([]db.ListRequestsByEmployeeRow, error)
	CreateLeaveRequest(ctx context.Context, employeeID, leaveTypeID int64, start, end time.Time, workingDays float64, reason string) (db.CreateLeaveRequestRow, error)
}

// Password is shared by every seeded account.
const Password = "password"

// Run seeds the demo org. Safe to run repeatedly.
func Run(ctx context.Context, st Store) error {
	hash, err := auth.HashPassword(Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Org tree: admin -> manager -> employees.
	admin, err := ensureEmployee(ctx, st, "Dara Admin", "admin@acme.test", hash, auth.RoleAdmin, nil)
	if err != nil {
		return err
	}
	manager, err := ensureEmployee(ctx, st, "Mona Manager", "manager@acme.test", hash, auth.RoleManager, &admin.ID)
	if err != nil {
		return err
	}
	sam, err := ensureEmployee(ctx, st, "Sam Employee", "sam@acme.test", hash, auth.RoleEmployee, &manager.ID)
	if err != nil {
		return err
	}
	nadia, err := ensureEmployee(ctx, st, "Nadia Employee", "nadia@acme.test", hash, auth.RoleEmployee, &manager.ID)
	if err != nil {
		return err
	}
	youssef, err := ensureEmployee(ctx, st, "Youssef Employee", "youssef@acme.test", hash, auth.RoleEmployee, &manager.ID)
	if err != nil {
		return err
	}

	types, err := ensureLeaveTypes(ctx, st)
	if err != nil {
		return err
	}

	// Allocations for the current year (managers get them too so they have a
	// dashboard). Each type's default becomes the allocation.
	year := int32(time.Now().Year())
	for _, emp := range []db.Employee{manager, sam, nadia, youssef} {
		for _, t := range types {
			if _, err := st.UpsertAllocation(ctx, emp.ID, t.ID, year, t.DefaultDays); err != nil {
				return fmt.Errorf("allocation for %s: %w", emp.Email, err)
			}
		}
	}

	if err := ensureHolidays(ctx, st, int(year)); err != nil {
		return err
	}

	// Sample pending requests so the manager has something to approve.
	annual, ok := typeByName(types, "Annual")
	if !ok {
		return fmt.Errorf("seed: Annual leave type missing")
	}
	sick, ok := typeByName(types, "Sick")
	if !ok {
		return fmt.Errorf("seed: Sick leave type missing")
	}
	mon := nextMonday(time.Now())
	if err := ensureSampleRequest(ctx, st, sam, annual, mon, mon.AddDate(0, 0, 2)); err != nil { // Mon–Wed
		return err
	}
	if err := ensureSampleRequest(ctx, st, nadia, sick, mon.AddDate(0, 0, 7), mon.AddDate(0, 0, 7)); err != nil { // one day
		return err
	}

	return nil
}

func ensureEmployee(ctx context.Context, st Store, name, email, hash, role string, managerID *int64) (db.Employee, error) {
	if emp, err := st.GetEmployeeByEmail(ctx, email); err == nil {
		return emp, nil
	}
	emp, err := st.CreateEmployee(ctx, name, email, hash, role, managerID)
	if err != nil {
		return db.Employee{}, fmt.Errorf("create employee %s: %w", email, err)
	}
	log.Printf("  created employee %s (%s)", name, role)
	return emp, nil
}

func ensureLeaveTypes(ctx context.Context, st Store) ([]db.LeaveType, error) {
	existing, err := st.ListLeaveTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list leave types: %w", err)
	}
	if len(existing) > 0 {
		return existing, nil
	}
	defs := []struct {
		Name  string
		Days  float64
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
			return nil, fmt.Errorf("create leave type %s: %w", d.Name, err)
		}
		log.Printf("  created leave type %s", d.Name)
		out = append(out, t)
	}
	return out, nil
}

func ensureHolidays(ctx context.Context, st Store, year int) error {
	existing, err := st.ListHolidays(ctx)
	if err != nil {
		return fmt.Errorf("list holidays: %w", err)
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
			return fmt.Errorf("parse holiday %s: %w", h.Date, err)
		}
		if _, err := st.CreateHoliday(ctx, h.Name, d); err != nil {
			return fmt.Errorf("create holiday %s: %w", h.Name, err)
		}
	}
	return nil
}

func ensureSampleRequest(ctx context.Context, st Store, emp db.Employee, t db.LeaveType, start, end time.Time) error {
	if reqs, _ := st.ListRequestsByEmployee(ctx, emp.ID); len(reqs) > 0 {
		return nil // already has requests — don't pile on more each run
	}
	days := leave.WorkingDays(start, end, leave.DefaultWorkingWeek(), nil)
	if _, err := st.CreateLeaveRequest(ctx, emp.ID, t.ID, start, end, float64(days), "Sample seeded request"); err != nil {
		return fmt.Errorf("create sample request for %s: %w", emp.Email, err)
	}
	return nil
}

func typeByName(types []db.LeaveType, name string) (db.LeaveType, bool) {
	for _, t := range types {
		if t.Name == name {
			return t, true
		}
	}
	return db.LeaveType{}, false
}

// nextMonday returns the next Monday on or after t, normalised to midnight UTC.
func nextMonday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
