// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"motes/internal/format"
)

// TestLs_Header_ThreeReadyMotes_RightZoneVerticallyAligned covers
// STORY-HDRZ-001 Scenario 3: three two-zone rows whose right-zone
// brackets all start at the same visible column.
func TestLs_Header_ThreeReadyMotes_RightZoneVerticallyAligned(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("MOTE_FORCE_WIDTH", "100")
	t.Setenv("NO_COLOR", "")

	// Three active motes — --ready filters out closed and in_progress, so
	// all three are active to keep the test focused on layout, not on the
	// readiness filter semantics.
	createDeterministicMote(t, root, "T1abc7", "Add login form")
	createDeterministicMoteWithStatus(t, root, "T3ghi3", "active", "Wire up auth flow")
	createDeterministicMoteWithStatus(t, root, "T5mno8", "active", "Mid-length title")

	// Trip the ls path. Reset all per-command flags up-front so leakage
	// from sibling tests does not pollute filter state.
	resetLsFlagsForHeader()
	defer resetLsFlagsForHeader()
	lsReady = true
	prettyFlag = true
	defer func() { prettyFlag = false }()
	format.SetColorEnabled(true)
	defer format.SetColorEnabled(false)

	out := captureStdout(func() {
		if err := lsCmd.RunE(lsCmd, nil); err != nil {
			t.Fatalf("runLs: %v", err)
		}
	})

	var lines []string
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if l == "" {
			continue
		}
		lines = append(lines, l)
	}
	if len(lines) != 3 {
		t.Fatalf("want 3 rendered lines, got %d:\n%s", len(lines), out)
	}

	var rightZoneCol = -1
	for i, line := range lines {
		stripped := format.StripANSI(line)
		bracketIdx := strings.LastIndex(stripped, "[")
		if bracketIdx < 0 {
			t.Fatalf("line %d: no right-zone bracket in %q", i, stripped)
		}
		if rightZoneCol < 0 {
			rightZoneCol = bracketIdx
			continue
		}
		if bracketIdx != rightZoneCol {
			t.Fatalf("line %d: right zone at col %d, want col %d for vertical alignment\nlines:\n%s",
				i, bracketIdx, rightZoneCol, strings.Join(lines, "\n"))
		}
	}
}

func resetLsFlagsForHeader() {
	lsType = ""
	lsTag = ""
	lsStatus = ""
	lsStale = false
	lsReady = false
	lsCompact = false
	lsParent = ""
	lsJSON = false
	lsExplain = false
	lsOverdue = false
	lsIncludeDeferred = false
	lsDueBefore = ""
	lsDueAfter = ""
	lsMetadataField = nil
	lsHasMetadataKey = nil
}
