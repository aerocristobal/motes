// SPDX-License-Identifier: MIT
//
// STORY-MQRY-001 — `mote ls --metadata-field` / `--has-metadata-key` CLI
// integration. Mirrors the scenarios in §2 of the spec at the command-line
// boundary; the core-layer filter is exercised by
// internal/core/list_metadata_test.go.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

// writeFixtureMote writes a raw mote .md file into the workspace's nodes dir,
// bypassing Create() so the test can pin frontmatter shape exactly (including
// "execution_parallel_group: \"\""). Matches the BDD fixture from §2 Background.
func writeFixtureMote(t *testing.T, root, id, frontmatter string) {
	t.Helper()
	path := filepath.Join(root, "nodes", id+".md")
	content := "---\n" + frontmatter + "---\n\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", id, err)
	}
}

// seedLsFixture writes the BDD §2 Background fixture into root.
func seedLsFixture(t *testing.T, root string) {
	t.Helper()
	writeFixtureMote(t, root, "motes-1", `id: motes-1
type: task
status: active
title: m1
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`)
	writeFixtureMote(t, root, "motes-2", `id: motes-2
type: task
status: active
title: m2
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`)
	writeFixtureMote(t, root, "motes-3", `id: motes-3
type: task
status: active
title: m3
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-B
`)
	writeFixtureMote(t, root, "motes-4", `id: motes-4
type: task
status: active
title: m4
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: delegated
`)
	writeFixtureMote(t, root, "motes-5", `id: motes-5
type: task
status: active
title: m5
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
`)
	writeFixtureMote(t, root, "motes-6", `id: motes-6
type: task
status: completed
title: m6
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`)
}

func parseLsJSON(t *testing.T, stdout string) []string {
	t.Helper()
	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	ids := make([]string, len(parsed.Motes))
	for i, m := range parsed.Motes {
		ids[i] = m.ID
	}
	return ids
}

func assertIDSet(t *testing.T, got []string, want ...string) {
	t.Helper()
	gotSet := make(map[string]bool, len(got))
	for _, id := range got {
		gotSet[id] = true
	}
	wantSet := make(map[string]bool, len(want))
	for _, id := range want {
		wantSet[id] = true
	}
	if len(gotSet) != len(wantSet) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for id := range wantSet {
		if !gotSet[id] {
			t.Fatalf("missing %s; got %v, want %v", id, got, want)
		}
	}
}

// ---- Flag parsing -----------------------------------------------------------

func TestParseMetadataFieldFlag_HappyPath(t *testing.T) {
	k, v, err := parseMetadataFieldFlag("execution_mode=parallel")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if k != "execution_mode" || v != "parallel" {
		t.Errorf("got %q=%q, want execution_mode=parallel", k, v)
	}
}

func TestParseMetadataFieldFlag_SplitsOnFirstEqualsOnly(t *testing.T) {
	k, v, err := parseMetadataFieldFlag("foo=bar=baz")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if k != "foo" || v != "bar=baz" {
		t.Errorf("got %q=%q, want foo=bar=baz", k, v)
	}
}

func TestParseMetadataFieldFlag_EmptyValue(t *testing.T) {
	k, v, err := parseMetadataFieldFlag("execution_parallel_group=")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if k != "execution_parallel_group" || v != "" {
		t.Errorf("got %q=%q, want execution_parallel_group=\"\"", k, v)
	}
}

func TestParseMetadataFieldFlag_MissingEquals_Error(t *testing.T) {
	_, _, err := parseMetadataFieldFlag("execution_mode")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected format key=value") {
		t.Errorf("err = %v; want substring 'expected format key=value'", err)
	}
}

// ---- BDD scenario: --metadata-field exact match ---------------------------

func TestRunLs_MetadataField_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--metadata-field", "execution_mode=parallel", "--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2", "motes-3", "motes-6")
}

func TestRunLs_TwoMetadataFieldFlags_AND(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls",
			"--metadata-field", "execution_mode=parallel",
			"--metadata-field", "execution_parallel_group=group-A",
			"--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2", "motes-6")
}

// ---- --has-metadata-key ----------------------------------------------------

func TestRunLs_HasMetadataKey_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--has-metadata-key", "execution_parallel_group", "--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2", "motes-3", "motes-6")
}

func TestRunLs_TwoHasMetadataKeyFlags_AND(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls",
			"--has-metadata-key", "execution_mode",
			"--has-metadata-key", "execution_parallel_group",
			"--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	// motes-4 (mode but no group) excluded.
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2", "motes-3", "motes-6")
}

// ---- Mixed ----------------------------------------------------------------

func TestRunLs_MixedMetadataFlags_AND(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls",
			"--metadata-field", "execution_mode=parallel",
			"--has-metadata-key", "execution_parallel_group",
			"--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2", "motes-3", "motes-6")
}

// ---- Composition with existing filters ------------------------------------

func TestRunLs_MetadataField_WithStatus(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls",
			"--status=active",
			"--metadata-field", "execution_mode=parallel",
			"--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	// motes-6 excluded by status filter.
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2", "motes-3")
}

func TestRunLs_MetadataField_WithReady(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--ready",
			"--metadata-field", "execution_mode=parallel",
			"--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	// motes-6 not ready (completed). motes-4 not parallel.
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2", "motes-3")
}

