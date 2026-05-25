// SPDX-License-Identifier: MIT
package core

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMote_ExecutionFields_YAMLRoundTrip_All(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	original := &Mote{
		ID:                       "proj-T1abc23",
		Type:                     "task",
		Status:                   "active",
		Title:                    "parallel job 1",
		Tags:                     []string{"work"},
		Weight:                   0.5,
		Origin:                   "normal",
		CreatedAt:                now,
		AccessCount:              0,
		ExecutionAgentType:       "mote-subagent",
		ExecutionSuggestedModel:  "haiku",
		ExecutionReasoningEffort: "low",
		ExecutionMode:            "parallel",
		ExecutionParallelGroup:   "group-A",
	}

	data, err := SerializeMote(original)
	if err != nil {
		t.Fatalf("SerializeMote: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseMote(path)
	if err != nil {
		t.Fatalf("ParseMote: %v", err)
	}

	if parsed.ExecutionAgentType != "mote-subagent" {
		t.Errorf("agent_type: got %q", parsed.ExecutionAgentType)
	}
	if parsed.ExecutionSuggestedModel != "haiku" {
		t.Errorf("suggested_model: got %q", parsed.ExecutionSuggestedModel)
	}
	if parsed.ExecutionReasoningEffort != "low" {
		t.Errorf("reasoning_effort: got %q", parsed.ExecutionReasoningEffort)
	}
	if parsed.ExecutionMode != "parallel" {
		t.Errorf("mode: got %q", parsed.ExecutionMode)
	}
	if parsed.ExecutionParallelGroup != "group-A" {
		t.Errorf("parallel_group: got %q", parsed.ExecutionParallelGroup)
	}

	// Confirm all five keys are emitted in the YAML frontmatter
	body := string(data)
	for _, key := range []string{
		"execution_agent_type", "execution_suggested_model",
		"execution_reasoning_effort", "execution_mode", "execution_parallel_group",
	} {
		if !strings.Contains(body, key+":") {
			t.Errorf("expected frontmatter to contain %q: \n%s", key, body)
		}
	}
}

func TestMote_ExecutionFields_OmitWhenUnset(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	m := &Mote{
		ID: "proj-T1abc23", Type: "task", Status: "active",
		Title: "ordinary", Tags: []string{}, Weight: 0.5,
		Origin: "normal", CreatedAt: now,
	}

	data, err := SerializeMote(m)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, key := range []string{
		"execution_agent_type", "execution_suggested_model",
		"execution_reasoning_effort", "execution_mode", "execution_parallel_group",
	} {
		if strings.Contains(body, key) {
			t.Errorf("unset field %q should be omitted from frontmatter: \n%s", key, body)
		}
	}
}

func TestMote_ExecutionFields_YAMLRoundTrip_Partial(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	m := &Mote{
		ID: "proj-T1abc23", Type: "task", Status: "active",
		Title: "partial", Tags: []string{}, Weight: 0.5,
		Origin:        "normal",
		CreatedAt:     now,
		ExecutionMode: "delegated",
	}

	data, err := SerializeMote(m)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "execution_mode: delegated") {
		t.Errorf("expected execution_mode in frontmatter: \n%s", body)
	}
	if strings.Contains(body, "execution_agent_type") {
		t.Error("unset execution_agent_type should be omitted")
	}
}

// --- Manager: set, clear, audit ---

func TestMoteManager_SetExecutionFields_AndAudit(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "agent-test")
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "exec test", CreateOpts{Weight: 0.5, Origin: "normal"})
	if err != nil {
		t.Fatal(err)
	}

	mode := "parallel"
	model := "haiku"
	if err := mm.Update(m.ID, UpdateOpts{
		ExecutionMode:           &mode,
		ExecutionSuggestedModel: &model,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	read, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.ExecutionMode != "parallel" || read.ExecutionSuggestedModel != "haiku" {
		t.Errorf("fields not set: mode=%q model=%q", read.ExecutionMode, read.ExecutionSuggestedModel)
	}

	// Audit log should reflect both fields
	got := lastAuditEntry(t, mm.Root())
	if got.Operation != "update" {
		t.Errorf("operation: got %q", got.Operation)
	}
	if !containsAll(got.FieldsSet, "execution_mode", "execution_suggested_model") {
		t.Errorf("audit fields_set missing entries: %v", got.FieldsSet)
	}
	if got.ChangeAfterLaunch {
		t.Error("change_after_launch should be false (mote was not claimed)")
	}
}

func TestMoteManager_ClearExecutionField_EmptyString(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "agent-test")
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "clear test", CreateOpts{Weight: 0.5, Origin: "normal"})
	if err != nil {
		t.Fatal(err)
	}
	grp := "group-A"
	if err := mm.Update(m.ID, UpdateOpts{ExecutionParallelGroup: &grp}); err != nil {
		t.Fatal(err)
	}

	empty := ""
	if err := mm.Update(m.ID, UpdateOpts{ExecutionParallelGroup: &empty}); err != nil {
		t.Fatalf("clear update: %v", err)
	}

	read, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.ExecutionParallelGroup != "" {
		t.Errorf("field should be cleared, got %q", read.ExecutionParallelGroup)
	}

	// On-disk frontmatter should not contain the key (omitempty kicks in)
	path, _ := mm.MoteFilePath(m.ID)
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "execution_parallel_group:") {
		t.Errorf("cleared key should be absent from frontmatter:\n%s", string(raw))
	}
}

