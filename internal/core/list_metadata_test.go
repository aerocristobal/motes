// SPDX-License-Identifier: MIT
//
// STORY-MQRY-001 — core-layer filter tests for --metadata-field and
// --has-metadata-key. Covers exact match, AND across multiple entries,
// presence checks, composition with --status/--ready/--tag, and empty-state
// preservation. CLI-layer tests live in cmd/mote/cmd_ls_metadata_test.go.
package core

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// metaT0 — reference instant for these tests; independent of other suites.
var metaT0 = time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

// writeRawMote writes a raw mote file to the manager's nodes dir. This avoids
// going through Create() so test cases can set raw YAML values (e.g.,
// `execution_parallel_group: ""`) that the writer might omit.
func writeRawMote(t *testing.T, mm *MoteManager, id, frontmatter, body string) {
	t.Helper()
	nodesDir := filepath.Join(mm.Root(), "nodes")
	if err := os.MkdirAll(nodesDir, 0755); err != nil {
		t.Fatalf("mkdir nodes: %v", err)
	}
	path := filepath.Join(nodesDir, id+".md")
	content := "---\n" + frontmatter + "---\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", id, err)
	}
}

// seedFixture seeds the BDD fixture from §2 Background:
//
//	| id      | status   | execution_mode | execution_parallel_group |
//	| motes-1 | active   | parallel       | group-A                  |
//	| motes-2 | active   | parallel       | group-A                  |
//	| motes-3 | active   | parallel       | group-B                  |
//	| motes-4 | active   | delegated      |                          |
//	| motes-5 | active   |                |                          |
//	| motes-6 | completed| parallel       | group-A                  |
func seedFixture(t *testing.T) *MoteManager {
	t.Helper()
	mm, _ := newManagerAt(t, metaT0)
	writeRawMote(t, mm, "motes-1", `id: motes-1
type: task
status: active
title: m1
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`, "")
	writeRawMote(t, mm, "motes-2", `id: motes-2
type: task
status: active
title: m2
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`, "")
	writeRawMote(t, mm, "motes-3", `id: motes-3
type: task
status: active
title: m3
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-B
`, "")
	writeRawMote(t, mm, "motes-4", `id: motes-4
type: task
status: active
title: m4
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: delegated
`, "")
	writeRawMote(t, mm, "motes-5", `id: motes-5
type: task
status: active
title: m5
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
`, "")
	writeRawMote(t, mm, "motes-6", `id: motes-6
type: task
status: completed
title: m6
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
execution_parallel_group: group-A
`, "")
	return mm
}

func gotIDs(motes []*Mote) []string {
	out := make([]string, len(motes))
	for i, m := range motes {
		out[i] = m.ID
	}
	sort.Strings(out)
	return out
}

func assertIDs(t *testing.T, got []*Mote, want ...string) {
	t.Helper()
	sort.Strings(want)
	gotSorted := gotIDs(got)
	if len(gotSorted) != len(want) {
		t.Fatalf("got %v, want %v", gotSorted, want)
	}
	for i, id := range want {
		if gotSorted[i] != id {
			t.Fatalf("got %v, want %v", gotSorted, want)
		}
	}
}

// ---- MetadataFields filter -------------------------------------------------

func TestList_MetadataField_ExactMatch(t *testing.T) {
	mm := seedFixture(t)
	got, err := mm.List(ListFilters{
		MetadataFields: map[string]string{"execution_mode": "parallel"},
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertIDs(t, got, "motes-1", "motes-2", "motes-3", "motes-6")
}

func TestList_MetadataField_NoMatch_Empty(t *testing.T) {
	mm := seedFixture(t)
	got, err := mm.List(ListFilters{
		MetadataFields: map[string]string{"execution_mode": "batch"},
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", gotIDs(got))
	}
}

func TestList_MetadataField_CaseSensitive(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		MetadataFields: map[string]string{"execution_mode": "Parallel"},
	})
	if len(got) != 0 {
		t.Errorf("case-sensitive match failed: %v", gotIDs(got))
	}
}

func TestList_MetadataField_TwoFields_AND(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		MetadataFields: map[string]string{
			"execution_mode":           "parallel",
			"execution_parallel_group": "group-A",
		},
	})
	assertIDs(t, got, "motes-1", "motes-2", "motes-6")
}

func TestList_MetadataField_OneMismatch_Excluded(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		MetadataFields: map[string]string{
			"execution_mode":           "parallel",
			"execution_parallel_group": "group-X",
		},
	})
	if len(got) != 0 {
		t.Errorf("got %v, want empty", gotIDs(got))
	}
}

// ---- HasMetadataKeys filter ------------------------------------------------

func TestList_HasMetadataKey_Present(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		HasMetadataKeys: []string{"execution_parallel_group"},
	})
	assertIDs(t, got, "motes-1", "motes-2", "motes-3", "motes-6")
}

