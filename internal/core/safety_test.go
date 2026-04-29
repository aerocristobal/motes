// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import "testing"

func TestIsLikelyTestTitle(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"Mote", true},
		{"Mote 1", true},
		{"Lesson B", true},
		{"BM25 lesson A", true},
		{"BM25 scoring lesson", true},
		{"Auth lesson B", true},
		{"Auth mote 2", true},
		{"Auth context 1", true},
		{"DB mote", true},
		{"Old mote", true},
		{"Old context", true},
		{"Test", true},
		{"Updated", true},
		{"Target mote", true},
		{"Access test A", true},
		{"Body one", true},
		{"'BM25 lesson A'", true}, // YAML quoted
		// Real titles must NOT match.
		{"Pi 4 (8GB) as single control-plane, all CM3+ as workers", false},
		{"k3s dual-stack requires reinstall, not runtime toggle", false},
		{"Cluster IP allocation map", false},
		{"Docker networking", false},
		{"Auth Domain", false}, // legitimate consolidated mote
		{"", false},
	}
	for _, c := range cases {
		got := IsLikelyTestTitle(c.title)
		if got != c.want {
			t.Errorf("IsLikelyTestTitle(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestBodyChars(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{"", 0},
		{"   \n\t\n   ", 0},
		{"abc", 3},
		{"  abc  def  ", 6},
		{"line one\nline two", 14},
	}
	for _, c := range cases {
		got := BodyChars(c.body)
		if got != c.want {
			t.Errorf("BodyChars(%q) = %d, want %d", c.body, got, c.want)
		}
	}
}

func TestReadGlobalMotes_FiltersContextAndDeprecated(t *testing.T) {
	// Use a fresh global dir for this test.
	gRoot := t.TempDir()
	t.Setenv("MOTE_GLOBAL_ROOT", gRoot)

	mm := NewMoteManager(t.TempDir())

	// Write directly to global nodes — bypassing Create so we can set type/status freely.
	gDir, err := mm.globalNodesDir()
	if err != nil {
		t.Fatalf("globalNodesDir: %v", err)
	}

	cases := []struct {
		id, typ, status string
		shouldSurface   bool
	}{
		{"global-decision-keep", "decision", "active", true},
		{"global-lesson-keep", "lesson", "active", true},
		{"global-context-skip", "context", "active", false},
		{"global-deprecated-skip", "decision", "deprecated", false},
		{"global-archived-skip", "lesson", "archived", false},
	}
	for _, c := range cases {
		m := &Mote{
			ID:     c.id,
			Type:   c.typ,
			Status: c.status,
			Title:  c.id,
			Body:   "Body content with enough length to clear any minimum-content guard.",
		}
		data, err := SerializeMote(m)
		if err != nil {
			t.Fatalf("serialize %s: %v", c.id, err)
		}
		if err := AtomicWrite(gDir+"/"+c.id+".md", data, 0644); err != nil {
			t.Fatalf("write %s: %v", c.id, err)
		}
	}

	got := mm.ReadGlobalMotes()
	gotIDs := map[string]bool{}
	for _, m := range got {
		gotIDs[m.ID] = true
	}
	for _, c := range cases {
		if c.shouldSurface && !gotIDs[c.id] {
			t.Errorf("%s should have surfaced from ReadGlobalMotes (type=%s status=%s)", c.id, c.typ, c.status)
		}
		if !c.shouldSurface && gotIDs[c.id] {
			t.Errorf("%s should have been filtered out of ReadGlobalMotes (type=%s status=%s)", c.id, c.typ, c.status)
		}
	}
}
