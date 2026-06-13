package components

import (
	"fmt"

	"github.com/nkhine/gohtmxelm/demo/internal/stopwatch"
)

// statusText is the running/paused label used both as text and as a CSS class.
func statusText(s stopwatch.Snapshot) string {
	if s.Running {
		return "running"
	}
	return "paused"
}

// dialStyle returns the CSS custom properties that drive the four dial rings.
func dialStyle(ms int64) string {
	sub, sec, min, hour := stopwatch.Dials(ms)
	return fmt.Sprintf("--d-sub:%.2fdeg;--d-sec:%.2fdeg;--d-min:%.2fdeg;--d-hour:%.2fdeg", sub, sec, min, hour)
}

// lapIndex formats a lap number as "#N".
func lapIndex(n int) string {
	return fmt.Sprintf("#%d", n)
}
