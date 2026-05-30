// SPDX-License-Identifier: MIT
// STORY-PRIMEOVR-001 — `mote prime --export` flag integration tests.
package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"motes/internal/prime"
)

// Scenario 5 — `--export` writes the baked-in default template to stdout
// with no data sections and no truncation directive. The static template
// content must be byte-for-byte identical to prime.DefaultExportTemplate().
func TestPrime_Export_WritesDefaultTemplateOnly(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	primeExport = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --export: %v", err)
		}
	})

	want := prime.DefaultExportTemplate()
	if output != want {
		t.Errorf("--export stdout drift\n got: %q\n want: %q", output, want)
	}

	// Must not include any mote-generated framing.
	if strings.Contains(output, prime.TruncationDirective) {
		t.Error("--export must not include the truncation directive (mote-generated, not part of user template)")
	}
	for _, banned := range []string{
		"## Persistent memories",
		"## Ready to start",
		"## Active work",
		"## Relevant decisions",
		"## Key lessons",
	} {
		if strings.Contains(output, banned) {
			t.Errorf("--export must not include live data section %q", banned)
		}
	}
}

// Scenario 5 — `--export` ignores project state. Seeding memories +
// motes must not change the output one byte.
func TestPrime_Export_IgnoresProjectState(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMemory(t, root, "should-not-appear", "neither should this")
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Hidden task", Tags: []string{"x"}, Weight: 0.9},
	})

	primeExport = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --export: %v", err)
		}
	})

	if strings.Contains(output, "should-not-appear") || strings.Contains(output, "Hidden task") {
		t.Errorf("--export must not surface live project data; output:\n%s", output)
	}
}

// Scenario 9 — when stdout is a TTY, a one-line redirect hint is sent
// to stderr. Triggered here via the format-package MOTE_FORCE_TTY hook
// (already documented as the internal test override for IsTTY).
func TestPrime_Export_TTYHintGoesToStderr(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv("MOTE_FORCE_TTY", "1")
	primeExport = true

	// Capture BOTH stderr and stdout; the hint must land on stderr.
	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	err := primeCmd.RunE(primeCmd, nil)
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	stdoutBytes, _ := io.ReadAll(rOut)
	stderrBytes, _ := io.ReadAll(rErr)

	if err != nil {
		t.Fatalf("prime --export: %v", err)
	}
	if !strings.Contains(string(stderrBytes), "Hint:") {
		t.Errorf("expected redirect hint on stderr when stdout is a TTY; got:\n%s", string(stderrBytes))
	}
	if !strings.Contains(string(stderrBytes), "mote prime --export") {
		t.Errorf("hint should reference the redirect form `mote prime --export`; got:\n%s", string(stderrBytes))
	}
	if strings.Contains(string(stdoutBytes), "Hint:") {
		t.Errorf("hint must not be on stdout (would corrupt redirected PRIME.md); stdout:\n%s", string(stdoutBytes))
	}
}

// Scenario 9 — when stdout is NOT a TTY (pipe), no hint is emitted so
// `mote prime --export > PRIME.md` produces a clean file.
func TestPrime_Export_NoHintWhenStdoutPiped(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// MOTE_FORCE_TTY *not* set; the test harness already pipes stdout
	// via captureStdout, so IsTTY returns false naturally.
	primeExport = true

	oldStderr := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr

	_ = captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --export: %v", err)
		}
	})
	_ = wErr.Close()
	os.Stderr = oldStderr
	stderrBytes, _ := io.ReadAll(rErr)

	if len(stderrBytes) != 0 {
		t.Errorf("expected empty stderr when stdout is piped; got:\n%s", string(stderrBytes))
	}
}
