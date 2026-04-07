// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"motes/internal/core"
)

func TestImportFromJSONL(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)
	t.Setenv("MOTE_GLOBAL_ROOT", root)

	// Create JSONL file
	jsonlPath := filepath.Join(dir, "test.jsonl")
	f, _ := os.Create(jsonlPath)
	entries := []ExportMote{
		{Type: "lesson", Title: "Lesson One", Tags: []string{"go"}, Weight: 0.5, Origin: "normal", Body: "Body one"},
		{Type: "decision", Title: "Decision Two", Tags: []string{"arch"}, Weight: 0.6, Origin: "normal", Body: "Body two"},
	}
	enc := json.NewEncoder(f)
	for _, e := range entries {
		enc.Encode(e)
	}
	f.Close()

	// Import
	mm := core.NewMoteManager(root)
	file, _ := os.Open(jsonlPath)
	defer file.Close()

	// Read motes before (should be 0)
	before, _ := mm.ReadAllParallel()
	if len(before) != 0 {
		t.Fatalf("expected 0 motes before import, got %d", len(before))
	}

	// Manually simulate import logic
	scanner := json.NewDecoder(file)
	created := 0
	for scanner.More() {
		var em ExportMote
		if err := scanner.Decode(&em); err != nil {
			break
		}
		_, err := mm.Create(em.Type, em.Title, core.CreateOpts{
			Tags:   em.Tags,
			Weight: em.Weight,
			Origin: em.Origin,
			Body:   em.Body,
		})
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		created++
	}

	if created != 2 {
		t.Fatalf("expected 2 motes created, got %d", created)
	}

	after, _ := mm.ReadAllParallel()
	if len(after) != 2 {
		t.Fatalf("expected 2 motes after import, got %d", len(after))
	}
}
