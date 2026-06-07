// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"motes/internal/format"
)

// TestShow_Header_PrettyActive_FirstLineIsTwoZone covers STORY-HDRZ-001
// Scenario 1: active mote, 100-col TTY, color on; first line of `mote show`
// output is the two-zone header.
func TestShow_Header_PrettyActive_FirstLineIsTwoZone(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("MOTE_FORCE_WIDTH", "100")
	t.Setenv("NO_COLOR", "")
	createDeterministicMote(t, root, "T1abc7", "Add login form")

	resetShowFlags()
	defer resetShowFlags()

	out := captureStdout(func() {
		// Force pretty (TTY+color) path by enabling --pretty so we don't
		// depend on stdout TTY detection (captureStdout pipes stdout).
		prettyFlag = true
		defer func() { prettyFlag = false }()
		format.SetColorEnabled(true)
		defer format.SetColorEnabled(false)
		if err := showCmd.RunE(showCmd, []string{"T1abc7"}); err != nil {
			t.Fatalf("runShow: %v", err)
		}
	})

	lines := strings.Split(out, "\n")
	if len(lines) == 0 {
		t.Fatal("no output captured")
	}
	first := lines[0]
	stripped := format.StripANSI(first)
	if !strings.HasPrefix(stripped, "○ T1abc7 · Add login form") {
		t.Fatalf("first line left zone: got %q", stripped)
	}
	if !strings.HasSuffix(stripped, "[○ ACTIVE w0.5]") {
		t.Fatalf("first line right zone: got %q", stripped)
	}
	if !strings.Contains(first, "\x1b[") {
		t.Fatalf("expected ANSI styling in TTY+color pretty mode, got %q", first)
	}
	// Verify the old flat key/value pair lines that have moved into the
	// header are no longer printed below it.
	for _, banned := range []string{"status:          active", "title:           Add login", "weight:          0.50"} {
		if strings.Contains(out, banned) {
			t.Errorf("redundant field %q should no longer appear below the header; got:\n%s", banned, out)
		}
	}
}

// TestShow_Header_ClosedMote_MutesLineAndDropsWeight covers Scenario 2.
func TestShow_Header_ClosedMote_MutesLineAndDropsWeight(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("MOTE_FORCE_WIDTH", "100")
	t.Setenv("NO_COLOR", "")
	createDeterministicMoteWithStatus(t, root, "T2def9", "completed", "Old work item")

	resetShowFlags()
	defer resetShowFlags()

	out := captureStdout(func() {
		prettyFlag = true
		defer func() { prettyFlag = false }()
		format.SetColorEnabled(true)
		defer format.SetColorEnabled(false)
		if err := showCmd.RunE(showCmd, []string{"T2def9"}); err != nil {
			t.Fatalf("runShow: %v", err)
		}
	})
	first := strings.Split(out, "\n")[0]
	stripped := format.StripANSI(first)
	if !strings.Contains(stripped, "[✓ COMPLETED]") {
		t.Fatalf("right zone: want '[✓ COMPLETED]' (no weight), got %q", stripped)
	}
	if strings.Contains(stripped, "w0.5") || strings.Contains(stripped, "w0.4") {
		t.Fatalf("closed mote header must not carry weight, got %q", stripped)
	}
	if !strings.HasPrefix(first, "\x1b[2m") {
		t.Fatalf("closed line should be wrapped in SGR dim, got prefix %q", first)
	}
}

