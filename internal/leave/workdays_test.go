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