func TestMoteManager_ChangeAfterLaunch_FlagsAudit(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "agent-test")
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "claim test", CreateOpts{Weight: 0.5, Origin: "normal"})
	if err != nil {
		t.Fatal(err)
	}
	// Claim atomically
	if _, err := mm.Claim(m.ID, "agent-test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// Now mutate an execution_* field — this is "change after launch"
	model := "opus"
	if err := mm.Update(m.ID, UpdateOpts{ExecutionSuggestedModel: &model}); err != nil {
		t.Fatalf("Update after claim: %v", err)
	}

	got := lastAuditEntry(t, mm.Root())
	if !got.ChangeAfterLaunch {
		t.Errorf("change_after_launch should be true when execution_* mutated on a claimed mote; entry=%+v", got)
	}
}

func TestMoteManager_ChangeAfterLaunch_NotFlaggedForOtherFields(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "agent-test")
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "claim-other test", CreateOpts{Weight: 0.5, Origin: "normal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Claim(m.ID, "agent-test"); err != nil {
		t.Fatal(err)
	}
	title := "renamed"
	if err := mm.Update(m.ID, UpdateOpts{Title: &title}); err != nil {
		t.Fatal(err)
	}
	got := lastAuditEntry(t, mm.Root())
	if got.ChangeAfterLaunch {
		t.Errorf("change_after_launch should NOT be set for non-execution field change; entry=%+v", got)
	}
}

func TestMoteManager_ValidationRejectsInvalidEnum(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "agent-test")
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "validation test", CreateOpts{Weight: 0.5, Origin: "normal"})
	if err != nil {
		t.Fatal(err)
	}
	bogus := "fire_and_forget"
	err = mm.Update(m.ID, UpdateOpts{ExecutionMode: &bogus})
	if err == nil {
		t.Fatal("expected error for invalid execution_mode")
	}
	if !strings.Contains(err.Error(), "invalid execution_mode") {
		t.Errorf("error must mention invalid execution_mode: %v", err)
	}
}

// --- helpers ---

func lastAuditEntry(t *testing.T, root string) AuditEntry {
	t.Helper()
	path := filepath.Join(root, "audit.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer func() { _ = f.Close() }()
	var last AuditEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("audit json: %v (line: %q)", err, scanner.Text())
		}
		last = e
	}
	return last
}

func containsAll(haystack []string, needles ...string) bool {
	have := map[string]bool{}
	for _, h := range haystack {
		have[h] = true
	}
	for _, n := range needles {
		if !have[n] {
			return false
		}
	}
	return true
}
