// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMote_RoundTrip(t *testing.T) {
	now := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	accessed := time.Date(2025, 3, 14, 8, 0, 0, 0, time.UTC)

	original := &Mote{
		ID:           "proj-T1abc23",
		Type:         "task",
		Status:       "active",
		Title:        "Implement OAuth flow",
		Tags:         []string{"oauth", "auth", "api"},
		Weight:       0.8,
		Origin:       "normal",
		CreatedAt:    now,
		LastAccessed: &accessed,
		AccessCount:  5,
		DependsOn:    []string{"proj-D2xyz45"},
		Blocks:       []string{"proj-T3def67"},
		RelatesTo:    []string{"proj-L4ghi89"},
		BuildsOn:     []string{"proj-D5jkl01"},
		Contradicts:  nil,
		Supersedes:   nil,
		CausedBy:     nil,
		InformedBy:   nil,
		Body:         "This is the body content.\n\nIt has multiple paragraphs.\n",
	}

	data, err := SerializeMote(original)
	if err != nil {
		t.Fatalf("SerializeMote: %v", err)
	}

	// Write to temp file and parse back
	dir := t.TempDir()
	path := filepath.Join(dir, "test-mote.md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	parsed, err := ParseMote(path)
	if err != nil {
		t.Fatalf("ParseMote: %v", err)
	}

	// Verify all fields
	if parsed.ID != original.ID {
		t.Errorf("ID: got %q, want %q", parsed.ID, original.ID)
	}
	if parsed.Type != original.Type {
		t.Errorf("Type: got %q, want %q", parsed.Type, original.Type)
	}
	if parsed.Status != original.Status {
		t.Errorf("Status: got %q, want %q", parsed.Status, original.Status)
	}
	if parsed.Title != original.Title {
		t.Errorf("Title: got %q, want %q", parsed.Title, original.Title)
	}
	if len(parsed.Tags) != len(original.Tags) {
		t.Errorf("Tags len: got %d, want %d", len(parsed.Tags), len(original.Tags))
	}
	for i, tag := range parsed.Tags {
		if tag != original.Tags[i] {
			t.Errorf("Tags[%d]: got %q, want %q", i, tag, original.Tags[i])
		}
	}
	if parsed.Weight != original.Weight {
		t.Errorf("Weight: got %f, want %f", parsed.Weight, original.Weight)
	}
	if parsed.Origin != original.Origin {
		t.Errorf("Origin: got %q, want %q", parsed.Origin, original.Origin)
	}
	if !parsed.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", parsed.CreatedAt, original.CreatedAt)
	}
	if parsed.LastAccessed == nil {
		t.Fatal("LastAccessed is nil")
	}
	if !parsed.LastAccessed.Equal(*original.LastAccessed) {
		t.Errorf("LastAccessed: got %v, want %v", *parsed.LastAccessed, *original.LastAccessed)
	}
	if parsed.AccessCount != original.AccessCount {
		t.Errorf("AccessCount: got %d, want %d", parsed.AccessCount, original.AccessCount)
	}
	if len(parsed.DependsOn) != 1 || parsed.DependsOn[0] != "proj-D2xyz45" {
		t.Errorf("DependsOn: got %v", parsed.DependsOn)
	}
	if len(parsed.Blocks) != 1 || parsed.Blocks[0] != "proj-T3def67" {
		t.Errorf("Blocks: got %v", parsed.Blocks)
	}
	if len(parsed.RelatesTo) != 1 || parsed.RelatesTo[0] != "proj-L4ghi89" {
		t.Errorf("RelatesTo: got %v", parsed.RelatesTo)
	}
	if len(parsed.BuildsOn) != 1 || parsed.BuildsOn[0] != "proj-D5jkl01" {
		t.Errorf("BuildsOn: got %v", parsed.BuildsOn)
	}
	if parsed.Body != original.Body {
		t.Errorf("Body: got %q, want %q", parsed.Body, original.Body)
	}
	if parsed.FilePath != path {
		t.Errorf("FilePath: got %q, want %q", parsed.FilePath, path)
	}
}