func TestRunLs_MetadataField_WithTag(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)
	// Add the swarm tag to motes-1 and motes-2 only.
	writeFixtureMote(t, root, "motes-1", `id: motes-1
type: task
status: active
title: m1
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
tags:
  - swarm
execution_mode: parallel
execution_parallel_group: group-A
`)
	writeFixtureMote(t, root, "motes-2", `id: motes-2
type: task
status: active
title: m2
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
tags:
  - swarm
execution_mode: parallel
execution_parallel_group: group-A
`)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls",
			"--tag=swarm",
			"--metadata-field", "execution_mode=parallel",
			"--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	assertIDSet(t, parseLsJSON(t, stdout), "motes-1", "motes-2")
}

// ---- Empty-state preservation (§23.16) ------------------------------------

func TestRunLs_MetadataField_NoMatches_ReturnsEmptyArray(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--metadata-field", "execution_mode=batch", "--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	got := strings.TrimSpace(stdout)
	// Empty result paths are either the short-circuit {"motes":[]} or a
	// marshaled empty array — both are valid empty states for §23.16.
	if got != `{"motes":[]}` && got != "{\n  \"motes\": []\n}" {
		t.Errorf("got %q; want empty-motes JSON", got)
	}
}

func TestRunLs_HasMetadataKey_UnknownKey_ReturnsEmptyArray(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--has-metadata-key", "execution_does_not_exist", "--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	got := strings.TrimSpace(stdout)
	if got != `{"motes":[]}` && got != "{\n  \"motes\": []\n}" {
		t.Errorf("got %q; want empty-motes JSON", got)
	}
}

// ---- Value semantics ------------------------------------------------------

func TestRunLs_MetadataField_EmptyValue_MatchesEmptyString(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)
	writeFixtureMote(t, root, "motes-7", `id: motes-7
type: task
status: active
title: m7
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_parallel_group: ""
`)
	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--metadata-field", "execution_parallel_group=", "--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	assertIDSet(t, parseLsJSON(t, stdout), "motes-7")
}

func TestRunLs_MetadataField_ValueWithEquals(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	// Use a generic frontmatter key whose value contains "=" to verify
	// the parser splits only on the first '='. (The execution_* fields are
	// constrained at write time; we use a free-form key here.)
	writeFixtureMote(t, root, "motes-eq", `id: motes-eq
type: task
status: active
title: equals test
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
origin_project: "bar=baz"
`)
	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--metadata-field", "origin_project=bar=baz", "--json"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	assertIDSet(t, parseLsJSON(t, stdout), "motes-eq")
}

// ---- Security boundary at the CLI -----------------------------------------

func TestRunLs_MetadataField_MissingEquals_ExitNonZero(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := runLsViaCobra([]string{"ls", "--metadata-field", "execution_mode"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected format key=value") {
		t.Errorf("err = %v; want substring 'expected format key=value'", err)
	}
}

func TestRunLs_MetadataField_InvalidKey_ExitNonZero(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	for _, key := range []string{"execution.mode", "execution/mode", "../etc/passwd", "$(rm -rf ~)", "foo bar"} {
		err := runLsViaCobra([]string{"ls", "--metadata-field", key + "=value"})
		if err == nil {
			t.Errorf("key=%q expected error; got nil", key)
			continue
		}
		if !strings.Contains(err.Error(), "invalid metadata key") {
			t.Errorf("key=%q err = %v; want substring 'invalid metadata key'", key, err)
		}
	}
}

func TestRunLs_HasMetadataKey_InvalidKey_ExitNonZero(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := runLsViaCobra([]string{"ls", "--has-metadata-key", "execution.mode"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid metadata key") {
		t.Errorf("err = %v; want substring 'invalid metadata key'", err)
	}
}

func TestRunLs_HasMetadataKey_TooLong_ExitNonZero(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	longKey := strings.Repeat("a", 257)
	err := runLsViaCobra([]string{"ls", "--has-metadata-key", longKey})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "metadata key too long") {
		t.Errorf("err = %v; want substring 'metadata key too long'", err)
	}
}

func TestRunLs_MetadataField_InvalidValue_ExitNonZero(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	tooLong := "execution_mode=" + strings.Repeat("v", 4097)
	err := runLsViaCobra([]string{"ls", "--metadata-field", tooLong})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "metadata value") {
		t.Errorf("err = %v; want substring 'metadata value'", err)
	}
}

// ---- Output formats -------------------------------------------------------

func TestRunLs_MetadataField_Compact(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--metadata-field", "execution_mode=parallel", "--compact"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 compact lines, got %d:\n%s", len(lines), stdout)
	}
	for _, line := range lines {
		if !strings.Contains(line, ": ") {
			t.Errorf("compact line missing colon: %q", line)
		}
	}
}

func TestRunLs_MetadataField_Table(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedLsFixture(t, root)

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--metadata-field", "execution_mode=parallel"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	// STORY-HDRZ-001: the old printf-table column header (ID/TYPE/STATUS/
	// WEIGHT/TITLE) is dropped; the two-zone header rows are self-describing.
	// Only the per-mote presence assertions remain.
	for _, id := range []string{"motes-1", "motes-2", "motes-3", "motes-6"} {
		if !strings.Contains(stdout, id) {
			t.Errorf("ls missing %s; got %q", id, stdout)
		}
	}
}

// Compile-time guard against unused import.
var _ = core.NewMoteManager
