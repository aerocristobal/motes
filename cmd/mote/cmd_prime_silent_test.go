// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// STORY-BR-23-4 — Silent failure model for `mote prime`.
//
// Prime must exit 0 with no stderr output when it cannot produce a meaningful
// prime (no .memory/, unreadable .memory/, corrupt index, panic). The silent
// path emits the mode-appropriate empty payload: nothing on default text,
// literal "{}" on --hook, an empty PrimeOutput envelope on --json. The
// escape hatches --debug and MOTE_DEBUG=1 surface the underlying error.

// runPrimeIn runs `mote prime <args>` in dir as an in-process call, returning
// stdout and the error returned by RunE. It saves and restores cwd plus the
// prime flag globals (primeJSON, primeHook, primeMode, primeDebug) so tests
// can mutate flags freely without bleed-over.
func runPrimeIn(t *testing.T, dir string, args ...string) (stdout string, err error) {
	t.Helper()

	origDir, _ := os.Getwd()
	origJSON, origHook, origMode, origDebug := primeJSON, primeHook, primeMode, primeDebug
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		primeJSON, primeHook, primeMode, primeDebug = origJSON, origHook, origMode, origDebug
	})

	primeJSON, primeHook, primeDebug = false, false, false
	primeMode = "startup"
	for _, a := range args {
		switch {
		case a == "--json":
			primeJSON = true
		case a == "--hook":
			primeHook = true
		case a == "--debug":
			primeDebug = true
		case strings.HasPrefix(a, "--mode="):
			primeMode = strings.TrimPrefix(a, "--mode=")
		}
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}

	stdout = captureStdout(func() { err = runPrime(primeCmd, nil) })
	return stdout, err
}

// --- Scenario 2: no .memory/ directory → silent exit 0 ---

func TestPrime_Silent_NoMemoryDir(t *testing.T) {
	dir := t.TempDir()
	stdout, err := runPrimeIn(t, dir)
	if err != nil {
		t.Errorf("expected nil error in non-mote dir, got %v", err)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout in non-mote dir (text mode), got %q", stdout)
	}
}

// --- Scenario 3: corrupt config.yaml → silent exit 0 ---
//
// (The story originally listed "corrupt index.jsonl" as the corruption
// vector, but prime rebuilds the edge index from motes in memory — it
// never reads index.jsonl on the priming path — so a corrupt index is
// not actually a fatal condition. config.yaml IS read fatally via
// core.LoadConfig; malformed YAML there is the real "corrupt project"
// scenario the story describes.)

func TestPrime_Silent_CorruptConfig(t *testing.T) {
	dir := t.TempDir()
	memdir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(filepath.Join(memdir, "nodes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memdir, "config.yaml"), []byte("not: [valid: yaml: at: all"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, err := runPrimeIn(t, dir)
	if err != nil {
		t.Errorf("expected nil error with corrupt config, got %v", err)
	}
	if strings.Contains(stdout, wantDirectivePrefix) {
		t.Errorf("silent path must not emit truncation directive; got %q", stdout)
	}
}

// --- Scenario 6: permission denied on .memory/ → silent exit 0 ---

func TestPrime_Silent_PermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod 0o000 ineffective as root")
	}
	dir := t.TempDir()
	memdir := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(memdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(memdir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(memdir, 0o755) })

	_, err := runPrimeIn(t, dir)
	if err != nil {
		t.Errorf("expected nil error with unreadable .memory/, got %v", err)
	}
}

// --- Scenario 4: --json silent path returns valid empty PrimeOutput envelope ---

func TestPrime_Silent_JSON_EmptyEnvelope(t *testing.T) {
	dir := t.TempDir()
	stdout, err := runPrimeIn(t, dir, "--json")
	if err != nil {
		t.Errorf("expected nil error in non-mote dir with --json, got %v", err)
	}

	var got PrimeOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("--json silent stdout did not parse as PrimeOutput: %v\nraw=%q", err, stdout)
	}
	if got.TruncationNotice != "" {
		t.Errorf("silent --json must have empty truncation_notice, got %q", got.TruncationNotice)
	}
	if len(got.ActiveTasks) != 0 || len(got.Decisions) != 0 || len(got.Lessons) != 0 || len(got.Explores) != 0 {
		t.Errorf("silent --json must have empty slice fields, got %+v", got)
	}
}

// --- Scenario 5: --hook silent path returns literal "{}" ---

func TestPrime_Silent_Hook_LiteralBraces(t *testing.T) {
	dir := t.TempDir()
	stdout, err := runPrimeIn(t, dir, "--hook")
	if err != nil {
		t.Errorf("expected nil error in non-mote dir with --hook, got %v", err)
	}
	if stdout != "{}\n" {
		t.Errorf("silent --hook must emit %q, got %q", "{}\n", stdout)
	}
}

// --- Cross-cut: silent path never leaks the truncation directive ---

func TestPrime_Silent_NoDirectiveLeaks(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"text mode", nil},
		{"hook mode", []string{"--hook"}},
		{"json mode", []string{"--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			stdout, _ := runPrimeIn(t, dir, tc.args...)
			if strings.Contains(stdout, wantDirectivePrefix) {
				t.Errorf("silent path leaked %q in stdout: %q", wantDirectivePrefix, stdout)
			}
		})
	}
}

// --- Scenario 8a: --debug flag surfaces error ---

func TestPrime_Debug_FlagSurfacesError(t *testing.T) {
	dir := t.TempDir()
	_, err := runPrimeIn(t, dir, "--debug")
	if err == nil {
		t.Error("expected non-nil error with --debug in non-mote dir, got nil")
	}
}

// --- Scenario 8b: MOTE_DEBUG=1 env var surfaces error ---

func TestPrime_Debug_EnvVarSurfacesError(t *testing.T) {
	t.Setenv("MOTE_DEBUG", "1")
	dir := t.TempDir()
	_, err := runPrimeIn(t, dir)
	if err == nil {
		t.Error("expected non-nil error with MOTE_DEBUG=1 in non-mote dir, got nil")
	}
}
