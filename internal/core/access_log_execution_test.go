// SPDX-License-Identifier: MIT
package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readAccessBatchEntries reads .access_batch.jsonl from root and returns the
// parsed entries. Returns nil if the file does not exist.
func readAccessBatchEntries(t *testing.T, root string) []AccessBatchEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".access_batch.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read batch: %v", err)
	}
	var entries []AccessBatchEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e AccessBatchEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("parse batch line %q: %v", line, err)
		}
		entries = append(entries, e)
	}
	return entries
}

// TestAccessLog_ReadExecutionEvent asserts that AppendAccessBatchExecution
// writes a record with action="read_execution" and the configured agent ID.
// STORY-EREAD-001: this is the inspect-only trace.
func TestAccessLog_ReadExecutionEvent(t *testing.T) {
	root, mm := setupTestMemory(t)
	m, err := mm.Create("task", "Test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("MOTE_AGENT_ID", "agent-orchestrator")
	if err := mm.AppendAccessBatchExecution(m.ID); err != nil {
		t.Fatalf("AppendAccessBatchExecution: %v", err)
	}

	entries := readAccessBatchEntries(t, root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(entries), entries)
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
	if e.Primed {
		t.Error("read_execution entry should not be marked primed")
	}
}

// TestAccessLog_PlainShow_RecordsRead asserts that a plain AppendAccessBatch
// writes action="read", distinct from "read_execution". STORY-EREAD-001
// Scenario "An orchestrator that calls plain mote show leaves a normal read trace".
func TestAccessLog_PlainShow_RecordsRead(t *testing.T) {
	root, mm := setupTestMemory(t)
	m, err := mm.Create("task", "Test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if err := mm.AppendAccessBatch(m.ID); err != nil {
		t.Fatalf("AppendAccessBatch: %v", err)
	}

	entries := readAccessBatchEntries(t, root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "read" {
		t.Errorf("Action: got %q, want %q", entries[0].Action, "read")
	}
}

// TestAccessLog_ReadExecution_NotConflatedWithRead asserts that read and
// read_execution remain distinguishable on the wire. A compliance reviewer
// MUST be able to filter the access log for "did this orchestrator inspect
// metadata before dispatching subagent Y?".
func TestAccessLog_ReadExecution_NotConflatedWithRead(t *testing.T) {
	root, mm := setupTestMemory(t)
	m, err := mm.Create("task", "Test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if err := mm.AppendAccessBatch(m.ID); err != nil {
		t.Fatal(err)
	}
	if err := mm.AppendAccessBatchExecution(m.ID); err != nil {
		t.Fatal(err)
	}

	entries := readAccessBatchEntries(t, root)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	var read, readExec int
	for _, e := range entries {
		switch e.Action {
		case "read":
			read++
		case "read_execution":
			readExec++
		default:
			t.Errorf("unexpected action %q", e.Action)
		}
	}
	if read != 1 || readExec != 1 {
		t.Errorf("expected 1 read + 1 read_execution, got read=%d read_execution=%d", read, readExec)
	}
}

// TestAccessLog_MultipleReadExecution_NotDeduped asserts the append-only
// semantics: each `--execution-only` call is a discrete inspect event
// (STORY-EREAD-001 Q6). The log MUST NOT de-dup within a session.
func TestAccessLog_MultipleReadExecution_NotDeduped(t *testing.T) {
	root, mm := setupTestMemory(t)
	m, err := mm.Create("task", "Test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := mm.AppendAccessBatchExecution(m.ID); err != nil {
			t.Fatalf("AppendAccessBatchExecution #%d: %v", i, err)
		}
	}

	entries := readAccessBatchEntries(t, root)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (no de-dup), got %d", len(entries))
	}
	for i, e := range entries {
		if e.Action != "read_execution" {
			t.Errorf("entry %d: Action=%q, want read_execution", i, e.Action)
		}
	}
}

// TestAccessLog_NoReadBodyEvent asserts the contract's negative scenario:
// `--execution-only` never emits a read_body event. The CLI only knows two
// flavors of read: "read" (default show) and "read_execution"
// (--execution-only). Confirming this on the type level prevents a future
// drive-by addition of "read_body" from quietly shipping.
func TestAccessLog_NoReadBodyEvent(t *testing.T) {
	root, mm := setupTestMemory(t)
	m, err := mm.Create("task", "Test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if err := mm.AppendAccessBatchExecution(m.ID); err != nil {
		t.Fatal(err)
	}

	entries := readAccessBatchEntries(t, root)
	for _, e := range entries {
		if e.Action == "read_body" {
			t.Errorf("read_body event must not appear: %+v", e)
		}
	}
}
