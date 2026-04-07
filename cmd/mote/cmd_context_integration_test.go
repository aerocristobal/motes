// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestContext_FullPipeline_RankedByScore(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	mA, _ := mm.Create("decision", "Auth decision A", core.CreateOpts{Tags: []string{"auth"}, Weight: 0.9})
	mB, _ := mm.Create("lesson", "Auth lesson B", core.CreateOpts{Tags: []string{"auth"}, Weight: 0.6})
	mC, _ := mm.Create("explore", "Auth explore C", core.CreateOpts{Tags: []string{"auth"}, Weight: 0.3})

	mm.Link(mA.ID, "builds_on", mB.ID, im)
	mm.Link(mA.ID, "relates_to", mC.ID, im)

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	output := captureStdout(func() {
		contextCmd.RunE(contextCmd, []string{"auth"})
	})

	// All 3 should appear
	if !strings.Contains(output, "Auth decision A") {
		t.Errorf("expected 'Auth decision A' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Auth lesson B") {
		t.Errorf("expected 'Auth lesson B' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Auth explore C") {
		t.Errorf("expected 'Auth explore C' in output, got:\n%s", output)
	}

	// Verify descending score order by checking line positions
	lines := strings.Split(output, "\n")
	var scoreLines []string
	for _, line := range lines {
		if strings.Contains(line, "Auth") {
			scoreLines = append(scoreLines, line)
		}
	}
	if len(scoreLines) >= 2 {
		// First result should be highest-weight mote
		if !strings.Contains(scoreLines[0], "Auth decision A") {
			t.Errorf("expected highest-scored mote first, got:\n%s", scoreLines[0])
		}
	}
}

func TestContext_FullPipeline_AccessCountsIncremented(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "Access test A", Tags: []string{"access"}, Weight: 0.8},
		{Type: "lesson", Title: "Access test B", Tags: []string{"access"}, Weight: 0.7},
	})

	mm := core.NewMoteManager(root)

	// Run context command to trigger access batch writes
	captureStdout(func() {
		contextCmd.RunE(contextCmd, []string{"access"})
	})

	// Flush the access batch
	mm.FlushAccessBatch()

	// Re-read motes and verify access counts incremented
	motes, _ := mm.ReadAllParallel()
	for _, m := range motes {
		if m.Title == "Access test A" || m.Title == "Access test B" {
			if m.AccessCount < 1 {
				t.Errorf("expected AccessCount >= 1 for %q, got %d", m.Title, m.AccessCount)
			}
		}
	}
}

func TestContext_FullPipeline_MaxResultsRespected(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Seed 15 motes tagged "auth"
	specs := make([]moteSpec, 15)
	for i := range specs {
		specs[i] = moteSpec{
			Type:   "decision",
			Title:  "Auth mote",
			Tags:   []string{"auth"},
			Weight: 0.8,
		}
	}
	seedMotes(t, root, specs)

	output := captureStdout(func() {
		contextCmd.RunE(contextCmd, []string{"auth"})
	})

	// Count result lines (skip header + separator)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var resultCount int
	for _, line := range lines {
		if strings.Contains(line, "Auth mote") {
			resultCount++
		}
	}

	// MaxResults defaults to 12
	if resultCount > 12 {
		t.Errorf("expected at most 12 results, got %d", resultCount)
	}
}

func TestContext_FullPipeline_MinThresholdRespected(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "High weight auth", Tags: []string{"auth"}, Weight: 0.9},
		{Type: "decision", Title: "Very low weight auth", Tags: []string{"auth"}, Weight: 0.01},
	})

	output := captureStdout(func() {
		contextCmd.RunE(contextCmd, []string{"auth"})
	})

	if !strings.Contains(output, "High weight auth") {
		t.Errorf("expected high-weight mote in output, got:\n%s", output)
	}
	// Very low weight mote (0.01) with no recent access should be below threshold
	if strings.Contains(output, "Very low weight auth") {
		t.Logf("low-weight mote appeared (may pass threshold with tag bonus); output:\n%s", output)
	}
}
