package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupGlobalTestMemory creates SEPARATE local and global directories,
// unlike setupTestMemory which points both at the same dir.
// This exercises real global routing logic.
func setupGlobalTestMemory(t *testing.T) (localRoot, globalRoot string, mm *MoteManager) {
	t.Helper()
	dir := t.TempDir()
	localRoot = filepath.Join(dir, "myproject", ".memory")
	if err := os.MkdirAll(filepath.Join(localRoot, "nodes"), 0755); err != nil {
		t.Fatal(err)
	}
	globalRoot = filepath.Join(dir, "global-store")
	if err := os.MkdirAll(filepath.Join(globalRoot, "nodes"), 0755); err != nil {
		t.Fatal(err)
	}
	mm = NewMoteManager(localRoot)
	mm.SetGlobalRoot(globalRoot)
	return
}

func TestGlobalRouting_KnowledgeTypesWriteToGlobal(t *testing.T) {
	localRoot, globalRoot, mm := setupGlobalTestMemory(t)

	for _, typ := range []string{"decision", "lesson", "explore", "context", "question"} {
		m, err := mm.Create(typ, "Test "+typ, CreateOpts{Tags: []string{"test"}})
		if err != nil {
			t.Fatalf("create %s: %v", typ, err)
		}

		// ID should have global- prefix
		if !strings.HasPrefix(m.ID, "global-") {
			t.Errorf("%s: expected global- prefix, got %s", typ, m.ID)
		}

		// File should exist in global dir, NOT local dir
		globalPath := filepath.Join(globalRoot, "nodes", m.ID+".md")
		if _, err := os.Stat(globalPath); os.IsNotExist(err) {
			t.Errorf("%s: file not found in global dir: %s", typ, globalPath)
		}
		localPath := filepath.Join(localRoot, "nodes", m.ID+".md")
		if _, err := os.Stat(localPath); err == nil {
			t.Errorf("%s: file unexpectedly found in local dir: %s", typ, localPath)
		}
	}
}

func TestGlobalRouting_TaskStaysLocal(t *testing.T) {
	localRoot, globalRoot, mm := setupGlobalTestMemory(t)

	for _, typ := range []string{"task", "constellation", "anchor"} {
		m, err := mm.Create(typ, "Test "+typ, CreateOpts{Tags: []string{"test"}})
		if err != nil {
			t.Fatalf("create %s: %v", typ, err)
		}

		// ID should NOT have global- prefix
		if strings.HasPrefix(m.ID, "global-") {
			t.Errorf("%s: unexpected global- prefix: %s", typ, m.ID)
		}

		// File should exist in local dir, NOT global dir
		localPath := filepath.Join(localRoot, "nodes", m.ID+".md")
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			t.Errorf("%s: file not found in local dir: %s", typ, localPath)
		}
		globalPath := filepath.Join(globalRoot, "nodes", m.ID+".md")
		if _, err := os.Stat(globalPath); err == nil {
			t.Errorf("%s: file unexpectedly found in global dir: %s", typ, globalPath)
		}
	}
}

