// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func readAuditEntries(t *testing.T, root string) []AuditEntry {
	t.Helper()
	path := filepath.Join(root, "audit.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit.jsonl: %v", err)
	}
	defer f.Close()

	var entries []AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal audit entry: %v", err)
		}
		entries = append(entries, e)
	}
	return entries
}

func TestAuditLogCreate(t *testing.T) {
	root := t.TempDir()
	al := NewAuditLogger(root)

	err := al.Log(AuditEntry{
		Operation: "create",
		MoteID:    "test-T123",
		AgentID:   "test-agent",
		Timestamp: "2025-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries := readAuditEntries(t, root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Operation != "create" {
		t.Errorf("expected operation=create, got %s", entries[0].Operation)
	}
	if entries[0].MoteID != "test-T123" {
		t.Errorf("expected mote_id=test-T123, got %s", entries[0].MoteID)
	}
}

func TestAuditLogUpdate(t *testing.T) {
	root := t.TempDir()
	al := NewAuditLogger(root)

	err := al.Log(AuditEntry{
		Operation: "update",
		MoteID:    "test-T123",
		AgentID:   "test-agent",
		Timestamp: "2025-01-01T00:00:00Z",
		FieldsSet: []string{"status", "title"},
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries := readAuditEntries(t, root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].FieldsSet) != 2 {
		t.Fatalf("expected 2 fields_set, got %d", len(entries[0].FieldsSet))
	}
	if entries[0].FieldsSet[0] != "status" || entries[0].FieldsSet[1] != "title" {
		t.Errorf("unexpected fields_set: %v", entries[0].FieldsSet)
	}
}

func TestAuditLogDelete(t *testing.T) {
	root := t.TempDir()
	al := NewAuditLogger(root)

	err := al.Log(AuditEntry{
		Operation: "delete",
		MoteID:    "test-T123",
		AgentID:   "test-agent",
		Timestamp: "2025-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries := readAuditEntries(t, root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Operation != "delete" {
		t.Errorf("expected operation=delete, got %s", entries[0].Operation)
	}
}

func TestAuditLogLink(t *testing.T) {
	root := t.TempDir()
	al := NewAuditLogger(root)

	err := al.Log(AuditEntry{
		Operation: "link",
		MoteID:    "src-123>tgt-456",
		AgentID:   "test-agent",
		Timestamp: "2025-01-01T00:00:00Z",
		FieldsSet: []string{"relates_to"},
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries := readAuditEntries(t, root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].MoteID != "src-123>tgt-456" {
		t.Errorf("expected mote_id=src-123>tgt-456, got %s", entries[0].MoteID)
	}
}

func TestAuditLogNoCorruption(t *testing.T) {
	root := t.TempDir()
	al := NewAuditLogger(root)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			al.Log(AuditEntry{
				Operation: "create",
				MoteID:    "test-concurrent",
				AgentID:   "test-agent",
				Timestamp: "2025-01-01T00:00:00Z",
			})
		}(i)
	}
	wg.Wait()

	entries := readAuditEntries(t, root)
	if len(entries) != n {
		t.Fatalf("expected %d entries, got %d", n, len(entries))
	}
}
