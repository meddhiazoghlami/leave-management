package seed_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/internal/seed"
)

var (
	errSeed     = errors.New("seed boom")
	errNotFound = errors.New("not found")
)

// fakeSeedStore is a seed.Store whose behaviour each scenario tunes to force one
// specific error branch inside seed.Run.
type fakeSeedStore struct {
	empExists bool           // GetEmployeeByEmail returns an employee (vs not-found)
	types     []db.LeaveType // returned by ListLeaveTypes
	failOn    string         // method name that returns errSeed
}

func (f *fakeSeedStore) bad(name string) bool { return f.failOn == name }

func (f *fakeSeedStore) GetEmployeeByEmail(context.Context, string) (db.Employee, error) {
	if f.bad("GetEmployeeByEmail") {
		return db.Employee{}, errSeed
	}
	if f.empExists {
		return db.Employee{ID: 1}, nil
	}
	return db.Employee{}, errNotFound
}
func (f *fakeSeedStore) CreateEmployee(context.Context, string, string, string, string, *int64) (db.Employee, error) {
	if f.bad("CreateEmployee") {
		return db.Employee{}, errSeed
	}
	return db.Employee{ID: 1}, nil
}
func (f *fakeSeedStore) ListLeaveTypes(context.Context) ([]db.LeaveType, error) {
	if f.bad("ListLeaveTypes") {
		return nil, errSeed
	}
	return f.types, nil
}
func (f *fakeSeedStore) CreateLeaveType(context.Context, string, float64, string) (db.LeaveType, error) {
	if f.bad("CreateLeaveType") {
		return db.LeaveType{}, errSeed
	}
	return db.LeaveType{ID: 1}, nil
}
func (f *fakeSeedStore) UpsertAllocation(context.Context, int64, int64, int32, float64) (db.LeaveAllocation, error) {
	if f.bad("UpsertAllocation") {
		return db.LeaveAllocation{}, errSeed
	}
	return db.LeaveAllocation{}, nil
}
func (f *fakeSeedStore) ListHolidays(context.Context) ([]db.PublicHoliday, error) {
	if f.bad("ListHolidays") {
		return nil, errSeed
	}
	return nil, nil
}
func (f *fakeSeedStore) CreateHoliday(context.Context, string, time.Time) (db.PublicHoliday, error) {
	if f.bad("CreateHoliday") {
		return db.PublicHoliday{}, errSeed
	}
	return db.PublicHoliday{}, nil
}
func (f *fakeSeedStore) ListRequestsByEmployee(context.Context, int64) ([]db.ListRequestsByEmployeeRow, error) {
	return nil, nil // no existing requests -> seed proceeds to create sample ones
}
func (f *fakeSeedStore) CreateLeaveRequest(context.Context, int64, int64, time.Time, time.Time, float64, string) (db.CreateLeaveRequestRow, error) {
	if f.bad("CreateLeaveRequest") {
		return db.CreateLeaveRequestRow{}, errSeed
	}
	return db.CreateLeaveRequestRow{}, nil
}

func TestRun_ErrorBranches(t *testing.T) {
	full := []db.LeaveType{{ID: 1, Name: "Annual"}, {ID: 2, Name: "Sick"}, {ID: 3, Name: "Unpaid"}}

	cases := []struct {
		name string
		fake *fakeSeedStore
	}{
		{"create employee fails", &fakeSeedStore{failOn: "CreateEmployee"}},
		{"list leave types fails", &fakeSeedStore{empExists: true, failOn: "ListLeaveTypes"}},
		{"create leave type fails", &fakeSeedStore{empExists: true, failOn: "CreateLeaveType"}}, // types empty -> create path
		{"upsert allocation fails", &fakeSeedStore{empExists: true, types: full, failOn: "UpsertAllocation"}},
		{"list holidays fails", &fakeSeedStore{empExists: true, types: full, failOn: "ListHolidays"}},
		{"create holiday fails", &fakeSeedStore{empExists: true, types: full, failOn: "CreateHoliday"}},
		{"annual type missing", &fakeSeedStore{empExists: true, types: []db.LeaveType{{Name: "Sick"}, {Name: "Unpaid"}}}},
		{"sick type missing", &fakeSeedStore{empExists: true, types: []db.LeaveType{{Name: "Annual"}}}},
		{"create sample request fails", &fakeSeedStore{empExists: true, types: full, failOn: "CreateLeaveRequest"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := seed.Run(context.Background(), tc.fake); err == nil {
				t.Fatal("expected seed.Run to return an error")
			}
		})
	}
}