func TestGlobalRouting_LocalFlagOverride(t *testing.T) {
	localRoot, globalRoot, mm := setupGlobalTestMemory(t)

	m, err := mm.Create("decision", "Local decision", CreateOpts{
		Tags:  []string{"test"},
		Local: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Should NOT have global- prefix
	if strings.HasPrefix(m.ID, "global-") {
		t.Errorf("expected non-global ID, got %s", m.ID)
	}

	// File in local, not global
	localPath := filepath.Join(localRoot, "nodes", m.ID+".md")
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Errorf("file not found in local dir: %s", localPath)
	}
	globalPath := filepath.Join(globalRoot, "nodes", m.ID+".md")
	if _, err := os.Stat(globalPath); err == nil {
		t.Errorf("file unexpectedly found in global dir: %s", globalPath)
	}

	// OriginProject should be empty for --local motes
	if m.OriginProject != "" {
		t.Errorf("expected empty OriginProject, got %q", m.OriginProject)
	}
}

func TestGlobalRouting_OriginProject(t *testing.T) {
	_, _, mm := setupGlobalTestMemory(t)

	// Global mote should have OriginProject set to "myproject" (from path .../myproject/.memory)
	global, err := mm.Create("lesson", "Global lesson", CreateOpts{Tags: []string{"test"}})
	if err != nil {
		t.Fatalf("create global: %v", err)
	}
	if global.OriginProject != "myproject" {
		t.Errorf("expected OriginProject=myproject, got %q", global.OriginProject)
	}

	// Local task should have empty OriginProject
	local, err := mm.Create("task", "Local task", CreateOpts{Tags: []string{"test"}})
	if err != nil {
		t.Fatalf("create local: %v", err)
	}
	if local.OriginProject != "" {
		t.Errorf("expected empty OriginProject for task, got %q", local.OriginProject)
	}
}

func TestReadAllWithGlobal_MergesBothScopes(t *testing.T) {
	_, _, mm := setupGlobalTestMemory(t)

	// Create local task + global lesson
	localMote, err := mm.Create("task", "Local task", CreateOpts{Tags: []string{"local"}})
	if err != nil {
		t.Fatalf("create local: %v", err)
	}
	globalMote, err := mm.Create("lesson", "Global lesson", CreateOpts{Tags: []string{"global"}})
	if err != nil {
		t.Fatalf("create global: %v", err)
	}

	all, err := mm.ReadAllWithGlobal()
	if err != nil {
		t.Fatalf("ReadAllWithGlobal: %v", err)
	}

	foundLocal, foundGlobal := false, false
	for _, m := range all {
		if m.ID == localMote.ID {
			foundLocal = true
		}
		if m.ID == globalMote.ID {
			foundGlobal = true
		}
	}
	if !foundLocal {
		t.Error("local mote not found in merged results")
	}
	if !foundGlobal {
		t.Error("global mote not found in merged results")
	}
}

func TestReadAllWithGlobal_LocalPrecedence(t *testing.T) {
	localRoot, globalRoot, mm := setupGlobalTestMemory(t)

	// Create a mote manually with the same ID in both directories
	sharedID := "global-Ltest00000precedence"
	localContent := `---
id: ` + sharedID + `
type: lesson
status: active
title: Local version
tags: [local]
weight: 0.5
origin: normal
created_at: 2026-01-01T00:00:00Z
---
Local body`
	globalContent := `---
id: ` + sharedID + `
type: lesson
status: active
title: Global version
tags: [global]
weight: 0.5
origin: normal
created_at: 2026-01-01T00:00:00Z
---
Global body`

	if err := os.WriteFile(filepath.Join(localRoot, "nodes", sharedID+".md"), []byte(localContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "nodes", sharedID+".md"), []byte(globalContent), 0644); err != nil {
		t.Fatal(err)
	}

	all, err := mm.ReadAllWithGlobal()
	if err != nil {
		t.Fatalf("ReadAllWithGlobal: %v", err)
	}

	count := 0
	for _, m := range all {
		if m.ID == sharedID {
			count++
			if m.Title != "Local version" {
				t.Errorf("expected local version to win, got title %q", m.Title)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 copy of %s, got %d", sharedID, count)
	}
}

func TestBuildInMemory_CrossScopeEdges(t *testing.T) {
	localRoot, _, mm := setupGlobalTestMemory(t)

	// Create global decision first
	globalMote, err := mm.Create("decision", "Global decision", CreateOpts{Tags: []string{"arch"}})
	if err != nil {
		t.Fatalf("create global: %v", err)
	}

	// Create local task that depends on the global decision
	localMote, err := mm.Create("task", "Local task", CreateOpts{Tags: []string{"arch"}})
	if err != nil {
		t.Fatalf("create local: %v", err)
	}
	// Add the cross-scope edge via Link
	im := NewIndexManager(localRoot)
	all, err := mm.ReadAllWithGlobal()
	if err != nil {
		t.Fatalf("ReadAllWithGlobal for index: %v", err)
	}
	im.BuildInMemory(all)
	if err := mm.Link(localMote.ID, "depends_on", globalMote.ID, im); err != nil {
		t.Fatalf("Link: %v", err)
	}

	// Rebuild unified graph after adding edge
	all, err = mm.ReadAllWithGlobal()
	if err != nil {
		t.Fatalf("ReadAllWithGlobal: %v", err)
	}
	idx := im.BuildInMemory(all)

	// Verify cross-scope edge resolves
	edges := idx.Neighbors(localMote.ID, map[string]bool{"depends_on": true})
	if len(edges) == 0 {
		t.Error("expected cross-scope depends_on edge, found none")
	}
	found := false
	for _, e := range edges {
		if e.Target == globalMote.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected edge to %s, edges: %+v", globalMote.ID, edges)
	}

	// Verify reverse edge
	incoming := idx.Incoming(globalMote.ID, map[string]bool{"depends_on": true})
	if len(incoming) == 0 {
		t.Error("expected incoming depends_on edge on global mote, found none")
	}
}
