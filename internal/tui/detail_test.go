package tui

import "testing"

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1_000, "1.0K"},
		{1_500, "1.5K"},
		{1_000_000, "1.0M"},
		{2_500_000, "2.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.n)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "—"},
		{5_000, "5s"},
		{65_000, "1m 5s"},
		{3_665_000, "1h 1m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.ms)
		if got != tt.want {
			t.Errorf("formatDuration(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}
