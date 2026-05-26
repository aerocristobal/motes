// SPDX-License-Identifier: MIT
//
// STORY-MQRY-001 — `mote search --metadata-field` / `--has-metadata-key`
// CLI integration. Confirms the same filter surface is wired into search and
// that the positional query argument is still required (§4 Q6).
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetSearchFlags() {
	searchTopK = 10
	searchJSON = false
	searchType = ""
	searchTag = ""
	searchStatus = ""
	searchExcludeStatus = ""
	searchMetadataField = nil
	searchHasMetadataKey = nil
}

// runSearchViaCobra invokes `mote search ...` through cobra with output
// silencing matching runLsViaCobra.
func runSearchViaCobra(args []string) error {
	resetSearchFlags()
	defer resetSearchFlags()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

// writeFixtureMoteWithBody writes a raw mote .md file with both frontmatter
// and a body. Used by search tests to seed BM25-scorable text content.
func writeFixtureMoteWithBody(t *testing.T, root, id, frontmatter, body string) {
	t.Helper()
	path := filepath.Join(root, "nodes", id+".md")
	content := "---\n" + frontmatter + "---\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", id, err)
	}
}

// seedSearchFixture seeds the BDD §2 fixture *plus* body text so BM25 has
// something to score. motes-1 and motes-2 share the "index" token; motes-3
// does not.
func seedSearchFixture(t *testing.T, root string) {
	t.Helper()
	writeFixtureMoteWithBody(t, root, "motes-1", `id: motes-1
type: task
status: active
title: m1
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`, "rebuild the index for parallel dispatch")
	writeFixtureMoteWithBody(t, root, "motes-2", `id: motes-2
type: task
status: active
title: m2
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`, "compact the index gracefully")
	writeFixtureMoteWithBody(t, root, "motes-3", `id: motes-3
type: task
status: active
title: m3
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-B
`, "unrelated work — different topic entirely")
}

// ---- Tests ----------------------------------------------------------------

func TestRunSearch_MetadataField_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedSearchFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runSearchViaCobra([]string{"search", "index",
			"--metadata-field", "execution_mode=parallel",
			"--json"})
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var parsed SearchOutput
	if jerr := json.Unmarshal([]byte(stdout), &parsed); jerr != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, jerr)
	}
	gotIDs := make(map[string]bool, len(parsed.Results))
	for _, r := range parsed.Results {
		gotIDs[r.ID] = true
	}
	// Only motes-1 and motes-2 match the text "index"; motes-3 doesn't.
	if !gotIDs["motes-1"] || !gotIDs["motes-2"] {
		t.Errorf("missing expected matches; got %v", parsed.Results)
	}
	if gotIDs["motes-3"] {
		t.Errorf("motes-3 should not match text 'index'; got %v", parsed.Results)
	}
}

func TestRunSearch_HasMetadataKey_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedSearchFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runSearchViaCobra([]string{"search", "index",
			"--has-metadata-key", "execution_parallel_group",
			"--json"})
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var parsed SearchOutput
	if jerr := json.Unmarshal([]byte(stdout), &parsed); jerr != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, jerr)
	}
	gotIDs := make(map[string]bool, len(parsed.Results))
	for _, r := range parsed.Results {
		gotIDs[r.ID] = true
	}
	if !gotIDs["motes-1"] || !gotIDs["motes-2"] {
		t.Errorf("missing expected matches; got %v", parsed.Results)
	}
}

func TestRunSearch_PositionalQueryStillRequired(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := runSearchViaCobra([]string{"search", "--metadata-field", "execution_mode=parallel"})
	if err == nil {
		t.Fatal("expected error when query argument is missing")
	}
}

func TestRunSearch_MetadataField_InvalidKey_ExitNonZero(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := runSearchViaCobra([]string{"search", "anything", "--metadata-field", "execution.mode=parallel"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid metadata key") {
		t.Errorf("err = %v; want substring 'invalid metadata key'", err)
	}
}
