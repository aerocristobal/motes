// SPDX-License-Identifier: MIT
//
// STORY-MQRY-001 — ParseMote must populate RawFrontmatter and preserve the
// distinction between `key: ""` (explicit empty string) and an absent key.
// Without this round-trip property, the `--metadata-field key=` scenario in
// the BDD spec cannot pass.
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFrontmatterFile(t *testing.T, fm string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "node.md")
	body := "---\n" + fm + "---\n\nbody text\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestParseMote_RawFrontmatter_CapturesAllKeys(t *testing.T) {
	path := writeFrontmatterFile(t, `id: motes-Test1
type: task
status: active
title: t
execution_mode: parallel
execution_parallel_group: group-A
`)
	m, err := ParseMote(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.RawFrontmatter == nil {
		t.Fatal("RawFrontmatter is nil")
	}
	if v, ok := m.RawFrontmatter["execution_mode"]; !ok || v != "parallel" {
		t.Errorf("RawFrontmatter[execution_mode] = %v (ok=%v); want parallel", v, ok)
	}
	if v, ok := m.RawFrontmatter["execution_parallel_group"]; !ok || v != "group-A" {
		t.Errorf("RawFrontmatter[execution_parallel_group] = %v (ok=%v); want group-A", v, ok)
	}
}

func TestParseMote_RawFrontmatter_EmptyStringVsAbsent(t *testing.T) {
	// Mote with execution_parallel_group explicitly set to empty string.
	emptyPath := writeFrontmatterFile(t, `id: motes-Empty
type: task
status: active
title: e
execution_parallel_group: ""
`)
	// Mote with the key absent.
	absentPath := writeFrontmatterFile(t, `id: motes-Absent
type: task
status: active
title: a
`)

	empty, err := ParseMote(emptyPath)
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	absent, err := ParseMote(absentPath)
	if err != nil {
		t.Fatalf("parse absent: %v", err)
	}

	v, present := empty.RawFrontmatter["execution_parallel_group"]
	if !present {
		t.Error("empty mote: RawFrontmatter must contain execution_parallel_group")
	}
	if v != "" {
		t.Errorf("empty mote: value = %v; want \"\"", v)
	}

	if _, present := absent.RawFrontmatter["execution_parallel_group"]; present {
		t.Error("absent mote: RawFrontmatter must NOT contain execution_parallel_group")
	}
}

func TestParseMote_RawFrontmatter_UnknownKeysPreserved(t *testing.T) {
	// A key not declared on the typed Mote struct must still appear in
	// RawFrontmatter so forward-compat metadata filters work.
	path := writeFrontmatterFile(t, `id: motes-Unk
type: task
status: active
title: u
custom_unknown_key: custom_value
`)
	m, err := ParseMote(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v, ok := m.RawFrontmatter["custom_unknown_key"]; !ok || v != "custom_value" {
		t.Errorf("unknown key not preserved: got %v (ok=%v); want custom_value", v, ok)
	}
}

func TestMotePassesMetadata_EmptyFilters_AlwaysMatch(t *testing.T) {
	m := &Mote{RawFrontmatter: map[string]any{}}
	if !MotePassesMetadata(m, nil, nil) {
		t.Error("empty filters must match every mote")
	}
}

func TestMotePassesMetadata_NilRawFrontmatter_OnlyEmptyFilterMatches(t *testing.T) {
	m := &Mote{RawFrontmatter: nil}
	if !MotePassesMetadata(m, nil, nil) {
		t.Error("nil raw + empty filter must match")
	}
	if MotePassesMetadata(m, map[string]string{"k": "v"}, nil) {
		t.Error("nil raw + non-empty filter must not match")
	}
	if MotePassesMetadata(m, nil, []string{"k"}) {
		t.Error("nil raw + non-empty hasKeys must not match")
	}
}

func TestMotePassesMetadata_NumericScalar_Stringified(t *testing.T) {
	// YAML scalar integers/floats unmarshal as int/float64 — the filter
	// stringifies them so users can write --metadata-field weight=0.5.
	m := &Mote{RawFrontmatter: map[string]any{
		"weight": 0.5,
		"count":  42,
		"flag":   true,
	}}
	if !MotePassesMetadata(m, map[string]string{"weight": "0.5"}, nil) {
		t.Error("float scalar should stringify to 0.5")
	}
	if !MotePassesMetadata(m, map[string]string{"count": "42"}, nil) {
		t.Error("int scalar should stringify to 42")
	}
	if !MotePassesMetadata(m, map[string]string{"flag": "true"}, nil) {
		t.Error("bool scalar should stringify to true")
	}
}

func TestMotePassesMetadata_NilValue_MatchesEmptyString(t *testing.T) {
	m := &Mote{RawFrontmatter: map[string]any{"k": nil}}
	if !MotePassesMetadata(m, map[string]string{"k": ""}, nil) {
		t.Error("YAML null should match empty-string filter")
	}
}