func TestParseMote_NilLastAccessed(t *testing.T) {
	content := `---
id: proj-T1abc
type: task
status: active
title: Test
weight: 0.5
origin: normal
created_at: 2025-03-15T10:00:00Z
access_count: 0
---
Body here.
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := ParseMote(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.LastAccessed != nil {
		t.Errorf("LastAccessed should be nil, got %v", m.LastAccessed)
	}
}

func TestParseMote_MalformedYAML(t *testing.T) {
	content := `---
id: [invalid yaml
  this is broken: {{{
---
body
`
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseMote(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "bad frontmatter") {
		t.Errorf("error should mention bad frontmatter, got: %v", err)
	}
}

func TestParseMote_NoFrontmatter(t *testing.T) {
	content := "Just a plain file with no frontmatter at all."

	dir := t.TempDir()
	path := filepath.Join(dir, "plain.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseMote(path)
	if err == nil {
		t.Fatal("expected error for missing frontmatter, got nil")
	}
	if !strings.Contains(err.Error(), "no frontmatter") {
		t.Errorf("error should mention no frontmatter, got: %v", err)
	}
}

func TestParseMote_FileNotFound(t *testing.T) {
	_, err := ParseMote("/nonexistent/path/mote.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseMote_EmptyBody(t *testing.T) {
	content := `---
id: proj-T1abc
type: task
status: active
title: Empty body
weight: 0.5
origin: normal
created_at: 2025-03-15T10:00:00Z
access_count: 0
---
`
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-body.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := ParseMote(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "" {
		t.Errorf("Body should be empty, got %q", m.Body)
	}
}

func TestParseMote_OptionalFields(t *testing.T) {
	content := `---
id: proj-A1xyz
type: anchor
status: active
title: OAuth anchor
weight: 0.7
origin: normal
created_at: 2025-03-15T10:00:00Z
access_count: 0
source_issue: "#42"
strata_corpus: oauth-docs
strata_query_hint: token refresh
---
Anchor body.
`
	dir := t.TempDir()
	path := filepath.Join(dir, "anchor.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := ParseMote(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.SourceIssue != "#42" {
		t.Errorf("SourceIssue: got %q, want %q", m.SourceIssue, "#42")
	}
	if m.StrataCorpus != "oauth-docs" {
		t.Errorf("StrataCorpus: got %q, want %q", m.StrataCorpus, "oauth-docs")
	}
	if m.StrataQueryHint != "token refresh" {
		t.Errorf("StrataQueryHint: got %q, want %q", m.StrataQueryHint, "token refresh")
	}
}

func TestSerializeMote_EmptySlices(t *testing.T) {
	m := &Mote{
		ID:        "proj-T1abc",
		Type:      "task",
		Status:    "active",
		Title:     "Test",
		Weight:    0.5,
		Origin:    "normal",
		CreatedAt: time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := SerializeMote(m)
	if err != nil {
		t.Fatal(err)
	}

	// Should still be parseable
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseMote(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ID != m.ID {
		t.Errorf("ID: got %q, want %q", parsed.ID, m.ID)
	}
}

func TestParseMote_CodeFilePaths(t *testing.T) {
	now := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	m := &Mote{
		ID:            "proj-A1xyz",
		Type:          "anchor",
		Status:        "active",
		Title:         "Score engine anchor",
		Weight:        0.5,
		Origin:        "normal",
		CreatedAt:     now,
		StrataCorpus:  "codebase",
		CodeFilePaths: []string{"internal/core/score.go", "internal/core/index.go"},
	}

	data, err := SerializeMote(m)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseMote(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(parsed.CodeFilePaths) != 2 {
		t.Fatalf("CodeFilePaths: got %d items, want 2", len(parsed.CodeFilePaths))
	}
	if parsed.CodeFilePaths[0] != "internal/core/score.go" {
		t.Errorf("CodeFilePaths[0]: got %q, want %q", parsed.CodeFilePaths[0], "internal/core/score.go")
	}
	if parsed.CodeFilePaths[1] != "internal/core/index.go" {
		t.Errorf("CodeFilePaths[1]: got %q, want %q", parsed.CodeFilePaths[1], "internal/core/index.go")
	}
}

func TestCreateOpts_CodeFilePaths(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)

	mm := NewMoteManager(root)
	m, err := mm.Create("anchor", "Test anchor", CreateOpts{
		CodeFilePaths: []string{"cmd/main.go"},
		StrataCorpus:  "codebase",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(m.CodeFilePaths) != 1 || m.CodeFilePaths[0] != "cmd/main.go" {
		t.Errorf("CodeFilePaths not set on created mote: %v", m.CodeFilePaths)
	}

	// Read back from disk
	read, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.CodeFilePaths) != 1 || read.CodeFilePaths[0] != "cmd/main.go" {
		t.Errorf("CodeFilePaths not persisted: %v", read.CodeFilePaths)
	}
}
