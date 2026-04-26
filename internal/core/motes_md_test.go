// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateMotesIndex_EmptyNodes(t *testing.T) {
	root := t.TempDir()
	if err := GenerateMotesIndex(root); err != nil {
		t.Fatalf("GenerateMotesIndex: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, MotesIndexFilename))
	if err != nil {
		t.Fatalf("read MOTES.md: %v", err)
	}
	if !strings.Contains(string(got), "# Motes Memory Index") {
		t.Errorf("missing title heading\n%s", got)
	}
	if !strings.Contains(string(got), "no motes yet") {
		t.Errorf("missing empty placeholder\n%s", got)
	}
}

func TestGenerateMotesIndex_GroupsByTypeAndSortsByTitle(t *testing.T) {
	root := t.TempDir()
	nodes := filepath.Join(root, "nodes")
	mustMkdir(t, nodes)
	writeMote(t, nodes, "global-Lzulu", "lesson", "Zulu lesson", "Last alphabetically. Body text here.")
	writeMote(t, nodes, "global-Lalpha", "lesson", "Alpha lesson", "First alphabetically. Body text.")
	writeMote(t, nodes, "global-Dxyz", "decision", "Some decision", "Decision body.")

	if err := GenerateMotesIndex(root); err != nil {
		t.Fatalf("GenerateMotesIndex: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, MotesIndexFilename))
	out := string(body)

	// Decision section comes before Lesson alphabetically.
	dIdx := strings.Index(out, "## Decision")
	lIdx := strings.Index(out, "## Lesson")
	if dIdx < 0 || lIdx < 0 {
		t.Fatalf("missing section headers\n%s", out)
	}
	if dIdx > lIdx {
		t.Errorf("Decision should come before Lesson:\n%s", out)
	}

	// Within Lesson section, Alpha must precede Zulu.
	lessonSect := out[lIdx:]
	aIdx := strings.Index(lessonSect, "Alpha lesson")
	zIdx := strings.Index(lessonSect, "Zulu lesson")
	if aIdx < 0 || zIdx < 0 {
		t.Fatalf("missing lesson titles\n%s", out)
	}
	if aIdx > zIdx {
		t.Errorf("titles not sorted alphabetically:\n%s", lessonSect)
	}
}

func TestGenerateMotesIndex_SkipsTombstonesAndDeleted(t *testing.T) {
	root := t.TempDir()
	nodes := filepath.Join(root, "nodes")
	mustMkdir(t, nodes)

	// Live mote.
	writeMote(t, nodes, "global-Lalive", "lesson", "Live one", "Visible.")
	// Tombstone (forwarded).
	writeMoteWith(t, nodes, "global-Lold", "lesson", "Old forwarded", "x", map[string]string{
		"forwarded_to": "global-Lalive",
	})
	// Soft-deleted.
	writeMoteWith(t, nodes, "global-Ldead", "lesson", "Soft deleted", "x", map[string]string{
		"deleted_at": time.Now().UTC().Format(time.RFC3339),
	})

	if err := GenerateMotesIndex(root); err != nil {
		t.Fatalf("GenerateMotesIndex: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(root, MotesIndexFilename))
	s := string(out)

	if !strings.Contains(s, "Live one") {
		t.Errorf("expected live mote in index\n%s", s)
	}
	if strings.Contains(s, "Old forwarded") {
		t.Errorf("expected tombstone excluded\n%s", s)
	}
	if strings.Contains(s, "Soft deleted") {
		t.Errorf("expected soft-deleted excluded\n%s", s)
	}
}

func TestGenerateMotesIndex_HookFromBody(t *testing.T) {
	root := t.TempDir()
	nodes := filepath.Join(root, "nodes")
	mustMkdir(t, nodes)
	writeMote(t, nodes, "global-Labc", "lesson", "Sample", "# heading line\n\nThis is the real hook content for the index.")

	if err := GenerateMotesIndex(root); err != nil {
		t.Fatalf("GenerateMotesIndex: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(root, MotesIndexFilename))
	if !strings.Contains(string(out), "This is the real hook content") {
		t.Errorf("hook not extracted from body:\n%s", out)
	}
	if strings.Contains(string(out), "heading line") {
		t.Errorf("# heading should not be used as hook:\n%s", out)
	}
}

func TestGenerateMotesIndex_MalformedSkipped(t *testing.T) {
	root := t.TempDir()
	nodes := filepath.Join(root, "nodes")
	mustMkdir(t, nodes)
	writeMote(t, nodes, "global-Lgood", "lesson", "Good one", "ok")
	mustWriteFile(t, filepath.Join(nodes, "broken.md"), "no frontmatter at all")

	if err := GenerateMotesIndex(root); err != nil {
		t.Fatalf("GenerateMotesIndex: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(root, MotesIndexFilename))
	if !strings.Contains(string(out), "Good one") {
		t.Errorf("expected good mote despite broken sibling\n%s", out)
	}
}

func TestGenerateMotesIndex_NoNodesDir(t *testing.T) {
	root := t.TempDir() // no nodes/ subdir
	if err := GenerateMotesIndex(root); err != nil {
		t.Fatalf("GenerateMotesIndex: %v", err)
	}
	mustExist(t, filepath.Join(root, MotesIndexFilename))
}

// --- helpers ---

func writeMote(t *testing.T, dir, id, mtype, title, body string) {
	t.Helper()
	writeMoteWith(t, dir, id, mtype, title, body, nil)
}

func writeMoteWith(t *testing.T, dir, id, mtype, title, body string, extra map[string]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + id + "\n")
	b.WriteString("type: " + mtype + "\n")
	b.WriteString("status: active\n")
	b.WriteString("title: \"" + title + "\"\n")
	b.WriteString("tags: []\n")
	b.WriteString("weight: 0.5\n")
	b.WriteString("origin: normal\n")
	b.WriteString("created_at: 2026-01-01T00:00:00Z\n")
	b.WriteString("last_accessed: null\n")
	b.WriteString("access_count: 0\n")
	b.WriteString("depends_on: []\n")
	b.WriteString("blocks: []\n")
	b.WriteString("relates_to: []\n")
	b.WriteString("builds_on: []\n")
	b.WriteString("contradicts: []\n")
	b.WriteString("supersedes: []\n")
	b.WriteString("caused_by: []\n")
	b.WriteString("informed_by: []\n")
	for k, v := range extra {
		b.WriteString(k + ": " + v + "\n")
	}
	b.WriteString("---\n")
	b.WriteString(body)
	mustWriteFile(t, filepath.Join(dir, id+".md"), b.String())
}
