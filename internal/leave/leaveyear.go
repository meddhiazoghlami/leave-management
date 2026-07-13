package leave

import "time"

// LeaveYearWindow returns the inclusive [start, end] date span of the leave year
// that contains ref, for a leave year beginning on the 1-based month startMonth
// (1 = January, i.e. the calendar-year default). label is the calendar year the
// window opens in — the value used to key per-year allocation rows.
//
// startMonth is clamped: anything outside 1..12 falls back to January.
//
// Examples (startMonth = 4, an April–March leave year):
//
//	ref 2026-06-15 -> [2026-04-01, 2027-03-31], label 2026
//	ref 2026-02-15 -> [2025-04-01, 2026-03-31], label 2025
func LeaveYearWindow(ref time.Time, startMonth int) (start, end time.Time, label int) {
	if startMonth < 1 || startMonth > 12 {
		startMonth = 1
	}
	startYear := ref.Year()
	// Before the start month we're still inside the leave year that opened the
	// previous calendar year.
	if int(ref.Month()) < startMonth {
		startYear--
	}
	start = time.Date(startYear, time.Month(startMonth), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(1, 0, 0).AddDate(0, 0, -1) // last day of the 12-month span
	return start, end, startYear
}
