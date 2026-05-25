// SPDX-License-Identifier: MIT
package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxTimeSpecDistance caps how far in the future a parsed time spec may
// resolve. The bound exists for two reasons:
//   - It bounds parser cost (no arithmetic on huge integers).
//   - It bounds serialization size (RFC3339 timestamps stay readable).
//
// 10 years was chosen per STORY-TIME-001 Q4. Caller may compare to an
// already-parsed time; the bound is enforced relative to the `now` argument
// passed in to ParseTimeSpec.
const maxTimeSpecDistance = 10 * 365 * 24 * time.Hour

// ErrInvalidTime is the sentinel returned for every rejected input. Callers
// that need to distinguish parser errors from other errors use errors.Is.
var ErrInvalidTime = errors.New("invalid time")

// allowedRunes are the characters permitted in a time-spec input string.
// Anything outside this set (shell metacharacters, path separators,
// punctuation, Unicode controls, …) is rejected before any parsing happens.
//
// Format `T` is needed for RFC3339 ("2026-12-01T10:00:00Z"); `Z` and `+`/`-`
// for the offset; `:` for time components.
func isAllowedRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '+' || r == '-' || r == ':' || r == ' ':
		return true
	}
	return false
}

// weekdayByName maps full English day names (lowercase) to time.Weekday.
var weekdayByName = map[string]time.Weekday{
	"sunday":    time.Sunday,
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
}

// ParseTimeSpec parses a user-supplied time string against the allowlist
// documented in STORY-TIME-001. It returns an RFC3339 instant (in UTC for
// downstream serialization) or wraps ErrInvalidTime on any rejection.
//
// The parser is the security boundary for --due / --defer. It must:
//   - never panic on any input
//   - reject shell metacharacters and path-traversal sequences
//   - reject Unicode bidi controls before character allowlisting
//   - reject negative relative forms (e.g. "-1h")
//   - reject anything that resolves more than 10 years from `now`
//
// `now` is injected to keep the function deterministic under test. Callers in
// production pass time.Now(). The function treats `now.Location()` as the
// reference local zone for natural-language inputs ("tomorrow", "next monday"),
// matching beads' behavior and what a CLI user expects.
func ParseTimeSpec(input string, now time.Time) (time.Time, error) {
	if input == "" {
		return time.Time{}, fmt.Errorf("%w: empty input", ErrInvalidTime)
	}
	if len(input) > 64 {
		return time.Time{}, fmt.Errorf("%w: input too long", ErrInvalidTime)
	}

	// Reject Unicode bidi controls BEFORE the ASCII allowlist so we don't
	// silently treat them as "outside allowed runes" (the error message
	// would be confusing). These can spoof rendering in terminals/editors.
	for _, r := range input {
		switch r {
		case '‪', '‫', '‬', '‭', '‮',
			'⁦', '⁧', '⁨', '⁩':
			return time.Time{}, fmt.Errorf("%w: contains unicode bidi control", ErrInvalidTime)
		}
	}

	// Path-traversal defense in depth: `..` has no legitimate use in a time
	// spec but is the classic CLI smuggling sequence.
	if strings.Contains(input, "..") {
		return time.Time{}, fmt.Errorf("%w: contains path-traversal sequence", ErrInvalidTime)
	}

	// ASCII allowlist.
	for _, r := range input {
		if !isAllowedRune(r) {
			return time.Time{}, fmt.Errorf("%w: disallowed character %q", ErrInvalidTime, r)
		}
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("%w: empty input", ErrInvalidTime)
	}

	lower := strings.ToLower(trimmed)
	var resolved time.Time
	switch {
	case lower == "now":
		resolved = now
	case lower == "tomorrow":
		resolved = nextLocalMidnight(now, 1)
	case strings.HasPrefix(lower, "next "):
		rest := strings.TrimSpace(lower[len("next "):])
		wd, ok := weekdayByName[rest]
		if !ok {
			return time.Time{}, fmt.Errorf("%w: unsupported natural-language form %q", ErrInvalidTime, trimmed)
		}
		resolved = nextWeekdayMidnight(now, wd)
	case strings.HasPrefix(trimmed, "+"):
		d, err := parseRelativeDuration(trimmed[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: %v", ErrInvalidTime, err)
		}
		resolved = now.Add(d)
	case strings.HasPrefix(trimmed, "-"):
		return time.Time{}, fmt.Errorf("%w: negative relative durations are not allowed", ErrInvalidTime)
	default:
		// Fall through to absolute forms. Try RFC3339 first (most specific),
		// then date-only.
		if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
			resolved = t
		} else if t, err := time.ParseInLocation("2006-01-02", trimmed, now.Location()); err == nil {
			resolved = t
		} else {
			return time.Time{}, fmt.Errorf("%w: unrecognized format %q", ErrInvalidTime, trimmed)
		}
	}

	// Enforce the +10y cap relative to now. Past times are allowed (the
	// caller decides whether past is meaningful — past-due is back-dating,
	// past-defer is rejected at the caller layer).
	if resolved.Sub(now) > maxTimeSpecDistance {
		return time.Time{}, fmt.Errorf("%w: more than 10 years in the future", ErrInvalidTime)
	}

	return resolved.UTC(), nil
}

// parseRelativeDuration handles the body of "+Nh" / "+Nd" etc., already
// stripped of the leading "+". `N` must be a positive integer, and the
// resulting duration must not overflow int64. Per-unit caps are set well
// above the downstream 10-year window so the bound itself is the rejection
// point in normal use; the caps exist to keep the arithmetic safe.
func parseRelativeDuration(body string) (time.Duration, error) {
	if body == "" {
		return 0, fmt.Errorf("missing duration")
	}
	unit := body[len(body)-1]
	numStr := body[:len(body)-1]
	if numStr == "" {
		return 0, fmt.Errorf("missing numeric portion")
	}
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("non-integer duration %q", body)
	}
	if n <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %d", n)
	}
	// Per-unit safety caps. 10y = 3650 days = 87600 hours = 5_256_000 min
	// = 521.43 weeks. We pad by ~10x to leave room for the 10y check to
	// produce the user-visible rejection message rather than this one.
	switch unit {
	case 'm':
		if n > 60_000_000 {
			return 0, fmt.Errorf("duration too large")
		}
		return time.Duration(n) * time.Minute, nil
	case 'h':
		if n > 1_000_000 {
			return 0, fmt.Errorf("duration too large")
		}
		return time.Duration(n) * time.Hour, nil
	case 'd':
		if n > 40_000 {
			return 0, fmt.Errorf("duration too large")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		if n > 6_000 {
			return 0, fmt.Errorf("duration too large")
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("unknown unit %q (expected m|h|d|w)", string(unit))
}

// nextLocalMidnight returns 00:00 in now.Location() of the day `days` after
// now's calendar date.
func nextLocalMidnight(now time.Time, days int) time.Time {
	loc := now.Location()
	t := now.In(loc).AddDate(0, 0, days)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// nextWeekdayMidnight returns 00:00 in now.Location() on the NEXT occurrence
// of the given weekday, exclusive (i.e., if today is Monday, "next monday" is
// 7 days from now, not 0).
func nextWeekdayMidnight(now time.Time, target time.Weekday) time.Time {
	loc := now.Location()
	today := now.In(loc).Weekday()
	delta := int(target) - int(today)
	if delta <= 0 {
		delta += 7
	}
	return nextLocalMidnight(now, delta)
}
