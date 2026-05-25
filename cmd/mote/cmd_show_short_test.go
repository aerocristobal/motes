// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"motes/internal/core"
)

// Scenario 2: --short emits a single non-blank line in the required format.
func TestRenderShort_FormatAndContent(t *testing.T) {
	m := &core.Mote{
		ID:     "proj-T1ABC",
		Type:   "task",
		Status: "active",
		Weight: 0.5,
		Title:  "Add login flow",
	}
	got := renderShort(m, false)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly one line, got %d: %q", len(lines), got)
	}
	line := lines[0]
	if !strings.HasPrefix(line, "○ ") {
		t.Errorf("missing/wrong icon: %q", line)
	}
	if !strings.Contains(line, "proj-T1ABC") {
		t.Errorf("missing ID: %q", line)
	}
	if !regexp.MustCompile(`\b0\.50\b`).MatchString(line) {
		t.Errorf("missing weight in two-decimal form: %q", line)
	}
	if !strings.Contains(line, "[task]") {
		t.Errorf("missing [type]: %q", line)
	}
	if !strings.HasSuffix(line, "Add login flow") {
		t.Errorf("title not at end: %q", line)
	}
}

// Scenario 2: --short bounds line length even for pathological titles.
func TestRenderShort_TitleTruncation(t *testing.T) {
	m := &core.Mote{
		ID: "proj-T1ABC", Type: "task", Status: "active", Weight: 0.5,
		Title: strings.Repeat("x", 200),
	}
	got := renderShort(m, false)
	if len(got) > 100 {
		t.Errorf("short line should be bounded; got len %d:\n%s", len(got), got)
	}
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), "...") {
		t.Errorf("truncated title should end with ellipsis; got %q", got)
	}
}

// Scenario 4: status icon reflects lifecycle state.
func TestRenderShort_IconPerStatus(t *testing.T) {
	tests := []struct {
		status   string
		wantIcon string
	}{
		{"active", "○"},
		{"in_progress", "◐"},
		{"completed", "✓"},
		{"archived", "●"},
		{"deprecated", "❄"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			m := &core.Mote{
				ID: "x-1", Type: "task", Status: tt.status, Weight: 0.5, Title: "t",
			}
			got := renderShort(m, false)
			if !strings.HasPrefix(got, tt.wantIcon+" ") {
				t.Errorf("status=%s: want icon %q, got %q", tt.status, tt.wantIcon, got)
			}
		})
	}
}

// Scenario 4: deprecated status renders with the snowflake icon.
func TestRenderShort_DeprecatedShowsRecededIcon(t *testing.T) {
	m := &core.Mote{
		ID: "proj-D1XYZ", Type: "decision", Status: "deprecated",
		Weight: 0.1, Title: "Old auth approach",
	}
	got := renderShort(m, false)
	if !strings.HasPrefix(got, "❄ ") {
		t.Errorf("deprecated should use snowflake icon; got %q", got)
	}
}

// Scenario 4: --ascii forces ASCII glyphs.
func TestRenderShort_ASCIIMode(t *testing.T) {
	m := &core.Mote{
		ID: "x-1", Type: "task", Status: "active", Weight: 0.5, Title: "t",
	}
	got := renderShort(m, true)
	if !strings.HasPrefix(got, "o ") {
		t.Errorf("ASCII active icon should be 'o'; got %q", got)
	}
}

// Scenario 6: --short --json contains exactly the five top-level keys.
func TestShow_ShortJSON_HasExactKeySet(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, err := mm.Create("task", "Add login flow", core.CreateOpts{Body: "body", Local: true, Weight: 0.5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resetShowFlags()
	showShort = true
	showJSON = true
	defer resetShowFlags()

	out := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{m.ID}); err != nil {
			t.Fatalf("runShow: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse JSON: %v\noutput: %s", err, out)
	}

	want := map[string]bool{"id": true, "status": true, "type": true, "weight": true, "title": true}
	for k := range parsed {
		if !want[k] {
			t.Errorf("--short --json contains unexpected key %q (full keys: %v)", k, keysOf(parsed))
		}
	}
	for k := range want {
		if _, ok := parsed[k]; !ok {
			t.Errorf("--short --json missing key %q (full keys: %v)", k, keysOf(parsed))
		}
	}
}

// Scenario 9: --short does NOT increment access_count via the access batch.
// Loop iteration over many ready motes would otherwise inflate counts and
// skew weight decay; --short is explicitly loop-pure.
func TestShow_Short_NoAccessBatchAppend(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, err := mm.Create("task", "Loop target", core.CreateOpts{Body: "body", Local: true, Weight: 0.5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resetShowFlags()
	showShort = true
	defer resetShowFlags()

	for i := 0; i < 20; i++ {
		_ = captureStdout(func() {
			if err := showCmd.RunE(showCmd, []string{m.ID}); err != nil {
				t.Fatalf("runShow iter %d: %v", i, err)
			}
		})
	}

	// Force the access batch to flush (if anything was appended) by reading
	// the mote and inspecting its persisted state.
	after, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("read after loop: %v", err)
	}
	if after.AccessCount != m.AccessCount {
		t.Errorf("access_count should be unchanged after 20 --short calls; before=%d after=%d",
			m.AccessCount, after.AccessCount)
	}
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
