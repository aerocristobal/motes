//go:build !windows

package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentAppendAccessBatch(t *testing.T) {
	_, mm := setupTestMemory(t)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if err := mm.AppendAccessBatch("mote-test-id"); err != nil {
					t.Errorf("goroutine %d append %d: %v", n, j, err)
				}
			}
		}(i)
	}
	wg.Wait()

	// Read batch and count lines
	data, err := os.ReadFile(filepath.Join(mm.Root(), ".access_batch.jsonl"))
	if err != nil {
		t.Fatalf("read batch: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1000 {
		t.Fatalf("expected 1000 entries, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var entry AccessBatchEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d invalid JSON: %v", i, err)
		}
		if entry.AgentID == "" {
			t.Fatalf("line %d missing agent_id", i)
		}
	}
}

func TestFlushAccessBatch_TOCTOU(t *testing.T) {
	_, mm := setupTestMemory(t)

	// Create a mote to flush access for
	m, err := mm.Create("lesson", "Test Mote", CreateOpts{})
	if err != nil {
		t.Fatalf("create mote: %v", err)
	}

	// Append some entries
	for i := 0; i < 10; i++ {
		if err := mm.AppendAccessBatch(m.ID); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	// Flush and concurrently append
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		mm.FlushAccessBatch()
	}()

	go func() {
		defer wg.Done()
		// Append during flush — should go to new file
		for i := 0; i < 5; i++ {
			mm.AppendAccessBatch(m.ID)
		}
	}()

	wg.Wait()

	// The post-flush appends should survive in the batch file
	batchPath := filepath.Join(mm.Root(), ".access_batch.jsonl")
	data, _ := os.ReadFile(batchPath)
	if len(data) > 0 {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		// Some entries may have landed in the new file
		t.Logf("surviving entries after flush+concurrent append: %d", len(lines))
	}

	// Verify the mote was updated by the flush
	updated, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if updated.AccessCount < 10 {
		t.Fatalf("expected access_count >= 10, got %d", updated.AccessCount)
	}
}

func TestLinkLocked_Serialization(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)

	// Create motes to link
	source, _ := mm.Create("lesson", "Source", CreateOpts{})
	var targets []*Mote
	for i := 0; i < 5; i++ {
		m, _ := mm.Create("lesson", "Target", CreateOpts{})
		targets = append(targets, m)
	}

	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(targetID string) {
			defer wg.Done()
			if err := mm.LinkLocked(source.ID, "relates_to", targetID, im); err != nil {
				t.Errorf("LinkLocked: %v", err)
			}
		}(target.ID)
	}
	wg.Wait()

	// Verify all links are present
	updated, err := mm.Read(source.ID)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if len(updated.RelatesTo) != 5 {
		t.Fatalf("expected 5 relates_to links, got %d: %v", len(updated.RelatesTo), updated.RelatesTo)
	}
}

func TestAgentIdentity_CreateUpdate(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "test-agent-1")
	_, mm := setupTestMemory(t)

	m, err := mm.Create("lesson", "Test", CreateOpts{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.CreatedBy != "test-agent-1" {
		t.Fatalf("expected created_by 'test-agent-1', got %q", m.CreatedBy)
	}
	if m.ModifiedBy != "test-agent-1" {
		t.Fatalf("expected modified_by 'test-agent-1', got %q", m.ModifiedBy)
	}

	// Update with different agent
	t.Setenv("MOTE_AGENT_ID", "test-agent-2")
	if err := mm.Update(m.ID, UpdateOpts{Title: StringPtr("Updated")}); err != nil {
		t.Fatalf("update: %v", err)
	}

	updated, _ := mm.Read(m.ID)
	if updated.CreatedBy != "test-agent-1" {
		t.Fatalf("created_by should not change, got %q", updated.CreatedBy)
	}
	if updated.ModifiedBy != "test-agent-2" {
		t.Fatalf("expected modified_by 'test-agent-2', got %q", updated.ModifiedBy)
	}
}

func TestDreamLockExclusion(t *testing.T) {
	_, mm := setupTestMemory(t)

	// Acquire dream lock
	lock, acquired, err := mm.TryLockDream()
	if err != nil {
		t.Fatalf("TryLockDream: %v", err)
	}
	if !acquired {
		t.Fatal("first TryLockDream should succeed")
	}

	// Second attempt should fail
	_, acquired2, err := mm.TryLockDream()
	if err != nil {
		t.Fatalf("second TryLockDream: %v", err)
	}
	if acquired2 {
		t.Fatal("second TryLockDream should fail while first is held")
	}

	lock.Unlock()

	// After unlock, should succeed again
	lock3, acquired3, err := mm.TryLockDream()
	if err != nil {
		t.Fatalf("third TryLockDream: %v", err)
	}
	if !acquired3 {
		t.Fatal("TryLockDream should succeed after unlock")
	}
	lock3.Unlock()
}
