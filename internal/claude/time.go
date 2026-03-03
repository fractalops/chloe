package claude

import (
	"fmt"
	"time"
)

// RelativeTime formats a time as a human-readable relative string.
func RelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		months := int(d.Hours() / 24 / 30)
		if months == 0 {
			months = 1
		}
		return fmt.Sprintf("%dmo", months)
	}
}
