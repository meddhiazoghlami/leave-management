package leave

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestWorkingDays(t *testing.T) {
	// 2026-01-01 is a Thursday, so the anchoring math is easy to verify by hand.
	holidays := HolidaySet([]time.Time{date("2026-01-01")}) // New Year's Day (Thu)

	tests := []struct {
		name     string
		start    string
		end      string
		holidays map[string]bool
		want     int
	}{
		{"single weekday", "2026-01-05", "2026-01-05", nil, 1},        // Mon
		{"single weekend day", "2026-01-03", "2026-01-03", nil, 0},    // Sat
		{"full week Mon-Fri", "2026-01-05", "2026-01-09", nil, 5},     // Mon–Fri
		{"week incl weekend", "2026-01-05", "2026-01-11", nil, 5},     // Mon–Sun -> 5
		{"spans two weeks", "2026-01-05", "2026-01-16", nil, 10},      // two full work weeks
		{"holiday excluded", "2026-01-01", "2026-01-02", holidays, 1}, // Thu(hol) + Fri -> 1
		{"reversed range", "2026-01-09", "2026-01-05", nil, 0},        // end before start
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := WorkingDays(date(tc.start), date(tc.end), tc.holidays)
			if got != tc.want {
				t.Errorf("WorkingDays(%s..%s) = %d, want %d", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

// TestWorkingDays_TimeOfDayIgnored confirms only the calendar date matters: the
// same day at different clock times still counts as one working day, and the
// holiday match ignores the time component.
func TestWorkingDays_TimeOfDayIgnored(t *testing.T) {
	start := time.Date(2026, 1, 5, 23, 30, 0, 0, time.UTC) // Mon late evening
	end := time.Date(2026, 1, 5, 1, 0, 0, 0, time.UTC)     // same Mon early morning
	if got := WorkingDays(start, end, nil); got != 1 {
		t.Fatalf("same-day (different clock times) = %d, want 1", got)
	}

	// A holiday supplied with a time-of-day still matches the date-only key.
	hol := HolidaySet([]time.Time{time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)})
	if got := WorkingDays(date("2026-01-05"), date("2026-01-05"), hol); got != 0 {
		t.Fatalf("holiday with clock time not excluded, got %d want 0", got)
	}
}

// TestHolidaySet dedupes by calendar date and ignores weekends' relevance (a
// holiday on a weekend simply never affects the count).
func TestHolidaySet(t *testing.T) {
	set := HolidaySet([]time.Time{
		time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC), // same date, different time
		date("2026-01-02"),
	})
	if len(set) != 2 {
		t.Fatalf("HolidaySet size = %d, want 2 (deduped)", len(set))
	}
	if !set["2026-01-01"] || !set["2026-01-02"] {
		t.Fatalf("expected both dates present, got %v", set)
	}

	// A holiday that lands on a Saturday doesn't change a Mon–Fri count.
	satHoliday := HolidaySet([]time.Time{date("2026-01-03")}) // Sat
	if got := WorkingDays(date("2026-01-05"), date("2026-01-09"), satHoliday); got != 5 {
		t.Fatalf("weekend holiday affected weekday count: got %d want 5", got)
	}

	// An empty input yields an empty (non-nil) set.
	if s := HolidaySet(nil); s == nil || len(s) != 0 {
		t.Fatalf("HolidaySet(nil) = %v, want empty non-nil", s)
	}
}