func TestList_HasMetadataKey_AbsentExcluded(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		HasMetadataKeys: []string{"execution_does_not_exist"},
	})
	if len(got) != 0 {
		t.Errorf("got %v, want empty", gotIDs(got))
	}
}

func TestList_HasMetadataKey_PresentEmptyValue(t *testing.T) {
	mm := seedFixture(t)
	// Seed an extra mote with execution_parallel_group set to "".
	writeRawMote(t, mm, "motes-7", `id: motes-7
type: task
status: active
title: m7
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_parallel_group: ""
`, "")
	got, _ := mm.List(ListFilters{
		HasMetadataKeys: []string{"execution_parallel_group"},
	})
	// motes-7 must appear (key present, value "").
	var found bool
	for _, m := range got {
		if m.ID == "motes-7" {
			found = true
		}
	}
	if !found {
		t.Errorf("motes-7 (key=empty string) must appear; got %v", gotIDs(got))
	}
}

func TestList_HasMetadataKey_TwoKeys_AND(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		HasMetadataKeys: []string{"execution_mode", "execution_parallel_group"},
	})
	// motes-4 has mode but no group → excluded.
	assertIDs(t, got, "motes-1", "motes-2", "motes-3", "motes-6")
}

// ---- Mixed filters ---------------------------------------------------------

func TestList_MetadataField_AND_HasMetadataKey(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		MetadataFields:  map[string]string{"execution_mode": "parallel"},
		HasMetadataKeys: []string{"execution_parallel_group"},
	})
	assertIDs(t, got, "motes-1", "motes-2", "motes-3", "motes-6")
}

// ---- Composition with existing filters ------------------------------------

func TestList_MetadataField_WithStatus(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		Status:         "active",
		MetadataFields: map[string]string{"execution_mode": "parallel"},
	})
	// motes-6 excluded by status=active.
	assertIDs(t, got, "motes-1", "motes-2", "motes-3")
}

func TestList_MetadataField_WithReady(t *testing.T) {
	mm := seedFixture(t)
	got, _ := mm.List(ListFilters{
		Ready:          true,
		MetadataFields: map[string]string{"execution_mode": "parallel"},
	})
	// All seeded motes are tasks with no blockers; status active → ready.
	// motes-6 (completed) → not ready. motes-4 (delegated) → not parallel.
	assertIDs(t, got, "motes-1", "motes-2", "motes-3")
}

func TestList_MetadataField_WithReadyAndDeferred(t *testing.T) {
	// STORY-TIME-001 composition guard: a deferred parallel-mode mote must
	// be hidden from `--ready --metadata-field execution_mode=parallel`.
	mm := seedFixture(t)
	future := metaT0.Add(24 * time.Hour).Format(time.RFC3339)
	writeRawMote(t, mm, "motes-d", `id: motes-d
type: task
status: active
title: deferred parallel
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_mode: parallel
defer_until: `+future+`
`, "")
	got, _ := mm.List(ListFilters{
		Ready:          true,
		MetadataFields: map[string]string{"execution_mode": "parallel"},
	})
	for _, m := range got {
		if m.ID == "motes-d" {
			t.Errorf("deferred mote must be hidden; got %v", gotIDs(got))
		}
	}
}

func TestList_MetadataField_WithTag(t *testing.T) {
	mm := seedFixture(t)
	// Add the swarm tag to motes-1 and motes-2 only.
	writeRawMote(t, mm, "motes-1", `id: motes-1
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
`, "")
	writeRawMote(t, mm, "motes-2", `id: motes-2
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
`, "")
	got, _ := mm.List(ListFilters{
		Tag:            "swarm",
		MetadataFields: map[string]string{"execution_mode": "parallel"},
	})
	assertIDs(t, got, "motes-1", "motes-2")
}

// ---- §23.16 empty-state preservation --------------------------------------

func TestList_MetadataField_NoMatches_EmptySlice(t *testing.T) {
	mm := seedFixture(t)
	got, err := mm.List(ListFilters{
		MetadataFields: map[string]string{"execution_mode": "batch"},
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", gotIDs(got))
	}
}

func TestList_HasMetadataKey_UnknownKey_EmptySlice(t *testing.T) {
	mm := seedFixture(t)
	got, err := mm.List(ListFilters{
		HasMetadataKeys: []string{"execution_does_not_exist"},
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", gotIDs(got))
	}
}

// ---- Empty value matches empty string exactly -----------------------------

func TestList_MetadataField_EmptyValue_MatchesExplicitEmptyString(t *testing.T) {
	mm := seedFixture(t)
	writeRawMote(t, mm, "motes-7", `id: motes-7
type: task
status: active
title: m7
created_at: 2026-05-25T00:00:00Z
last_accessed: null
access_count: 0
weight: 0.5
execution_parallel_group: ""
`, "")
	got, _ := mm.List(ListFilters{
		MetadataFields: map[string]string{"execution_parallel_group": ""},
	})
	// motes-7 must be present; motes-1..6 (which have non-empty group or no
	// group at all) must NOT be present.
	assertIDs(t, got, "motes-7")
}
