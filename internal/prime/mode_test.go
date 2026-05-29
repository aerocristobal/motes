// SPDX-License-Identifier: MIT
// STORY-ADAPRIME-001 — ResolveMode truth table.
package prime_test

import (
	"testing"

	"motes/internal/prime"
)

func TestResolveMode_TruthTable(t *testing.T) {
	// The precedence rule from Q1: explicit flag > auto-detection > default.
	// --mcp and --full are mutually exclusive (rejected upstream by the cobra
	// layer); the row where both are true falls back to --full here so the
	// function is total. Callers MUST gate that conflict before reaching this.
	tests := []struct {
		name       string
		mcpFlag    bool
		fullFlag   bool
		detected   bool
		wantMode   prime.Mode
		wantSource prime.ModeSource
	}{
		{"default — nothing set, nothing detected", false, false, false, prime.ModeCLI, prime.SourceAuto},
		{"auto MCP — detected, no flags", false, false, true, prime.ModeMCP, prime.SourceAuto},
		{"--mcp forces MCP without detection", true, false, false, prime.ModeMCP, prime.SourceFlag},
		{"--mcp confirms detection", true, false, true, prime.ModeMCP, prime.SourceFlag},
		{"--full forces CLI without detection", false, true, false, prime.ModeCLI, prime.SourceFlag},
		{"--full overrides detection", false, true, true, prime.ModeCLI, prime.SourceFlag},
		// Mutex case — caller should reject before reaching here, but be total.
		{"both flags (caller failed to gate)", true, true, false, prime.ModeCLI, prime.SourceFlag},
		{"both flags + detected (caller failed to gate)", true, true, true, prime.ModeCLI, prime.SourceFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotSource := prime.ResolveMode(tt.mcpFlag, tt.fullFlag, tt.detected)
			if gotMode != tt.wantMode {
				t.Errorf("mode mismatch\n got:  %q\n want: %q", gotMode, tt.wantMode)
			}
			if gotSource != tt.wantSource {
				t.Errorf("source mismatch\n got:  %q\n want: %q", gotSource, tt.wantSource)
			}
		})
	}
}
