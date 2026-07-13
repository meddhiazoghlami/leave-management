// Package leave holds the domain's pure business rules — logic that doesn't
// touch the database or HTTP and is therefore trivially unit-testable. Right
// now that's the working-days calculation used to size a leave request.
package leave

import "time"

// dateKey formats a time as the YYYY-MM-DD used to key the holiday set. Only the
// calendar date matters; any time-of-day / location component is ignored.
func dateKey(t time.Time) string { return t.Format("2006-01-02") }

// HolidaySet builds a lookup set of public-holiday dates from a list of times.
func HolidaySet(dates []time.Time) map[string]bool {
	set := make(map[string]bool, len(dates))
	for _, d := range dates {
		set[dateKey(d)] = true
	}
	return set
}

// DefaultWorkingWeek is the Mon–Fri working week — the policy baked in until
// per-company working schedules land (see docs/next-steps.md, M1). Callers pass
// it into WorkingDays; once schedules are configurable this is replaced by the
// weekday set loaded from the DB. Returns a fresh map so callers can't mutate a
// shared instance.
func DefaultWorkingWeek() map[time.Weekday]bool {
	return map[time.Weekday]bool{
		time.Monday:    true,
		time.Tuesday:   true,
		time.Wednesday: true,
		time.Thursday:  true,
		time.Friday:    true,
	}
}

// WorkingWeek builds the set of working weekdays from seven Monday-first flags.
// It's the bridge from the company_settings work_* columns to the set that
// WorkingDays expects; only the days flagged true end up in the set.
func WorkingWeek(mon, tue, wed, thu, fri, sat, sun bool) map[time.Weekday]bool {
	flags := map[time.Weekday]bool{
		time.Monday:    mon,
		time.Tuesday:   tue,
		time.Wednesday: wed,
		time.Thursday:  thu,
		time.Friday:    fri,
		time.Saturday:  sat,
		time.Sunday:    sun,
	}
	week := make(map[time.Weekday]bool, len(flags))
	for d, on := range flags {
		if on {
			week[d] = true
		}
	}
	return week
}

// WorkingDays counts the working days in the inclusive range [start, end]: days
// whose weekday is in workingWeekdays and that are not in the holidays set.
// A nil/empty workingWeekdays means no day counts (0). Returns 0 if end is
// before start. Comparison is by calendar date, so the clock component of the
// inputs is irrelevant.
func WorkingDays(start, end time.Time, workingWeekdays map[time.Weekday]bool, holidays map[string]bool) int {
	// Normalise to midnight UTC so iterating by 24h lands on each calendar day
	// exactly once (no DST drift within the loop).
	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	count := 0
	for !cur.After(last) {
		if isWorkday(cur, workingWeekdays, holidays) {
			count++
		}
		cur = cur.AddDate(0, 0, 1)
	}
	return count
}

func isWorkday(d time.Time, workingWeekdays map[time.Weekday]bool, holidays map[string]bool) bool {
	return workingWeekdays[d.Weekday()] && !holidays[dateKey(d)]
}
