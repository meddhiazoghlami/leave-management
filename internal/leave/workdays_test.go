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

	// A Sun–Thu working week (Fri/Sat weekend), as used across much of MENA/Gulf.
	friSatWeekend := map[time.Weekday]bool{
		time.Sunday:    true,
		time.Monday:    true,
		time.Tuesday:   true,
		time.Wednesday: true,
		time.Thursday:  true,
	}

	tests := []struct {
		name     string
		start    string
		end      string
		workweek map[time.Weekday]bool // nil -> DefaultWorkingWeek (Mon–Fri)
		holidays map[string]bool
		want     int
	}{
		{"single weekday", "2026-01-05", "2026-01-05", nil, nil, 1},        // Mon
		{"single weekend day", "2026-01-03", "2026-01-03", nil, nil, 0},    // Sat
		{"full week Mon-Fri", "2026-01-05", "2026-01-09", nil, nil, 5},     // Mon–Fri
		{"week incl weekend", "2026-01-05", "2026-01-11", nil, nil, 5},     // Mon–Sun -> 5
		{"spans two weeks", "2026-01-05", "2026-01-16", nil, nil, 10},      // two full work weeks
		{"holiday excluded", "2026-01-01", "2026-01-02", nil, holidays, 1}, // Thu(hol) + Fri -> 1
		{"reversed range", "2026-01-09", "2026-01-05", nil, nil, 0},        // end before start

		// Configurable weekend: the same Sun–Thu block counts differently under a
		// Fri/Sat weekend (5 working days) than under the Mon–Fri default (4).
		{"Fri/Sat weekend, Sun-Thu", "2026-01-04", "2026-01-08", friSatWeekend, nil, 5},
		{"Mon-Fri default, Sun-Thu", "2026-01-04", "2026-01-08", nil, nil, 4},
		// Fri+Sat are the days excluded from a full Sun–Sat span under Fri/Sat.
		{"Fri/Sat weekend, full span", "2026-01-04", "2026-01-10", friSatWeekend, nil, 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workweek := tc.workweek
			if workweek == nil {
				workweek = DefaultWorkingWeek()
			}
			got := WorkingDays(date(tc.start), date(tc.end), workweek, tc.holidays)
			if got != tc.want {
				t.Errorf("WorkingDays(%s..%s) = %d, want %d", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

// TestWorkingDays_EmptyWeek confirms that with no working weekdays configured,
// nothing counts — guards the nil/empty-map path documented on WorkingDays.
func TestWorkingDays_EmptyWeek(t *testing.T) {
	if got := WorkingDays(date("2026-01-05"), date("2026-01-09"), nil, nil); got != 0 {
		t.Fatalf("empty working week = %d, want 0", got)
	}
	if got := WorkingDays(date("2026-01-05"), date("2026-01-09"), map[time.Weekday]bool{}, nil); got != 0 {
		t.Fatalf("empty working week = %d, want 0", got)
	}
}

// TestWorkingWeek builds a Sun–Thu week (Fri/Sat weekend) from flags and
// confirms it drops into WorkingDays.
func TestWorkingWeek(t *testing.T) {
	// mon, tue, wed, thu, fri, sat, sun
	w := WorkingWeek(true, true, true, true, false, false, true)
	for _, d := range []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday} {
		if !w[d] {
			t.Errorf("expected %s to be a working day", d)
		}
	}
	for _, d := range []time.Weekday{time.Friday, time.Saturday} {
		if w[d] {
			t.Errorf("expected %s to be a weekend day", d)
		}
	}
	if len(w) != 5 {
		t.Errorf("WorkingWeek size = %d, want 5", len(w))
	}
	if got := WorkingDays(date("2026-01-04"), date("2026-01-08"), w, nil); got != 5 { // Sun–Thu
		t.Errorf("WorkingDays with a Sun–Thu week = %d, want 5", got)
	}
}

// TestDefaultWorkingWeek locks in the Mon–Fri default (Sat/Sun excluded).
func TestDefaultWorkingWeek(t *testing.T) {
	w := DefaultWorkingWeek()
	for _, d := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday} {
		if !w[d] {
			t.Errorf("DefaultWorkingWeek should include %s", d)
		}
	}
	for _, d := range []time.Weekday{time.Saturday, time.Sunday} {
		if w[d] {
			t.Errorf("DefaultWorkingWeek should exclude %s", d)
		}
	}
	if len(w) != 5 {
		t.Errorf("DefaultWorkingWeek size = %d, want 5", len(w))
	}
}

// TestWorkingDays_TimeOfDayIgnored confirms only the calendar date matters: the
// same day at different clock times still counts as one working day, and the
// holiday match ignores the time component.
func TestWorkingDays_TimeOfDayIgnored(t *testing.T) {
	start := time.Date(2026, 1, 5, 23, 30, 0, 0, time.UTC) // Mon late evening
	end := time.Date(2026, 1, 5, 1, 0, 0, 0, time.UTC)     // same Mon early morning
	if got := WorkingDays(start, end, DefaultWorkingWeek(), nil); got != 1 {
		t.Fatalf("same-day (different clock times) = %d, want 1", got)
	}

	// A holiday supplied with a time-of-day still matches the date-only key.
	hol := HolidaySet([]time.Time{time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)})
	if got := WorkingDays(date("2026-01-05"), date("2026-01-05"), DefaultWorkingWeek(), hol); got != 0 {
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
	if got := WorkingDays(date("2026-01-05"), date("2026-01-09"), DefaultWorkingWeek(), satHoliday); got != 5 {
		t.Fatalf("weekend holiday affected weekday count: got %d want 5", got)
	}

	// An empty input yields an empty (non-nil) set.
	if s := HolidaySet(nil); s == nil || len(s) != 0 {
		t.Fatalf("HolidaySet(nil) = %v, want empty non-nil", s)
	}
}
