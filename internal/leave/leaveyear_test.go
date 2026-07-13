package leave

import "testing"

func TestLeaveYearWindow(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		startMonth int
		wantStart  string
		wantEnd    string
		wantLabel  int
	}{
		{"calendar year, mid", "2026-06-15", 1, "2026-01-01", "2026-12-31", 2026},
		{"calendar year, first day", "2026-01-01", 1, "2026-01-01", "2026-12-31", 2026},
		{"april start, after boundary", "2026-06-15", 4, "2026-04-01", "2027-03-31", 2026},
		{"april start, before boundary", "2026-02-15", 4, "2025-04-01", "2026-03-31", 2025},
		{"april start, on boundary", "2026-04-01", 4, "2026-04-01", "2027-03-31", 2026},
		{"clamp: month 0 -> january", "2026-06-15", 0, "2026-01-01", "2026-12-31", 2026},
		{"clamp: month 13 -> january", "2026-06-15", 13, "2026-01-01", "2026-12-31", 2026},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end, label := LeaveYearWindow(date(tc.ref), tc.startMonth)
			if got := start.Format("2006-01-02"); got != tc.wantStart {
				t.Errorf("start = %s, want %s", got, tc.wantStart)
			}
			if got := end.Format("2006-01-02"); got != tc.wantEnd {
				t.Errorf("end = %s, want %s", got, tc.wantEnd)
			}
			if label != tc.wantLabel {
				t.Errorf("label = %d, want %d", label, tc.wantLabel)
			}
		})
	}
}