// TestShow_Header_LongTitle_TruncatesPreservesRightZone covers Scenario 4.
func TestShow_Header_LongTitle_TruncatesPreservesRightZone(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("MOTE_FORCE_WIDTH", "80")
	t.Setenv("NO_COLOR", "")
	createDeterministicMote(t, root, "T4jkl5", "A title that is much longer than the available terminal width permits")

	resetShowFlags()
	defer resetShowFlags()

	out := captureStdout(func() {
		prettyFlag = true
		defer func() { prettyFlag = false }()
		format.SetColorEnabled(true)
		defer format.SetColorEnabled(false)
		if err := showCmd.RunE(showCmd, []string{"T4jkl5"}); err != nil {
			t.Fatalf("runShow: %v", err)
		}
	})
	first := strings.Split(out, "\n")[0]
	stripped := format.StripANSI(first)
	if !strings.HasSuffix(stripped, "[○ ACTIVE w0.5]") {
		t.Fatalf("right zone must survive truncation, got %q", stripped)
	}
	if !strings.Contains(stripped, "…") {
		t.Fatalf("left zone should be truncated with ellipsis, got %q", stripped)
	}
}

// TestShow_Header_PlainMode_ASCIISeparator covers Scenario 5.
func TestShow_Header_PlainMode_ASCIISeparator(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	createDeterministicMote(t, root, "T1abc7", "Add login form")

	resetShowFlags()
	plainFlag = true
	defer func() {
		resetShowFlags()
		plainFlag = false
	}()

	out := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{"T1abc7"}); err != nil {
			t.Fatalf("runShow: %v", err)
		}
	})
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("--plain must not contain ANSI; got:\n%s", out)
	}
	first := strings.Split(out, "\n")[0]
	want := "o T1abc7 - Add login form" + format.PlainSeparator + "[o ACTIVE w0.5]"
	if first != want {
		t.Fatalf("plain header:\n  got:  %q\n  want: %q", first, want)
	}
	// type should still appear below the header as its own field.
	if !strings.Contains(out, "type: task") {
		t.Errorf("plain output should still print `type: task` below the header; got:\n%s", out)
	}
	// The four header-redundant fields must NOT appear as standalone lines.
	for _, banned := range []string{"id: T1abc7", "status: active", "title: Add login form", "weight: 0.50"} {
		if strings.Contains(out, banned+"\n") {
			t.Errorf("redundant standalone line %q should not appear in plain mode; got:\n%s", banned, out)
		}
	}
}

// TestShow_Header_JSONMode_NoANSI_NoHeaderField covers Scenario 6:
// --json output is unaffected by the header refactor — no ANSI escapes,
// no `header` field, and the document parses as JSON.
func TestShow_Header_JSONMode_NoANSI_NoHeaderField(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	createDeterministicMote(t, root, "T1abc7", "Add login form")

	resetShowFlags()
	showJSON = true
	defer resetShowFlags()

	out := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{"T1abc7"}); err != nil {
			t.Fatalf("runShow: %v", err)
		}
	})
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("--json must not contain ANSI; got:\n%s", out)
	}
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("no JSON object found in output:\n%s", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out[idx:]), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	// Walk envelope if present so the check works in both legacy and
	// envelope JSON modes.
	payload := parsed
	if data, ok := parsed["data"].(map[string]any); ok {
		payload = data
	}
	if _, present := payload["header"]; present {
		t.Errorf("JSON payload must not include a presentation-layer 'header' field; got: %v", payload)
	}
	for _, key := range []string{"id", "status", "title", "weight"} {
		if _, present := payload[key]; !present {
			t.Errorf("JSON payload missing expected key %q; got: %v", key, payload)
		}
	}
}

// TestShow_Header_ScopeGuard covers Scenario 7: other command files must
// not call RenderHeader. This is a static analysis over source bytes —
// the bar is intentionally low (substring match) since RenderHeader is
// the only API anyone could reach for this purpose.
func TestShow_Header_ScopeGuard(t *testing.T) {
	forbidden := []string{
		"cmd_pulse.go",
		"cmd_constellation.go",
		"cmd_context.go",
	}
	for _, path := range forbidden {
		t.Run(path, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("%s not present: %v", path, err)
				return
			}
			if bytes.Contains(src, []byte("RenderHeader(")) {
				t.Fatalf("%s must not call format.RenderHeader — STORY-HDRZ-001 scope is mote show + mote ls only", path)
			}
		})
	}
}
