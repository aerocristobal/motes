package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

// setupMigrateTest creates .memory/ and a separate global dir, seeds local knowledge motes,
// and chdir's into the project dir. Tests using this MUST NOT call t.Parallel().
func setupMigrateTest(t *testing.T) (memDir, globalDir string, cleanup func()) {
	t.Helper()
	tmpDir := t.TempDir()
	memDir = filepath.Join(tmpDir, ".memory")
	os.MkdirAll(filepath.Join(memDir, "nodes"), 0755)
	globalDir = filepath.Join(tmpDir, "global-store")
	os.Setenv("MOTE_GLOBAL_ROOT", globalDir)

	// Initialize config + index
	cfg := core.DefaultConfig()
	core.SaveConfig(memDir, cfg)
	im := core.NewIndexManager(memDir)
	im.Rebuild(nil)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	return memDir, globalDir, func() {
		os.Chdir(origDir)
		os.Unsetenv("MOTE_GLOBAL_ROOT")
	}
}

// seedLocalKnowledge creates knowledge motes forced to local storage for migration testing.
func seedLocalKnowledge(t *testing.T, root string) []string {
	t.Helper()
	mm := core.NewMoteManager(root)
	var ids []string
	specs := []struct {
		typ, title string
	}{
		{"decision", "Local decision"},
		{"lesson", "Local lesson"},
		{"explore", "Local explore"},
	}
	for _, s := range specs {
		m, err := mm.Create(s.typ, s.title, core.CreateOpts{
			Tags:  []string{"migrate-test"},
			Local: true,
		})
		if err != nil {
			t.Fatalf("seed %s: %v", s.typ, err)
		}
		ids = append(ids, m.ID)
	}
	// Also seed a task (should NOT be migrated)
	task, err := mm.Create("task", "Local task", core.CreateOpts{Tags: []string{"migrate-test"}})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	_ = task

	// Rebuild index
	motes, _ := mm.ReadAllParallel()
	im := core.NewIndexManager(root)
	im.Rebuild(motes)

	return ids
}

func TestMigrateGlobal_MovesKnowledgeMotes(t *testing.T) {
	memDir, globalDir, cleanup := setupMigrateTest(t)
	defer cleanup()

	knowledgeIDs := seedLocalKnowledge(t, memDir)

	output := captureStdout(func() {
		err := runMigrateGlobal(migrateGlobalCmd, nil)
		if err != nil {
			t.Fatalf("migrate-global: %v", err)
		}
	})

	// Should report migrating 3 knowledge motes
	if !strings.Contains(output, "Migrated 3 knowledge motes") {
		t.Errorf("expected 'Migrated 3 knowledge motes', got: %s", output)
	}

	// Global dir should have new files
	globalNodes := filepath.Join(globalDir, "nodes")
	entries, err := os.ReadDir(globalNodes)
	if err != nil {
		t.Fatalf("read global dir: %v", err)
	}
	globalCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			globalCount++
		}
	}
	if globalCount != 3 {
		t.Errorf("expected 3 global motes, found %d", globalCount)
	}

	// Local tombstones should exist with ForwardedTo set
	mm := core.NewMoteManager(memDir)
	for _, oldID := range knowledgeIDs {
		// Read should follow tombstone to global mote
		m, err := mm.Read(oldID)
		if err != nil {
			t.Errorf("Read(%s) failed: %v", oldID, err)
			continue
		}
		if !strings.HasPrefix(m.ID, "global-") {
			t.Errorf("expected forwarded mote to have global- prefix, got %s", m.ID)
		}
	}

	// Task should still be local and unaffected
	localEntries, _ := os.ReadDir(filepath.Join(memDir, "nodes"))
	taskCount := 0
	for _, e := range localEntries {
		if strings.Contains(e.Name(), "-T") && !strings.Contains(e.Name(), "global-") {
			taskCount++
		}
	}
	if taskCount != 1 {
		t.Errorf("expected 1 local task, found %d", taskCount)
	}
}

func TestMigrateGlobal_Idempotent(t *testing.T) {
	memDir, _, cleanup := setupMigrateTest(t)
	defer cleanup()

	seedLocalKnowledge(t, memDir)

	// First run
	captureStdout(func() {
		if err := runMigrateGlobal(migrateGlobalCmd, nil); err != nil {
			t.Fatalf("first migrate: %v", err)
		}
	})

	// Second run should find nothing to migrate
	output := captureStdout(func() {
		if err := runMigrateGlobal(migrateGlobalCmd, nil); err != nil {
			t.Fatalf("second migrate: %v", err)
		}
	})

	if !strings.Contains(output, "No knowledge motes to migrate") {
		t.Errorf("expected idempotent 'No knowledge motes to migrate', got: %s", output)
	}
}

func TestMigrateGlobal_RewritesEdges(t *testing.T) {
	memDir, globalDir, cleanup := setupMigrateTest(t)
	defer cleanup()

	mm := core.NewMoteManager(memDir)

	// Create two linked knowledge motes (forced local)
	decision, err := mm.Create("decision", "Linked decision", core.CreateOpts{
		Tags:  []string{"edge-test"},
		Local: true,
	})
	if err != nil {
		t.Fatalf("create decision: %v", err)
	}
	lesson, err := mm.Create("lesson", "Linked lesson", core.CreateOpts{
		Tags:  []string{"edge-test"},
		Local: true,
		Body:  "See also [[" + decision.ID + "]] for context.",
	})
	if err != nil {
		t.Fatalf("create lesson: %v", err)
	}
	// Add edge: lesson relates_to decision
	im := core.NewIndexManager(memDir)
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	mm.Link(lesson.ID, "relates_to", decision.ID, im)

	// Migrate
	captureStdout(func() {
		if err := runMigrateGlobal(migrateGlobalCmd, nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	})

	// Read the migrated global motes and verify edges are rewritten
	globalNodes := filepath.Join(globalDir, "nodes")
	entries, err := os.ReadDir(globalNodes)
	if err != nil {
		t.Fatalf("read global dir: %v", err)
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		m, err := core.ParseMote(filepath.Join(globalNodes, e.Name()))
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		// All edges should point to global- IDs, not old local IDs
		for _, rel := range m.RelatesTo {
			if !strings.HasPrefix(rel, "global-") {
				t.Errorf("mote %s: relates_to still references local ID %s", m.ID, rel)
			}
		}
		// Body wikilinks should also be rewritten
		if strings.Contains(m.Body, "[["+decision.ID+"]]") {
			t.Errorf("mote %s: body still contains old wikilink [[%s]]", m.ID, decision.ID)
		}
	}
}

func TestMigrateGlobal_DryRun(t *testing.T) {
	memDir, globalDir, cleanup := setupMigrateTest(t)
	defer cleanup()

	seedLocalKnowledge(t, memDir)

	// Set dry-run flag
	oldDryRun := migrateGlobalDryRun
	migrateGlobalDryRun = true
	defer func() { migrateGlobalDryRun = oldDryRun }()

	output := captureStdout(func() {
		if err := runMigrateGlobal(migrateGlobalCmd, nil); err != nil {
			t.Fatalf("dry-run: %v", err)
		}
	})

	if !strings.Contains(output, "Would migrate") {
		t.Errorf("expected dry-run output, got: %s", output)
	}

	// Global dir should have no mote files
	globalNodes := filepath.Join(globalDir, "nodes")
	entries, err := os.ReadDir(globalNodes)
	if err != nil {
		// Dir might not even exist — that's fine for dry-run
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			t.Errorf("dry-run created file in global dir: %s", e.Name())
		}
	}
}
