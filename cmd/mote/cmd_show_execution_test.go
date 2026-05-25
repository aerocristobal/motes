// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"motes/internal/core"
)

func runShowViaCobra(args []string) error {
	resetShowFlags()
	defer resetShowFlags()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

func seedExecutionMote(t *testing.T, memDir string) *core.Mote {
	t.Helper()
	mm := core.NewMoteManager(memDir)
	m, err := mm.Create("task", "with execution", core.CreateOpts{
		Weight: 0.5, Origin: "normal",
		Body: "body content here",
	})
	if err != nil {
		t.Fatal(err)
	}
	mode := "parallel"
	model := "haiku"
	effort := "low"
	agent := "mote-subagent"
	grp := "group-A"
	if err := mm.Update(m.ID, core.UpdateOpts{
		ExecutionAgentType:       &agent,
		ExecutionSuggestedModel:  &model,
		ExecutionReasoningEffort: &effort,
		ExecutionMode:            &mode,
		ExecutionParallelGroup:   &grp,
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := mm.Read(m.ID)
	return got
}

func TestShow_Text_ExecutionSection_AppearsBeforeBody(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})

	execIdx := strings.Index(out, "--- execution ---")
	bodyIdx := strings.Index(out, "--- body ---")
	if execIdx == -1 {
		t.Fatalf("execution section missing:\n%s", out)
	}
	if bodyIdx == -1 {
		t.Fatalf("body section missing:\n%s", out)
	}
	if execIdx > bodyIdx {
		t.Errorf("execution section must appear before body (exec=%d body=%d):\n%s", execIdx, bodyIdx, out)
	}
	for _, want := range []string{
		"agent_type", "suggested_model", "reasoning_effort", "mode", "parallel_group",
		"mote-subagent", "haiku", "low", "parallel", "group-A",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in text output:\n%s", want, out)
		}
	}
}

func TestShow_Text_NoExecutionMetadata_OmitsSection(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedClaimTask(t, memDir, "plain task")

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})
	if strings.Contains(out, "--- execution ---") {
		t.Errorf("execution section should not appear when no fields set:\n%s", out)
	}
}

func TestShow_JSON_ExecutionKeysAppearBeforeBody(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedExecutionMote(t, memDir)

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID, "--json"}); err != nil {
			t.Fatalf("show --json: %v", err)
		}
	})

	// Decode-and-recheck: all keys present
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, key := range []string{
		"execution_agent_type", "execution_suggested_model",
		"execution_reasoning_effort", "execution_mode", "execution_parallel_group",
	} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("expected %q in JSON keys, got %v", key, parsed)
		}
	}

	// String-position check: every execution_* key must precede the body key
	// in the raw serialized output. Go's encoding/json preserves struct field
	// declaration order, so this enforces the per-spec ordering.
	bodyPos := strings.Index(out, `"body"`)
	if bodyPos < 0 {
		t.Fatalf("body key missing:\n%s", out)
	}
	for _, k := range []string{
		`"execution_agent_type"`, `"execution_suggested_model"`,
		`"execution_reasoning_effort"`, `"execution_mode"`, `"execution_parallel_group"`,
	} {
		pos := strings.Index(out, k)
		if pos < 0 {
			t.Errorf("key %s missing in JSON:\n%s", k, out)
			continue
		}
		if pos > bodyPos {
			t.Errorf("key %s appears AFTER body (%d > %d):\n%s", k, pos, bodyPos, out)
		}
	}
}

func TestShow_JSON_NoExecutionMetadata_OmitsKeys(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedClaimTask(t, memDir, "plain task")

	out := captureStdout(func() {
		if err := runShowViaCobra([]string{"show", m.ID, "--json"}); err != nil {
			t.Fatalf("show --json: %v", err)
		}
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, key := range []string{
		"execution_agent_type", "execution_suggested_model",
		"execution_reasoning_effort", "execution_mode", "execution_parallel_group",
	} {
		if _, ok := parsed[key]; ok {
			t.Errorf("unset key %q should be absent from JSON, got %v", key, parsed[key])
		}
	}
}
