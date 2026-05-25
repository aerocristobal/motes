// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"motes/internal/core"
)

// Scenario 12: mote export --json emits a top-level object with both
// "motes" and "memories" arrays.
func TestExport_EnvelopeIncludesMemoriesAndMotes(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "Auth decision", Tags: []string{"auth"}, Body: "We chose OAuth."},
	})
	store := core.NewMemoryStore(root)
	if _, err := store.Put("auth-jwt", "JWT not sessions", "test", core.PutOpts{}); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	output := captureStdout(func() {
		_ = runExport(exportCmd, nil)
	})

	var env ExportEnvelope
	if err := json.Unmarshal([]byte(output), &env); err != nil {
		t.Fatalf("invalid envelope JSON: %v\n%s", err, output)
	}
	if len(env.Motes) != 1 || env.Motes[0].Title != "Auth decision" {
		t.Errorf("motes array wrong: %+v", env.Motes)
	}
	if len(env.Memories) != 1 || env.Memories[0].Key != "auth-jwt" {
		t.Errorf("memories array wrong: %+v", env.Memories)
	}
	if env.Memories[0].Body != "JWT not sessions" {
		t.Errorf("memory body: %q", env.Memories[0].Body)
	}
	if env.Memories[0].CreatedAt.IsZero() {
		t.Error("memory created_at should be set")
	}
	if env.Memories[0].UpdatedAt.IsZero() {
		t.Error("memory updated_at should be set")
	}
}

// Empty memories store still emits a memories array (not omitted).
func TestExport_EmptyMemoriesEmitsEmptyArray(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Some task", Tags: []string{"x"}},
	})

	output := captureStdout(func() {
		_ = runExport(exportCmd, nil)
	})
	if !strings.Contains(output, `"memories": []`) && !strings.Contains(output, `"memories":[]`) {
		t.Errorf("expected empty memories array in envelope:\n%s", output)
	}
	var env ExportEnvelope
	if err := json.Unmarshal([]byte(output), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.Memories == nil {
		t.Error("memories array should be initialized (not nil) for schema stability")
	}
	if len(env.Memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(env.Memories))
	}
	_ = root
}
