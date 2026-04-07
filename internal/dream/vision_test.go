// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
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

func setupApplyTest(t *testing.T) (string, *core.MoteManager, *core.IndexManager, *VisionWriter) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)
	os.MkdirAll(filepath.Join(root, "dream"), 0755)
	mm := core.NewMoteManager(root)
	mm.SetGlobalRoot(root)
	im := core.NewIndexManager(root)
	vw := NewVisionWriter(filepath.Join(root, "dream"))
	return root, mm, im, vw
}

func TestApply_Contradiction(t *testing.T) {
	root, mm, im, vw := setupApplyTest(t)
	_ = root

	mA, _ := mm.Create("decision", "Choice A", core.CreateOpts{Tags: []string{"auth"}})
	mB, _ := mm.Create("decision", "Choice B", core.CreateOpts{Tags: []string{"auth"}})

	// Rebuild index
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "contradiction", SourceMotes: []string{mA.ID, mB.ID}}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply contradiction: %v", err)
	}

	// Verify link exists
	updated, _ := mm.Read(mA.ID)
	found := false
	for _, c := range updated.Contradicts {
		if c == mB.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s to have contradicts link to %s", mA.ID, mB.ID)
	}
}

func TestApply_TagRefinement(t *testing.T) {
	_, mm, im, vw := setupApplyTest(t)

	m, _ := mm.Create("context", "Tag test", core.CreateOpts{Tags: []string{"old-tag"}})

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "tag_refinement", SourceMotes: []string{m.ID}, Tags: []string{"new-tag", "refined"}}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply tag_refinement: %v", err)
	}

	updated, _ := mm.Read(m.ID)
	if len(updated.Tags) != 2 || updated.Tags[0] != "new-tag" {
		t.Errorf("expected tags [new-tag refined], got %v", updated.Tags)
	}
}

func TestApply_Compression(t *testing.T) {
	_, mm, im, vw := setupApplyTest(t)

	m, _ := mm.Create("context", "Verbose mote", core.CreateOpts{Body: "Very long body with lots of words."})

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "compression", SourceMotes: []string{m.ID}, Rationale: "Compressed summary."}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply compression: %v", err)
	}

	updated, _ := mm.Read(m.ID)
	if updated.Body != "Compressed summary." {
		t.Errorf("expected compressed body, got %q", updated.Body)
	}
}

func TestApply_Constellation(t *testing.T) {
	root, mm, im, vw := setupApplyTest(t)
	_ = root

	m1, _ := mm.Create("context", "Auth mote 1", core.CreateOpts{Tags: []string{"auth"}})
	m2, _ := mm.Create("context", "Auth mote 2", core.CreateOpts{Tags: []string{"auth"}})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "constellation", SourceMotes: []string{m1.ID, m2.ID}, Tags: []string{"auth"}}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply constellation: %v", err)
	}

	// Verify a constellation mote was created
	allMotes, _ := mm.ReadAllParallel()
	found := false
	for _, m := range allMotes {
		if m.Type == "constellation" && strings.Contains(m.Title, "auth") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a constellation mote to be created")
	}
}

func TestApply_LinkSuggestion_InsertsBodyRef(t *testing.T) {
	_, mm, im, vw := setupApplyTest(t)

	mA, _ := mm.Create("context", "Source mote", core.CreateOpts{Tags: []string{"test"}, Body: "Original body."})
	mB, _ := mm.Create("context", "Target mote", core.CreateOpts{Tags: []string{"test"}})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "link_suggestion", SourceMotes: []string{mA.ID}, TargetMotes: []string{mB.ID}, LinkType: "relates_to"}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply link_suggestion: %v", err)
	}

	updated, _ := mm.Read(mA.ID)
	ref := "[[" + mB.ID + "]]"
	if !strings.Contains(updated.Body, ref) {
		t.Errorf("expected body to contain %s, got %q", ref, updated.Body)
	}
}

func TestApply_LinkSuggestion_NoDuplicateBodyRef(t *testing.T) {
	_, mm, im, vw := setupApplyTest(t)

	mB, _ := mm.Create("context", "Target mote", core.CreateOpts{Tags: []string{"test"}})
	mA, _ := mm.Create("context", "Source mote", core.CreateOpts{Tags: []string{"test"}, Body: "Already has [[" + mB.ID + "]] link."})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "link_suggestion", SourceMotes: []string{mA.ID}, TargetMotes: []string{mB.ID}, LinkType: "relates_to"}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply link_suggestion: %v", err)
	}

	updated, _ := mm.Read(mA.ID)
	ref := "[[" + mB.ID + "]]"
	count := strings.Count(updated.Body, ref)
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of %s, got %d in %q", ref, count, updated.Body)
	}
}

func TestApply_LinkSuggestion_MissingLinkType(t *testing.T) {
	_, mm, im, vw := setupApplyTest(t)

	mA, _ := mm.Create("context", "Source", core.CreateOpts{Tags: []string{"test"}})

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "link_suggestion", SourceMotes: []string{mA.ID}, TargetMotes: []string{"target"}, LinkType: ""}

	if err := vr.apply(v); err == nil {
		t.Error("expected error for empty LinkType")
	}
}

func TestApply_Staleness_Deprecate(t *testing.T) {
	_, mm, im, vw := setupApplyTest(t)

	m, _ := mm.Create("context", "Old mote", core.CreateOpts{Tags: []string{"stale"}})

	vr := NewVisionReviewer(vw, mm, im)
	v := Vision{Type: "staleness", Action: "deprecate", SourceMotes: []string{m.ID}}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply staleness deprecate: %v", err)
	}

	updated, _ := mm.Read(m.ID)
	if updated.Status != "deprecated" {
		t.Errorf("expected status 'deprecated', got %q", updated.Status)
	}
}

func TestApply_Signal(t *testing.T) {
	root, mm, im, vw := setupApplyTest(t)

	cfg := core.DefaultConfig()
	initialSignals := len(cfg.Priming.Signals)

	vr := NewVisionReviewerWithConfig(vw, mm, im, root, cfg)
	v := Vision{
		Type:   "signal",
		Action: "test_signal",
		Tags:   []string{"testing"},
		Rationale: "Test signal pattern",
	}

	if err := vr.apply(v); err != nil {
		t.Fatalf("apply signal: %v", err)
	}

	if len(cfg.Priming.Signals) != initialSignals+1 {
		t.Errorf("expected %d signals, got %d", initialSignals+1, len(cfg.Priming.Signals))
	}

	// Verify config file was written
	loaded, err := core.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Priming.Signals) < initialSignals+1 {
		t.Error("saved config should have the new signal")
	}
}
