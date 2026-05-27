// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 Scenario 2 — `--pretty` forces ANSI on a non-TTY context.
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestLsPretty_ForcesANSIOnNonTTY(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "pretty target", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := mm.Update(seeded.ID, core.UpdateOpts{Status: core.StringPtr("deprecated")}); err != nil {
		t.Fatalf("deprecate: %v", err)
	}

	resetModeFlags(t)
	prettyFlag = true
	t.Setenv("MOTE_FORCE_TTY", "")
	t.Setenv("NO_COLOR", "")

	stdout, _ := captureBothStreams(t, func() {
		if err := runLsViaCobra([]string{"ls", "--pretty"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(stdout, "\x1b[") {
		t.Fatalf("--pretty on non-TTY must emit ANSI escapes for the deprecated row; got %q", stdout)
	}
}
