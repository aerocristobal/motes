// SPDX-License-Identifier: MIT
//
// STORY-JSCHEMA-001 Scenario 8 — `mote doctor` flags drift between the
// compile-time registry of JSON shapes and what docs/JSON_SCHEMA.md
// documents. These tests exercise jsonSchemaDocDrift directly so we don't
// need a full repo checkout to validate the check.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/jsonenv"
)

func TestDoctorJSONSchema_AllShapesDocumented_NoFindings(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "JSON_SCHEMA.md")
	contents := strings.Join(jsonenv.RegisteredShapes(), "\n")
	if err := os.WriteFile(docPath, []byte(contents), 0644); err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	got := jsonSchemaDocDrift(docPath, jsonenv.RegisteredShapes())
	if len(got) != 0 {
		t.Fatalf("expected no findings when every shape is documented, got %d: %+v", len(got), got)
	}
}

func TestDoctorJSONSchema_MissingShape_OneFindingPerShape(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "JSON_SCHEMA.md")
	// Document everything except show.short.v1 and error.v1.
	var kept []string
	for _, s := range jsonenv.RegisteredShapes() {
		if s == "show.short.v1" || s == "error.v1" {
			continue
		}
		kept = append(kept, s)
	}
	if err := os.WriteFile(docPath, []byte(strings.Join(kept, "\n")), 0644); err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	got := jsonSchemaDocDrift(docPath, jsonenv.RegisteredShapes())
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(got), got)
	}
	codes := map[string]bool{}
	for _, iss := range got {
		if iss.Category != "undocumented_json_shape" {
			t.Errorf("category = %q, want undocumented_json_shape", iss.Category)
		}
		codes[iss.MoteID] = true
	}
	if !codes["show.short.v1"] || !codes["error.v1"] {
		t.Errorf("missing shapes not flagged; got %v", codes)
	}
}

func TestDoctorJSONSchema_MissingDocFile_OneFinding(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "JSON_SCHEMA.md") // never created
	got := jsonSchemaDocDrift(docPath, jsonenv.RegisteredShapes())
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 finding when doc file is absent, got %d: %+v", len(got), got)
	}
	if got[0].Category != "json_schema_doc_missing" {
		t.Errorf("category = %q, want json_schema_doc_missing", got[0].Category)
	}
}

// TestDoctorJSONSchema_RealRepoDocs is a smoke check that the docs file
// shipped in this commit covers every registered shape. If a later PR adds a
// shape to RegisteredShapes() without updating docs/JSON_SCHEMA.md, this test
// fails locally before doctor would (i.e. before the CI lint task).
func TestDoctorJSONSchema_RealRepoDocs(t *testing.T) {
	// Walk up from the test working directory to find the repo root marker.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	var docPath string
	for {
		candidate := filepath.Join(dir, "docs", "JSON_SCHEMA.md")
		if _, err := os.Stat(candidate); err == nil {
			docPath = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("docs/JSON_SCHEMA.md not found on filesystem walk; skipping repo-aware check")
		}
		dir = parent
	}
	got := jsonSchemaDocDrift(docPath, jsonenv.RegisteredShapes())
	if len(got) != 0 {
		t.Fatalf("real docs/JSON_SCHEMA.md is missing %d shape(s): %+v", len(got), got)
	}
}
