// SPDX-License-Identifier: MIT
package format

import (
	"fmt"
	"time"
)

// RelativeTime renders d as a short relative string suitable for inline
// agent-facing output: "30s", "5m", "3h", "21d". Boundary rule: under one
// minute → seconds, under one hour → minutes, under one day → hours, else
// days. Negative durations clamp to "0s" so callers don't need to defend
// against clock skew. Used by STORY-EXPLAIN-001 for the freshness signal and
// cleared-blocker timing in `mote ls --ready --explain`.
func RelativeTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}
