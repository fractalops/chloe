package util

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"just now", 30 * time.Second, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48 * time.Hour, "2d"},
		{"months", 45 * 24 * time.Hour, "1mo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelativeTime(time.Now().Add(-tt.ago))
			if got != tt.want {
				t.Errorf("RelativeTime(-%v) = %q, want %q", tt.ago, got, tt.want)
			}
		})
	}
}
