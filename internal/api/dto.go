package api

import (
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/db"
)

// dateLayout is the wire format for date-only fields (start/end/holiday dates).
// Timestamps (created_at, decided_at) stay RFC3339 via time.Time's default JSON.
const dateLayout = "2006-01-02"

// The DTOs below are the API's public contract. They exist so responses never
// leak internal representation: db.Employee carries a PasswordHash and pgtype.*
// columns that serialize badly, and the sqlc row structs have no json tags. Each
// DTO is explicit and each mapper is the single place that shape is defined.

// UserDTO is a safe view of an employee — no password hash, manager_id as a
// nullable int rather than pgtype.Int8.
type UserDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ManagerID *int64 `json:"manager_id"`
}

func toUserDTO(e db.Employee) UserDTO {
	u := UserDTO{ID: e.ID, Name: e.Name, Email: e.Email, Role: e.Role}
	if e.ManagerID.Valid {
		id := e.ManagerID.Int64
		u.ManagerID = &id
	}
	return u
}

// EmployeeDTO is a row in the team directory (joined manager name, no email of
// the manager etc.).
type EmployeeDTO struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	ManagerName string `json:"manager_name"`
}

func toEmployeeDTO(r db.ListEmployeesRow) EmployeeDTO {
	return EmployeeDTO{ID: r.ID, Name: r.Name, Email: r.Email, Role: r.Role, ManagerName: r.ManagerName}
}

// LeaveTypeDTO is a configurable leave type.
type LeaveTypeDTO struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	DefaultDays float64 `json:"default_days"`
	Color       string  `json:"color"`
}

func toLeaveTypeDTO(t db.LeaveType) LeaveTypeDTO {
	return LeaveTypeDTO{ID: t.ID, Name: t.Name, DefaultDays: t.DefaultDays, Color: t.Color}
}

// BalanceDTO is one leave type's allocation/usage/remaining for the current
// leave year.
type BalanceDTO struct {
	LeaveTypeID int64   `json:"leave_type_id"`
	Name        string  `json:"leave_type_name"`
	Color       string  `json:"leave_type_color"`
	Allocated   float64 `json:"allocated"`
	Used        float64 `json:"used"`
	Remaining   float64 `json:"remaining"`
}

func toBalanceDTO(b db.ListBalancesRow) BalanceDTO {
	return BalanceDTO{
		LeaveTypeID: b.LeaveTypeID, Name: b.LeaveTypeName, Color: b.LeaveTypeColor,
		Allocated: b.Allocated, Used: b.Used, Remaining: b.Remaining,
	}
}

// RequestDTO is one of the caller's own leave requests.
type RequestDTO struct {
	ID          int64   `json:"id"`
	LeaveTypeID int64   `json:"leave_type_id"`
	Name        string  `json:"leave_type_name"`
	Color       string  `json:"leave_type_color"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	WorkingDays float64 `json:"working_days"`
	Reason      string  `json:"reason"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
}

func toRequestDTO(r db.ListRequestsByEmployeeRow) RequestDTO {
	return RequestDTO{
		ID: r.ID, LeaveTypeID: r.LeaveTypeID, Name: r.LeaveTypeName, Color: r.LeaveTypeColor,
		StartDate: r.StartDate.Format(dateLayout), EndDate: r.EndDate.Format(dateLayout),
		WorkingDays: r.WorkingDays, Reason: r.Reason, Status: r.Status,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}

// PendingDTO is a pending request awaiting the caller's decision, with the
// requester's name attached.
type PendingDTO struct {
	ID           int64   `json:"id"`
	EmployeeID   int64   `json:"employee_id"`
	EmployeeName string  `json:"employee_name"`
	Name         string  `json:"leave_type_name"`
	Color        string  `json:"leave_type_color"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
	WorkingDays  float64 `json:"working_days"`
	Reason       string  `json:"reason"`
	CreatedAt    string  `json:"created_at"`
}

func toPendingDTO(r db.ListPendingForManagerRow) PendingDTO {
	return PendingDTO{
		ID: r.ID, EmployeeID: r.EmployeeID, EmployeeName: r.EmployeeName,
		Name: r.LeaveTypeName, Color: r.LeaveTypeColor,
		StartDate: r.StartDate.Format(dateLayout), EndDate: r.EndDate.Format(dateLayout),
		WorkingDays: r.WorkingDays, Reason: r.Reason, CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}

// HolidayDTO is a public holiday.
type HolidayDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Date string `json:"date"`
}

func toHolidayDTO(h db.PublicHoliday) HolidayDTO {
	return HolidayDTO{ID: h.ID, Name: h.Name, Date: h.HolidayDate.Format(dateLayout)}
}

// SettingsDTO is the company-wide configuration (working week + leave-year start).
type SettingsDTO struct {
	Name                string `json:"name"`
	LeaveYearStartMonth int32  `json:"leave_year_start_month"`
	WorkMonday          bool   `json:"work_monday"`
	WorkTuesday         bool   `json:"work_tuesday"`
	WorkWednesday       bool   `json:"work_wednesday"`
	WorkThursday        bool   `json:"work_thursday"`
	WorkFriday          bool   `json:"work_friday"`
	WorkSaturday        bool   `json:"work_saturday"`
	WorkSunday          bool   `json:"work_sunday"`
}

func toSettingsDTO(s db.CompanySetting) SettingsDTO {
	return SettingsDTO{
		Name: s.Name, LeaveYearStartMonth: s.LeaveYearStartMonth,
		WorkMonday: s.WorkMonday, WorkTuesday: s.WorkTuesday, WorkWednesday: s.WorkWednesday,
		WorkThursday: s.WorkThursday, WorkFriday: s.WorkFriday,
		WorkSaturday: s.WorkSaturday, WorkSunday: s.WorkSunday,
	}
}

// CalendarEntryDTO is one approved leave block overlapping a queried range,
// used to paint a calendar client-side.
type CalendarEntryDTO struct {
	ID           int64  `json:"id"`
	EmployeeName string `json:"employee_name"`
	Name         string `json:"leave_type_name"`
	Color        string `json:"leave_type_color"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
}

func toCalendarEntryDTO(r db.ListApprovedInRangeRow) CalendarEntryDTO {
	return CalendarEntryDTO{
		ID: r.ID, EmployeeName: r.EmployeeName, Name: r.LeaveTypeName, Color: r.LeaveTypeColor,
		StartDate: r.StartDate.Format(dateLayout), EndDate: r.EndDate.Format(dateLayout),
	}
}
