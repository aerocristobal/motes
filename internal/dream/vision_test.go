package dream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVisionWriter_WriteDrafts(t *testing.T) {
	dir := t.TempDir()
	vw := NewVisionWriter(dir)

	visions := []Vision{
		{Type: "link_suggestion", SourceMotes: []string{"m1"}, Rationale: "test"},
		{Type: "staleness", SourceMotes: []string{"m2"}, Rationale: "old"},
	}
	if err := vw.WriteDrafts(visions); err != nil {
		t.Fatal(err)
	}

	drafts := vw.ReadDrafts()
	if len(drafts) != 2 {
		t.Errorf("expected 2 drafts, got %d", len(drafts))
	}
}

func TestVisionWriter_WriteDrafts_Append(t *testing.T) {
	dir := t.TempDir()
	vw := NewVisionWriter(dir)

	vw.WriteDrafts([]Vision{{Type: "a", SourceMotes: []string{"m1"}, Rationale: "first"}})
	vw.WriteDrafts([]Vision{{Type: "b", SourceMotes: []string{"m2"}, Rationale: "second"}})

	drafts := vw.ReadDrafts()
	if len(drafts) != 2 {
		t.Errorf("expected 2 appended drafts, got %d", len(drafts))
	}
}

func TestVisionWriter_WriteFinal(t *testing.T) {
	dir := t.TempDir()
	vw := NewVisionWriter(dir)

	visions := []Vision{
		{Type: "contradiction", SourceMotes: []string{"a", "b"}, Rationale: "conflict"},
	}
	if err := vw.WriteFinal(visions); err != nil {
		t.Fatal(err)
	}

	final := vw.ReadFinal()
	if len(final) != 1 {
		t.Errorf("expected 1 final vision, got %d", len(final))
	}
}

func TestVisionWriter_ClearDrafts(t *testing.T) {
	dir := t.TempDir()
	vw := NewVisionWriter(dir)

	vw.WriteDrafts([]Vision{{Type: "a", SourceMotes: []string{"m1"}, Rationale: "test"}})
	vw.ClearDrafts()

	if _, err := os.Stat(filepath.Join(dir, "visions_draft.jsonl")); !os.IsNotExist(err) {
		t.Error("draft file should be removed after ClearDrafts")
	}
	drafts := vw.ReadDrafts()
	if len(drafts) != 0 {
		t.Errorf("expected 0 drafts after clear, got %d", len(drafts))
	}
}

func TestVisionWriter_ReadEmpty(t *testing.T) {
	dir := t.TempDir()
	vw := NewVisionWriter(dir)

	if drafts := vw.ReadDrafts(); len(drafts) != 0 {
		t.Error("reading non-existent drafts should return nil")
	}
	if final := vw.ReadFinal(); len(final) != 0 {
		t.Error("reading non-existent final should return nil")
	}
}
