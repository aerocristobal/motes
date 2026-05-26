// SPDX-License-Identifier: MIT
//
// STORY-JSCHEMA-001 — envelope-mode and legacy-mode coverage for
// `mote ls --json`. Reuses the same harness as cmd_ls_empty_state_test.go
// (resetLsFlags, runLsViaCobra, captureBothStreams) and the existing
// integration-test setup (setupIntegrationTest).
//
// Pulse routes through doLs, so `mote pulse --json` is exercised by a sibling
// test in cmd_pulse_envelope_test.go.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/jsonenv"
)

// TestLs_EnvelopeMode_WrapsListUnderData covers Scenario 2.
func TestLs_EnvelopeMode_WrapsListUnderData(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "envelope smoke task", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0, got %v", rerr)
	}
	if stderr != "" {
		t.Errorf("envelope mode must NOT emit the legacy deprecation notice on stderr, got: %q", stderr)
	}

	var got struct {
		SchemaVersion int      `json:"schema_version"`
		Data          LsOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope JSON must parse, got error %v over %q", err, stdout)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if len(got.Data.Motes) != 1 {
		t.Fatalf("data.motes len = %d, want 1 (stdout=%q)", len(got.Data.Motes), stdout)
	}
}

// TestLs_LegacyMode_EmitsLegacyShape covers Scenario 1.
func TestLs_LegacyMode_EmitsLegacyShape(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "legacy smoke task", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0, got %v", rerr)
	}

	// Legacy stdout must be the existing `{"motes":[...]}` shape — NOT wrapped.
	if !strings.HasPrefix(strings.TrimSpace(stdout), `{`) {
		t.Fatalf("stdout must start with JSON object, got %q", stdout)
	}
	if strings.Contains(stdout, "schema_version") {
		t.Errorf("legacy stdout must NOT carry schema_version, got %q", stdout)
	}
	if !strings.Contains(stdout, `"motes"`) {
		t.Errorf("legacy stdout must contain top-level `motes` key, got %q", stdout)
	}

	// Stderr must carry the one-line deprecation notice naming the env var.
	if !strings.Contains(stderr, jsonenv.EnvVar) {
		t.Errorf("legacy stderr must name %s, got %q", jsonenv.EnvVar, stderr)
	}
}

// TestLs_LegacyMode_DeprecationNoticeFiresOncePerProcess pins the rate-limit
// described in Scenario 1's "the deprecation line written once" note.
func TestLs_LegacyMode_DeprecationNoticeFiresOncePerProcess(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	// First invocation: notice fires.
	_, stderr1 := captureBothStreams(t, func() {
		_ = runLsViaCobra([]string{"ls", "--json"})
	})
	if !strings.Contains(stderr1, jsonenv.EnvVar) {
		t.Fatalf("first call: expected notice on stderr, got %q", stderr1)
	}

	// Second invocation in the same process: notice MUST NOT fire again.
	_, stderr2 := captureBothStreams(t, func() {
		_ = runLsViaCobra([]string{"ls", "--json"})
	})
	if strings.Contains(stderr2, jsonenv.EnvVar) {
		t.Errorf("second call in same process: notice fired twice (got %q); rate-limit broken", stderr2)
	}
}

// TestLs_EnvelopeMode_EmptyListIsPreserved covers the Sprint-2 §23.16
// guarantee called out in §6 of the story.
func TestLs_EnvelopeMode_EmptyListIsPreserved(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0 on empty workspace, got %v", rerr)
	}
	if stderr != "" {
		t.Errorf("envelope mode must NOT emit notice; got stderr=%q", stderr)
	}
	var got struct {
		SchemaVersion int      `json:"schema_version"`
		Data          LsOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope JSON must parse, got %v over %q", err, stdout)
	}
	if got.Data.Motes == nil {
		t.Fatalf("data.motes must be [] not null in envelope mode; agents loop on the shape")
	}
	if len(got.Data.Motes) != 0 {
		t.Fatalf("data.motes len = %d, want 0", len(got.Data.Motes))
	}
}
