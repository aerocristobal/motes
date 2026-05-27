// SPDX-License-Identifier: MIT
//
// STORY-EXPLAIN-001 — short relative-time formatter used by the --explain
// flag. The contract is intentionally narrow: a single coarse unit per
// duration, no decimals, no "ago" suffix (callers add it).
package format

import (
	"testing"
	"time"
)

func TestRelativeTime_Seconds(t *testing.T) {
	if got := RelativeTime(30 * time.Second); got != "30s" {
		t.Fatalf("want %q, got %q", "30s", got)
	}
}

func TestRelativeTime_Minutes(t *testing.T) {
	if got := RelativeTime(5 * time.Minute); got != "5m" {
		t.Fatalf("want %q, got %q", "5m", got)
	}
}

func TestRelativeTime_Hours(t *testing.T) {
	if got := RelativeTime(3 * time.Hour); got != "3h" {
		t.Fatalf("want %q, got %q", "3h", got)
	}
}

func TestRelativeTime_Days(t *testing.T) {
	if got := RelativeTime(2 * 24 * time.Hour); got != "2d" {
		t.Fatalf("want %q, got %q", "2d", got)
	}
}

func TestRelativeTime_Zero(t *testing.T) {
	if got := RelativeTime(0); got != "0s" {
		t.Fatalf("want %q, got %q", "0s", got)
	}
}

func TestRelativeTime_NegativeClampsToZero(t *testing.T) {
	if got := RelativeTime(-5 * time.Second); got != "0s" {
		t.Fatalf("negative duration must clamp to 0s; got %q", got)
	}
}

func TestRelativeTime_BoundaryMinuteFlipsToMinutes(t *testing.T) {
	// 60s exactly should render as 1m, not 60s — the < case is exclusive.
	if got := RelativeTime(time.Minute); got != "1m" {
		t.Fatalf("60s boundary should render as 1m, got %q", got)
	}
}

func TestRelativeTime_BoundaryHourFlipsToHours(t *testing.T) {
	if got := RelativeTime(time.Hour); got != "1h" {
		t.Fatalf("3600s boundary should render as 1h, got %q", got)
	}
}

func TestRelativeTime_BoundaryDayFlipsToDays(t *testing.T) {
	if got := RelativeTime(24 * time.Hour); got != "1d" {
		t.Fatalf("24h boundary should render as 1d, got %q", got)
	}
}

func TestRelativeTime_LargeDays(t *testing.T) {
	if got := RelativeTime(21 * 24 * time.Hour); got != "21d" {
		t.Fatalf("want %q, got %q", "21d", got)
	}
}
