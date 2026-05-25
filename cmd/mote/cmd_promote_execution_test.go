// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func runPromoteViaCobra(args []string) error {
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

// Promoting a mote with execution metadata strips it from the global copy.
// Execution hints are workflow-local; a cross-project lesson has no dispatch mode.
func TestPromote_StripsExecutionMetadata(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(memDir)
	source, err := mm.Create("lesson", "promote me", core.CreateOpts{
		Weight: 0.5, Origin: "normal",
		Body:  "this is a lesson body long enough to satisfy the promote threshold",
		Local: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mode := "parallel"
	model := "haiku"
	if err := mm.Update(source.ID, core.UpdateOpts{
		ExecutionMode:           &mode,
		ExecutionSuggestedModel: &model,
	}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(func() {
		if err := runPromoteViaCobra([]string{"promote", source.ID}); err != nil {
			t.Fatalf("promote: %v", err)
		}
	})

	// Extract the global ID from "Promoted <source> -> <global>".
	var globalID string
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, " -> "); i >= 0 {
			globalID = strings.TrimSpace(line[i+len(" -> "):])
			break
		}
	}
	if globalID == "" {
		t.Fatalf("could not parse global ID from output: %q", out)
	}

	global, err := mm.Read(globalID)
	if err != nil {
		t.Fatalf("read global: %v", err)
	}
	if global.ExecutionMode != "" || global.ExecutionSuggestedModel != "" {
		t.Errorf("execution metadata must be stripped on promote, got mode=%q model=%q",
			global.ExecutionMode, global.ExecutionSuggestedModel)
	}
}
