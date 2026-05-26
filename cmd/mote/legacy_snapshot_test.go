// SPDX-License-Identifier: MIT
//
// STORY-JSCHEMA-001 §8 sprint-review checklist: "byte-for-byte snapshot test
// of one representative command's legacy output should be unchanged from the
// pre-sprint commit". `mote ls --json` is the representative command — it is
// the most-consumed JSON emitter, and pulse delegates to the same branch.
//
// The snapshot is generated from a deterministic seed: one task mote with a
// fixed title. We compare the structural shape (parsed JSON keys and types)
// rather than the literal bytes because the mote ID is a non-deterministic
// UUID. If a later PR adds or removes a top-level key, this test trips.
package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/jsonenv"
)

func TestLs_LegacySnapshot_ShapeUnchanged(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "legacy snapshot probe", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	var rerr error
	stdout, _ := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--json"})
	})
	if rerr != nil {
		t.Fatalf("unexpected error: %v", rerr)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
		t.Fatalf("legacy stdout must parse, got %v over %q", err, stdout)
	}

	// Top-level key set is exactly {motes}.
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) != 1 || keys[0] != "motes" {
		t.Fatalf("legacy top-level keys changed: got %v, want [motes]", keys)
	}

	// motes is an array.
	motes, ok := got["motes"].([]any)
	if !ok {
		t.Fatalf("motes must be an array, got %T", got["motes"])
	}
	if len(motes) != 1 {
		t.Fatalf("motes len = %d, want 1", len(motes))
	}

	// First entry shape.
	first, ok := motes[0].(map[string]any)
	if !ok {
		t.Fatalf("motes[0] must be object, got %T", motes[0])
	}
	wantEntryKeys := []string{"id", "status", "title", "type", "weight"}
	gotEntryKeys := make([]string, 0, len(first))
	for k := range first {
		gotEntryKeys = append(gotEntryKeys, k)
	}
	sort.Strings(gotEntryKeys)
	if !equalStringSlices(gotEntryKeys, wantEntryKeys) {
		t.Fatalf("legacy motes[].keys changed: got %v, want %v", gotEntryKeys, wantEntryKeys)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
