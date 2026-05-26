// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"motes/internal/core"
)

// TestShow_ExecutionOnly_ReturnsJustExecutionBlock asserts the happy path:
// `mote show <id> --execution-only` returns exactly {id, execution_*} keys
// for a mote with all five execution fields set. STORY-EREAD-001 Scenario 1.
func TestShow_ExecutionOnly_ReturnsJustExecutionBlock(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID, "--execution-only"}); err != nil {
			t.Fatalf("show --execution-only: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	wantKeys := map[string]bool{
		"id":                         true,
		"execution_agent_type":       true,
		"execution_suggested_model":  true,
		"execution_reasoning_effort": true,
		"execution_mode":             true,
		"execution_parallel_group":   true,
	}
	for k := range wantKeys {
		if _, ok := parsed[k]; !ok {
			t.Errorf("expected key %q in execution-only output, got %v", k, parsed)
		}
	}
	for k := range parsed {
		if !wantKeys[k] {
			t.Errorf("unexpected key %q in execution-only output (leakage from default show)", k)
		}
	}
	if got := parsed["id"]; got != m.ID {
		t.Errorf("id: got %v, want %s", got, m.ID)
	}
}

// TestShow_ExecutionOnly_OmitsBodyAndOtherFields asserts the strict bound:
// no body, no title, no tags, no weight — only id + execution_*.
func TestShow_ExecutionOnly_OmitsBodyAndOtherFields(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID, "--execution-only"}); err != nil {
			t.Fatalf("show --execution-only: %v", err)
		}
	})

	// Raw string check is the right tool here: omitempty would hide nil/empty
	// fields anyway, so a struct-level absent check would tautologically pass.
	for _, banned := range []string{
		`"body"`,
		`"title"`,
		`"tags"`,
		`"weight"`,
		`"origin"`,
		`"created_at"`,
		`"access_count"`,
	} {
		if strings.Contains(out, banned) {
			t.Errorf("execution-only output leaked %s:\n%s", banned, out)
		}
	}
}

// TestShow_ExecutionOnly_NoMetadata_ReturnsIDOnly composes with the empty-state
// contract (sprint-2 §23.16): no metadata is not an error. Output must be
// exactly `{"id":"<id>"}` (after JSON formatting whitespace).
// STORY-EREAD-001 Scenario 2.
func TestShow_ExecutionOnly_NoMetadata_ReturnsIDOnly(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(memDir)
	m, err := mm.Create("task", "plain task", core.CreateOpts{Weight: 0.5, Origin: "normal"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID, "--execution-only"}); err != nil {
			t.Fatalf("show --execution-only: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(parsed) != 1 {
		t.Errorf("expected exactly 1 key (id) for mote without execution metadata, got %d: %v", len(parsed), parsed)
	}
	if got := parsed["id"]; got != m.ID {
		t.Errorf("id: got %v, want %s", got, m.ID)
	}
}

// TestShow_ExecutionOnly_MutuallyExclusiveWithJSON asserts the rejection path.
// STORY-EREAD-001 Scenario 3.
func TestShow_ExecutionOnly_MutuallyExclusiveWithJSON(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	resetShowFlags()
	showExecutionOnly = true
	showJSON = true
	defer resetShowFlags()

	var runErr error
	stdout, _ := captureBoth(func() {
		runErr = showCmd.RunE(showCmd, []string{m.ID})
	})
	if runErr == nil {
		t.Fatal("--execution-only --json should fail")
	}
	var ec *exitCodeError
	if !errors.As(runErr, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", runErr, runErr)
	}
	if ec.code != 1 {
		t.Errorf("expected exit code 1, got %d", ec.code)
	}
	if !strings.Contains(ec.Error(), "--execution-only is mutually exclusive with --json") {
		t.Errorf("error message should match the expected text, got: %q", ec.Error())
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on mutex error, got: %q", stdout)
	}
}

// TestShow_ExecutionOnly_NonexistentMote_Error asserts that the not-found
// error path uses the existing "mote not found" pattern, matching plain
// `mote show`. STORY-EREAD-001 Scenario 4.
func TestShow_ExecutionOnly_NonexistentMote_Error(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	resetShowFlags()
	showExecutionOnly = true
	defer resetShowFlags()

	var runErr error
	stdout, _ := captureBoth(func() {
		runErr = showCmd.RunE(showCmd, []string{"motes-doesnotexist"})
	})
	if runErr == nil {
		t.Fatal("expected not-found error")
	}
	var ec *exitCodeError
	if !errors.As(runErr, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", runErr, runErr)
	}
	if ec.code != 1 {
		t.Errorf("expected exit code 1, got %d", ec.code)
	}
	if !strings.Contains(ec.Error(), "mote not found: motes-doesnotexist") {
		t.Errorf("error should match existing not-found pattern, got: %q", ec.Error())
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on not-found, got: %q", stdout)
	}
}

// TestShow_ExecutionOnly_AccessLog_InspectOnlyTrace exercises STORY-EREAD-001
// Scenario 11 through the full cobra path: running `mote show <id>
// --execution-only` with MOTE_AGENT_ID=agent-orchestrator must append exactly
// one access-batch entry with action="read_execution" and the configured
// agent ID, and must NOT emit a "read_body" event.
func TestShow_ExecutionOnly_AccessLog_InspectOnlyTrace(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	t.Setenv("MOTE_AGENT_ID", "agent-orchestrator")

	_ = captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID, "--execution-only"}); err != nil {
			t.Fatalf("show --execution-only: %v", err)
		}
	})

	entries := readAccessBatchEntriesFromDir(t, memDir)
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 access-batch entry, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.MoteID != m.ID {
		t.Errorf("MoteID: got %q, want %q", e.MoteID, m.ID)
	}
	if e.Action != "read_execution" {
		t.Errorf("Action: got %q, want %q", e.Action, "read_execution")
	}
	if e.AgentID != "agent-orchestrator" {
		t.Errorf("AgentID: got %q, want %q", e.AgentID, "agent-orchestrator")
	}
	for _, e := range entries {
		if e.Action == "read_body" {
			t.Errorf("read_body event must not be emitted by --execution-only: %+v", e)
		}
	}
}

// TestShow_PlainShow_AccessLog_NormalReadTrace exercises Scenario 12 through
// the full cobra path: plain `mote show <id>` must record action="read".
func TestShow_PlainShow_AccessLog_NormalReadTrace(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	_ = captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})

	entries := readAccessBatchEntriesFromDir(t, memDir)
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 access-batch entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].Action != "read" {
		t.Errorf("Action: got %q, want %q", entries[0].Action, "read")
	}
}

// TestShow_ExecutionOnly_KeyOrder asserts the serialized key order: id first,
// then the five execution_* keys in struct-declared order. Go's encoding/json
// preserves struct field order, so this enforces the per-spec shape that an
// orchestrator's streaming parser can rely on.
func TestShow_ExecutionOnly_KeyOrder(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID, "--execution-only"}); err != nil {
			t.Fatalf("show --execution-only: %v", err)
		}
	})

	expectedOrder := []string{
		`"id"`,
		`"execution_agent_type"`,
		`"execution_suggested_model"`,
		`"execution_reasoning_effort"`,
		`"execution_mode"`,
		`"execution_parallel_group"`,
	}
	prev := -1
	for _, key := range expectedOrder {
		pos := strings.Index(out, key)
		if pos < 0 {
			t.Errorf("key %s missing in output:\n%s", key, out)
			continue
		}
		if pos <= prev {
			t.Errorf("key %s appears out of order (pos=%d, prev=%d):\n%s", key, pos, prev, out)
		}
		prev = pos
	}
}
