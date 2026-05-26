// SPDX-License-Identifier: MIT
//
// STORY-JSCHEMA-001 — envelope coverage for `mote pulse --json`. Pulse
// delegates to doLs, so the test confirms the alias inherits the envelope
// without a separate code path.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/jsonenv"
)

func TestPulse_EnvelopeMode_WrapsListUnderData(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "pulse envelope target", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	pulseJSON = true
	defer func() { pulseJSON = false }()

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"pulse", "--json"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})
	if rerr != nil {
		t.Fatalf("unexpected error: %v (stderr=%q)", rerr, stderr)
	}
	if stderr != "" {
		t.Errorf("envelope mode must NOT emit notice on stderr, got %q", stderr)
	}

	var got struct {
		SchemaVersion int      `json:"schema_version"`
		Data          LsOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope JSON must parse, got %v over %q", err, stdout)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if len(got.Data.Motes) != 1 {
		t.Fatalf("data.motes len = %d, want 1", len(got.Data.Motes))
	}
}

func TestPulse_LegacyMode_PreservesShape(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "pulse legacy target", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	pulseJSON = true
	defer func() { pulseJSON = false }()

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"pulse", "--json"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})
	if rerr != nil {
		t.Fatalf("unexpected error: %v", rerr)
	}
	if strings.Contains(stdout, "schema_version") {
		t.Errorf("legacy stdout must NOT carry schema_version, got %q", stdout)
	}
	if !strings.Contains(stderr, jsonenv.EnvVar) {
		t.Errorf("legacy stderr must name %s, got %q", jsonenv.EnvVar, stderr)
	}
}
