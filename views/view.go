// Package views holds the templ components (the HTML layer) plus the small
// plain-Go view models and formatting helpers they use. Keeping the structs
// here — rather than passing raw handler locals — means each page has one typed
// "props" struct and the .templ files stay declarative.
package views

import (
	"fmt"
	"strconv"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/db"
)

// Nav is what the header needs on every authenticated page.
type Nav struct {
	Title        string
	Name         string
	Role         string
	PendingCount int64
	Active       string // current section: dashboard|requests|approvals|employees|calendar|admin
}

func (n Nav) IsManager() bool { return n.Role == "manager" || n.Role == "admin" || n.Role == "hr" }
func (n Nav) IsAdmin() bool   { return n.Role == "admin" || n.Role == "hr" }

// ─────────────────────────── page view models ────────────────────────────

type DashboardData struct {
	Nav      Nav
	Balances []db.ListBalancesRow
	Recent   []db.ListRequestsByEmployeeRow
}

type RequestsData struct {
	Nav        Nav
	Requests   []db.ListRequestsByEmployeeRow
	LeaveTypes []db.LeaveType
	Today      string
}

type ApprovalsData struct {
	Nav     Nav
	Pending []db.ListPendingForManagerRow
}

type EmployeesData struct {
	Nav       Nav
	Employees []db.ListEmployeesRow
}

type EmployeeProfileData struct {
	Nav      Nav
	Employee db.Employee
	Balances []db.ListBalancesRow
	Requests []db.ListRequestsByEmployeeRow
}

type AdminData struct {
	Nav        Nav
	LeaveTypes []db.LeaveType
	Holidays   []db.PublicHoliday
	Employees  []db.ListEmployeesRow
	Settings   db.CompanySetting
	Year       int
}

// Weekdays lists the working-week flags Monday-first, for rendering the
// settings checkboxes. Field is the form input name; On is the current value.
func (d AdminData) Weekdays() []struct {
	Field string
	Label string
	On    bool
} {
	return []struct {
		Field string
		Label string
		On    bool
	}{
		{"work_monday", "Mon", d.Settings.WorkMonday},
		{"work_tuesday", "Tue", d.Settings.WorkTuesday},
		{"work_wednesday", "Wed", d.Settings.WorkWednesday},
		{"work_thursday", "Thu", d.Settings.WorkThursday},
		{"work_friday", "Fri", d.Settings.WorkFriday},
		{"work_saturday", "Sat", d.Settings.WorkSaturday},
		{"work_sunday", "Sun", d.Settings.WorkSunday},
	}
}

// monthName maps 1..12 to its English name (for the leave-year start select).
func monthName(m int) string {
	names := []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	if m < 1 || m > 12 {
		return ""
	}
	return names[m-1]
}

// ───────────────────────────── calendar grid ─────────────────────────────

// CalEntry is one approved leave shown on a day cell.
type CalEntry struct {
	Employee string
	Type     string
	Color    string
}

// CalDay is a single cell in the month grid.
type CalDay struct {
	Date    time.Time
	InMonth bool // false for leading/trailing days from adjacent months
	IsToday bool
	Holiday string // holiday name, or "" if none
	Entries []CalEntry
}

type CalendarData struct {
	Nav   Nav
	Year  int
	Month int // 1-12
	Weeks [][]CalDay
}

// ───────────────────────────── formatting ────────────────────────────────

func fmtDate(t time.Time) string { return t.Format("02 Jan 2006") }

// fmtDays renders a fractional day count without trailing zeros:
// 22 -> "22", 1.83 -> "1.83", 2.5 -> "2.5".
func fmtDays(d float64) string { return strconv.FormatFloat(d, 'f', -1, 64) }

// fmtRange renders a single day as one date and a multi-day span as "a – b".
func fmtRange(a, b time.Time) string {
	if a.Year() == b.Year() && a.YearDay() == b.YearDay() {
		return fmtDate(a)
	}
	return fmtDate(a) + " – " + fmtDate(b)
}

// orDash renders "—" for an empty string (e.g. an employee with no manager).
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// calAlpine builds the Alpine x-data object for the calendar shell. Month
// navigation is client-side (Alpine tracks year/month and computes the label);
// fetching each month's grid is HTMX (window.htmx.ajax swaps #cal-grid). This
// is the Phase-9 payoff: Alpine owns ephemeral UI state, HTMX owns server data.
func calAlpine(year, month int) string {
	return fmt.Sprintf(`{
		y: %d, m: %d,
		names: ['January','February','March','April','May','June','July','August','September','October','November','December'],
		label() { return this.names[this.m-1] + ' ' + this.y },
		load() { window.htmx.ajax('GET', '/calendar/month?year=' + this.y + '&month=' + this.m, { target: '#cal-grid' }) },
		prev() { this.m--; if (this.m < 1) { this.m = 12; this.y-- } this.load() },
		next() { this.m++; if (this.m > 12) { this.m = 1; this.y++ } this.load() }
	}`, year, month)
}

// statusClass maps a request status to Tailwind badge classes.
func statusClass(status string) string {
	switch status {
	case "approved":
		return "bg-emerald-100 text-emerald-700"
	case "rejected":
		return "bg-rose-100 text-rose-700"
	case "cancelled":
		return "bg-slate-100 text-slate-500"
	default: // pending
		return "bg-amber-100 text-amber-700"
	}
}
