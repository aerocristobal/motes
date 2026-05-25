// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"os/exec"
	"testing"
)

// STORY-BR-23-4 Scenario 7 — Boundary: every OTHER mote command keeps its
// normal error model. The silent-failure trap is intentionally scoped to
// `mote prime` only; ls/add/update/show/search must still surface errors
// (non-empty stderr, non-zero exit) in a non-mote dir.
//
// This regression test uses a subprocess because the other commands call
// mustFindRoot, which terminates the process via os.Exit on failure.

func TestNonPrimeCommands_StillErrorInNonMoteDir(t *testing.T) {
	if testBinaryPath == "" {
		t.Skip("test binary not built; TestMain setup did not complete")
	}

	tests := [][]string{
		{"ls"},
		{"show", "some-id"},
		{"search", "foo"},
	}
	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(testBinaryPath, args...)
			cmd.Dir = dir
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			cmd.Stdout = &bytes.Buffer{}
			err := cmd.Run()

			exit := 0
			if ee, ok := err.(*exec.ExitError); ok {
				exit = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("unexpected exec error: %v", err)
			}
			if exit == 0 {
				t.Errorf("expected non-zero exit for `mote %v` in non-mote dir", args)
			}
			if stderr.Len() == 0 {
				t.Errorf("expected non-empty stderr for `mote %v` in non-mote dir", args)
			}
		})
	}
}
