// SPDX-License-Identifier: MIT
//
// STORY-JSCHEMA-001 — envelope coverage for `mote show <id> --json` and
// the structured error envelope from main()'s *jsonenv.EnvelopedError path.
// Reuses the show-test fixtures from cmd_show_*_test.go (resetShowFlags,
// captureBothStreams) and the integration-test harness from main_test.go.
package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/jsonenv"
)

// TestShow_EnvelopeMode_WrapsObjectUnderData covers Scenario 3.
func TestShow_EnvelopeMode_WrapsObjectUnderData(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "envelope show", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	resetShowFlags()
	showJSON = true
	defer resetShowFlags()

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", seeded.ID, "--json"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})
	if rerr != nil {
		t.Fatalf("expected nil error, got %v (stderr=%q)", rerr, stderr)
	}
	if stderr != "" {
		t.Errorf("envelope mode must NOT emit notice on stderr, got %q", stderr)
	}

	var got struct {
		SchemaVersion int        `json:"schema_version"`
		Data          ShowOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope JSON must parse, got %v over %q", err, stdout)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if got.Data.ID != seeded.ID {
		t.Fatalf("data.id = %q, want %q", got.Data.ID, seeded.ID)
	}
}

// TestShow_EnvelopeMode_NotFound_ReturnsEnvelopedError covers Scenario 4.
// main() emits the structured error envelope; we observe the *EnvelopedError
// directly from rootCmd.Execute() because the test harness doesn't run
// os.Exit on this path.
func TestShow_EnvelopeMode_NotFound_ReturnsEnvelopedError(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	resetShowFlags()
	showJSON = true
	defer resetShowFlags()

	var rerr error
	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", "does-not-exist", "--json"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})

	if rerr == nil {
		t.Fatal("expected error for missing mote, got nil")
	}

	var ee *jsonenv.EnvelopedError
	if !errors.As(rerr, &ee) {
		t.Fatalf("expected *EnvelopedError, got %T: %v", rerr, rerr)
	}
	if ee.Code != "MOTE_NOT_FOUND" {
		t.Errorf("code = %q, want MOTE_NOT_FOUND", ee.Code)
	}
	if ee.ExitCode == 0 {
		t.Errorf("exit code must be non-zero, got %d", ee.ExitCode)
	}
	if !strings.Contains(ee.Message, "does-not-exist") {
		t.Errorf("message should name the missing id, got %q", ee.Message)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty when error envelope is returned, got %q", stdout)
	}
}

// TestShow_LegacyMode_NotFound_PreservesPlainTextError ensures the existing
// exit-1 plain-stderr contract is unchanged when the envelope is opt-out.
func TestShow_LegacyMode_NotFound_PreservesPlainTextError(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	resetShowFlags()
	showJSON = true
	defer resetShowFlags()

	var rerr error
	_, _ = captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", "does-not-exist", "--json"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})

	if rerr == nil {
		t.Fatal("expected error, got nil")
	}
	var ee *jsonenv.EnvelopedError
	if errors.As(rerr, &ee) {
		t.Fatalf("legacy mode must NOT return an EnvelopedError, got %v", ee)
	}
	var ec *exitCodeError
	if !errors.As(rerr, &ec) {
		t.Fatalf("legacy mode should return *exitCodeError, got %T: %v", rerr, rerr)
	}
	if !strings.Contains(rerr.Error(), "not found") {
		t.Errorf("legacy error should describe not-found, got %q", rerr.Error())
	}
}

// TestShow_EnvelopeMode_ExecutionOnly_WrapsUnderData verifies that the
// --execution-only JSON shape participates in the envelope (registered as
// show.execution-only.v1 in docs/JSON_SCHEMA.md).
func TestShow_EnvelopeMode_ExecutionOnly_WrapsUnderData(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "exec only target", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	resetShowFlags()
	showExecutionOnly = true
	defer resetShowFlags()

	var rerr error
	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", seeded.ID, "--execution-only"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})
	if rerr != nil {
		t.Fatalf("unexpected error: %v", rerr)
	}

	var got struct {
		SchemaVersion int                     `json:"schema_version"`
		Data          ShowExecutionOnlyOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope JSON must parse, got %v over %q", err, stdout)
	}
	if got.Data.ID != seeded.ID {
		t.Fatalf("data.id = %q, want %q", got.Data.ID, seeded.ID)
	}
}
