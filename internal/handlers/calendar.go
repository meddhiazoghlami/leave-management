package handlers

import (
	"strconv"
	"time"

	"github.com/dzovi/leave-management/views"
	"github.com/gin-gonic/gin"
)

// Calendar renders the full page for the current month (Alpine shell + initial
// server-rendered grid).
func (h *Handlers) Calendar(c *gin.Context) {
	now := time.Now()
	data := h.buildCalendar(c, int(now.Year()), int(now.Month()))
	data.Nav = h.navFor(c, "calendar", "Calendar")
	render(c, 200, views.CalendarPage(data))
}

// CalendarMonthFragment returns just the month grid, used by the Alpine
// prev/next buttons via window.htmx.ajax.
func (h *Handlers) CalendarMonthFragment(c *gin.Context) {
	now := time.Now()
	year := atoiDefault(c.Query("year"), now.Year())
	month := atoiDefault(c.Query("month"), int(now.Month()))
	if month < 1 || month > 12 {
		month = int(now.Month())
	}
	render(c, 200, views.CalendarMonth(h.buildCalendar(c, year, month)))
}

// buildCalendar assembles the month grid: a Monday-first matrix of day cells,
// each annotated with today/holiday flags and any approved leave overlapping it.
func (h *Handlers) buildCalendar(c *gin.Context, year, month int) views.CalendarData {
	ctx := c.Request.Context()

	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := first.AddDate(0, 1, -1).Day()
	// Leading padding so the grid starts on a Monday (Go: Sunday=0..Saturday=6).
	offset := (int(first.Weekday()) + 6) % 7
	gridStart := first.AddDate(0, 0, -offset)
	numWeeks := (offset + daysInMonth + 6) / 7
	gridEnd := gridStart.AddDate(0, 0, numWeeks*7-1)

	approved, _ := h.Store.ListApprovedInRange(ctx, gridStart, gridEnd)
	holidays, _ := h.Store.ListHolidaysInRange(ctx, gridStart, gridEnd)

	holidayByDay := make(map[string]string, len(holidays))
	for _, hol := range holidays {
		holidayByDay[dayKey(hol.HolidayDate)] = hol.Name
	}
	todayKey := dayKey(time.Now())

	weeks := make([][]views.CalDay, 0, numWeeks)
	cur := gridStart
	for w := 0; w < numWeeks; w++ {
		week := make([]views.CalDay, 7)
		for i := 0; i < 7; i++ {
			key := dayKey(cur)
			day := views.CalDay{
				Date:    cur,
				InMonth: cur.Year() == year && int(cur.Month()) == month,
				IsToday: key == todayKey,
				Holiday: holidayByDay[key],
			}
			for _, a := range approved {
				if inRange(cur, a.StartDate, a.EndDate) {
					day.Entries = append(day.Entries, views.CalEntry{
						Employee: a.EmployeeName,
						Type:     a.LeaveTypeName,
						Color:    a.LeaveTypeColor,
					})
				}
			}
			week[i] = day
			cur = cur.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
	}
	return views.CalendarData{Year: year, Month: month, Weeks: weeks}
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// inRange reports whether day falls within [start, end] inclusive, comparing by
// calendar date only.
func inRange(day, start, end time.Time) bool {
	return dayKey(day) >= dayKey(start) && dayKey(day) <= dayKey(end)
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
