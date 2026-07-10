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

// WorkingDays counts the working days in the inclusive range [start, end]:
// weekdays (Mon–Fri) that are not in the holidays set. Returns 0 if end is
// before start. Comparison is by calendar date, so the clock component of the
// inputs is irrelevant.
func WorkingDays(start, end time.Time, holidays map[string]bool) int {
	// Normalise to midnight UTC so iterating by 24h lands on each calendar day
	// exactly once (no DST drift within the loop).
	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	count := 0
	for !cur.After(last) {
		if isWorkday(cur, holidays) {
			count++
		}
		cur = cur.AddDate(0, 0, 1)
	}
	return count
}

func isWorkday(d time.Time, holidays map[string]bool) bool {
	switch d.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	return !holidays[dateKey(d)]
}
