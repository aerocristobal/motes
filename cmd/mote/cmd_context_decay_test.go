// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

// STORY-DECAY-001 Scenario 8: mote context --planning mutes closed deps.
func TestPlanningContext_MutesClosedDependencies(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer func() { noColorFlag = false }()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	dep1, err := mm.Create("task", "Database schema", core.CreateOpts{})
	if err != nil {
		t.Fatalf("create dep1: %v", err)
	}
	if err := mm.Update(dep1.ID, core.UpdateOpts{Status: core.StringPtr("completed")}); err != nil {
		t.Fatalf("close dep1: %v", err)
	}
	dep2, err := mm.Create("task", "Auth middleware", core.CreateOpts{})
	if err != nil {
		t.Fatalf("create dep2: %v", err)
	}
	target, err := mm.Create("task", "Add login flow", core.CreateOpts{})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := mm.Link(target.ID, "depends_on", dep1.ID, im); err != nil {
		t.Fatalf("link dep1: %v", err)
	}
	if err := mm.Link(target.ID, "depends_on", dep2.ID, im); err != nil {
		t.Fatalf("link dep2: %v", err)
	}
	motes, _ := mm.ReadAllParallel()
	if err := im.Rebuild(motes); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "")

	contextPlanning = true
	defer func() { contextPlanning = false }()

	stdout, _ := captureBothStreams(t, func() {
		if err := contextCmd.RunE(contextCmd, []string{target.ID}); err != nil {
			t.Fatalf("context --planning: %v", err)
		}
	})

	var sawCompletedMuted, sawActive bool
	for _, line := range strings.Split(stdout, "\n") {
		switch {
		case strings.Contains(line, dep1.ID):
			sawCompletedMuted = true
			if !strings.Contains(line, "\x1b[2m") {
				t.Errorf("completed dep line should be muted; got %q", line)
			}
		case strings.Contains(line, dep2.ID):
			sawActive = true
			if strings.Contains(line, "\x1b[2m") {
				t.Errorf("active dep line should NOT be muted; got %q", line)
			}
		}
	}
	if !sawCompletedMuted {
		t.Errorf("completed dep %s not found in planning output: %q", dep1.ID, stdout)
	}
	if !sawActive {
		t.Errorf("active dep %s not found in planning output: %q", dep2.ID, stdout)
	}
}

// Non-TTY planning output must contain no ANSI escapes.
func TestPlanningContext_NonTTY_NoAnsiEscapes(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer func() { noColorFlag = false }()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	dep, err := mm.Create("task", "Database schema", core.CreateOpts{})
	if err != nil {
		t.Fatalf("create dep: %v", err)
	}
	if err := mm.Update(dep.ID, core.UpdateOpts{Status: core.StringPtr("completed")}); err != nil {
		t.Fatalf("close dep: %v", err)
	}
	target, err := mm.Create("task", "Add login flow", core.CreateOpts{})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := mm.Link(target.ID, "depends_on", dep.ID, im); err != nil {
		t.Fatalf("link: %v", err)
	}
	motes, _ := mm.ReadAllParallel()
	if err := im.Rebuild(motes); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	contextPlanning = true
	defer func() { contextPlanning = false }()

	stdout, _ := captureBothStreams(t, func() {
		if err := contextCmd.RunE(contextCmd, []string{target.ID}); err != nil {
			t.Fatalf("context --planning: %v", err)
		}
	})

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("non-TTY planning output contains ANSI escapes: %q", stdout)
	}
}
