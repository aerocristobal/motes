// SPDX-License-Identifier: MIT
//
// STORY-TIME-001 §6.1 — core.ParseTimeSpec scaffold.
//
// ParseTimeSpec is the security boundary for --due / --defer. These tests
// pin down each accepted form (happy + edge) and each rejected form
// (error/adversarial). The fuzz target at the bottom must complete with no
// panic on any input.
package core

import (
	"strings"
	"testing"
	"time"
)

// t0 is the reference instant for every test. It is intentionally pinned to a
// Monday in a known location so weekday math is reproducible. 2026-05-25 is a
// Monday. We use a fixed offset zone so DST never confounds the assertions.
var t0 = time.Date(2026, 5, 25, 10, 30, 0, 0, time.FixedZone("test", 5*3600))

// --- Happy paths --------------------------------------------------------

func TestParseTimeSpec_RelativeHours(t *testing.T) {
	got, err := ParseTimeSpec("+6h", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := t0.Add(6 * time.Hour).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_RelativeDays(t *testing.T) {
	got, err := ParseTimeSpec("+1d", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := t0.Add(24 * time.Hour).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_RelativeWeeks(t *testing.T) {
	got, err := ParseTimeSpec("+1w", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := t0.Add(7 * 24 * time.Hour).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_RelativeMinutes(t *testing.T) {
	got, err := ParseTimeSpec("+30m", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := t0.Add(30 * time.Minute).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_AbsoluteRFC3339(t *testing.T) {
	got, err := ParseTimeSpec("2026-12-01T10:00:00Z", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_AbsoluteDateOnly(t *testing.T) {
	got, err := ParseTimeSpec("2026-12-01", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Date-only is interpreted as 00:00 in now.Location(). t0 is in a
	// FixedZone("test", +5h), so 2026-12-01 00:00 local == 2026-11-30 19:00 UTC.
	want := time.Date(2026, 12, 1, 0, 0, 0, 0, t0.Location()).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_NaturalTomorrow(t *testing.T) {
	got, err := ParseTimeSpec("tomorrow", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// t0 is 2026-05-25 10:30 local; tomorrow at 00:00 local is 2026-05-26.
	want := time.Date(2026, 5, 26, 0, 0, 0, 0, t0.Location()).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_NaturalNextWeekday(t *testing.T) {
	// t0 is a Monday. "next monday" must be 7 days later (exclusive), not 0.
	got, err := ParseTimeSpec("next monday", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 6, 1, 0, 0, 0, 0, t0.Location()).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// "next tuesday" is the day after Monday.
	got, err = ParseTimeSpec("next tuesday", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want = time.Date(2026, 5, 26, 0, 0, 0, 0, t0.Location()).UTC()
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimeSpec_NowLiteral(t *testing.T) {
	got, err := ParseTimeSpec("now", t0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !got.Equal(t0.UTC()) {
		t.Errorf("got %v, want %v", got, t0.UTC())
	}
}

// --- Edge / boundary ----------------------------------------------------

func TestParseTimeSpec_CapAt10Years(t *testing.T) {
	// Just under 10 years should pass.
	if _, err := ParseTimeSpec("+520w", t0); err != nil {
		t.Errorf("520w (< 10y) should be accepted, got %v", err)
	}
	// 11 years out should fail.
	_, err := ParseTimeSpec("2037-12-31T00:00:00Z", t0)
	if err == nil {
		t.Error("expected rejection past 10-year cap")
	}
	if err != nil && !strings.Contains(err.Error(), "10 years") {
		t.Errorf("expected 10-years error, got %v", err)
	}
}

func TestParseTimeSpec_LocalMidnightForNaturalDate(t *testing.T) {
	// "tomorrow" must resolve to 00:00 LOCAL, not 00:00 UTC.
	got, _ := ParseTimeSpec("tomorrow", t0)
	localized := got.In(t0.Location())
	if localized.Hour() != 0 || localized.Minute() != 0 || localized.Second() != 0 {
		t.Errorf("expected 00:00 in t0.Location(), got %v", localized)
	}
}

// --- Error / adversarial input -----------------------------------------

func TestParseTimeSpec_RejectNegativeRelative(t *testing.T) {
	for _, in := range []string{"-1h", "-1d", "-1w", "-30m"} {
		if _, err := ParseTimeSpec(in, t0); err == nil {
			t.Errorf("%q should be rejected", in)
		}
	}
}

func TestParseTimeSpec_RejectInvalidString(t *testing.T) {
	for _, in := range []string{"yesterday", "tomrrow", "not a date", "next foobar"} {
		if _, err := ParseTimeSpec(in, t0); err == nil {
			t.Errorf("%q should be rejected", in)
		}
	}
}

func TestParseTimeSpec_RejectShellMetacharacters(t *testing.T) {
	for _, in := range []string{
		"$(rm -rf ~)",
		"`whoami`",
		"+1d; echo pwned",
		"+1d && ls",
		"+1d | cat",
		"<script>",
		"'OR'1'='1",
	} {
		if _, err := ParseTimeSpec(in, t0); err == nil {
			t.Errorf("%q should be rejected (shell metacharacter)", in)
		}
	}
}

func TestParseTimeSpec_RejectPathTraversal(t *testing.T) {
	for _, in := range []string{
		"../../etc/passwd",
		"..\\..\\windows\\system32",
		"foo..bar",
	} {
		if _, err := ParseTimeSpec(in, t0); err == nil {
			t.Errorf("%q should be rejected (path traversal)", in)
		}
	}
}

func TestParseTimeSpec_RejectUnicodeBidi(t *testing.T) {
	// Each of these contains a Unicode bidi control codepoint.
	for _, in := range []string{
		"+1d‮",
		"‪+1d",
		"tomorrow⁦",
	} {
		if _, err := ParseTimeSpec(in, t0); err == nil {
			t.Errorf("%q should be rejected (unicode bidi)", in)
		}
	}
}

func TestParseTimeSpec_RejectEmptyExceptForDeferClear(t *testing.T) {
	// Empty is the caller's signal for "clear" in the CLI layer; the parser
	// itself never accepts empty.
	if _, err := ParseTimeSpec("", t0); err == nil {
		t.Error("empty input should be rejected")
	}
	if _, err := ParseTimeSpec("   ", t0); err == nil {
		t.Error("whitespace-only input should be rejected")
	}
}

func TestParseTimeSpec_RejectImpossibleRelative(t *testing.T) {
	// +999999d is past the 10y cap.
	if _, err := ParseTimeSpec("+999999d", t0); err == nil {
		t.Error("+999999d should be rejected")
	}
	// Cap on the parser's intermediate arithmetic.
	if _, err := ParseTimeSpec("+99999999999w", t0); err == nil {
		t.Error("huge integer should be rejected")
	}
}

func TestParseTimeSpec_RejectMissingNumeric(t *testing.T) {
	for _, in := range []string{"+", "+h", "+d", "+m"} {
		if _, err := ParseTimeSpec(in, t0); err == nil {
			t.Errorf("%q should be rejected", in)
		}
	}
}

// --- Fuzz target -------------------------------------------------------
//
// The story DoD requires that ParseTimeSpec never panic on any input across
// at least 10k iterations. The fuzz target is seeded with the adversarial
// strings above plus a handful of accepted ones.

func FuzzParseTimeSpec(f *testing.F) {
	for _, seed := range []string{
		"+6h", "+1d", "+1w", "+30m",
		"tomorrow", "next monday", "next sunday",
		"2026-12-01", "2026-12-01T10:00:00Z",
		"yesterday", "-1d", "tomrrow", "+999999d", "not a date",
		"$(rm -rf ~)", "../../etc/passwd", "‪+1d",
		"", "   ",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		// We don't care about the result — only that the call returns
		// without panicking.
		_, _ = ParseTimeSpec(in, t0)
	})
}
