// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLucidLog_Initialize(t *testing.T) {
	ll := NewLucidLog(2000)
	ll.ObservedPatterns = []Pattern{{PatternID: "p1"}}
	ll.Initialize()

	if len(ll.ObservedPatterns) != 0 {
		t.Error("Initialize should clear patterns")
	}
	if ll.Metadata.BatchCount != 0 {
		t.Error("Initialize should reset metadata")
	}
}

func TestLucidLog_Update_NewPattern(t *testing.T) {
	ll := NewLucidLog(2000)
	updates := LucidLogUpdates{
		ObservedPatterns: []Pattern{
			{PatternID: "p1", Description: "test", MoteIDs: []string{"m1"}, Strength: 1.0},
		},
	}
	ll.Update(updates)

	if len(ll.ObservedPatterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(ll.ObservedPatterns))
	}
	if ll.Metadata.BatchCount != 1 {
		t.Errorf("expected batch count 1, got %d", ll.Metadata.BatchCount)
	}
}

func TestLucidLog_Update_MergePattern(t *testing.T) {
	ll := NewLucidLog(2000)
	ll.ObservedPatterns = []Pattern{
		{PatternID: "p1", Description: "first", MoteIDs: []string{"m1"}, Strength: 1.0},
	}

	updates := LucidLogUpdates{
		ObservedPatterns: []Pattern{
			{PatternID: "p1", Description: "updated", MoteIDs: []string{"m2"}, Strength: 2.0},
		},
	}
	ll.Update(updates)

	if len(ll.ObservedPatterns) != 1 {
		t.Errorf("expected 1 merged pattern, got %d", len(ll.ObservedPatterns))
	}
	if ll.ObservedPatterns[0].Strength != 3.0 {
		t.Errorf("expected merged strength 3, got %g", ll.ObservedPatterns[0].Strength)
	}
	if len(ll.ObservedPatterns[0].MoteIDs) != 2 {
		t.Errorf("expected 2 mote IDs, got %d", len(ll.ObservedPatterns[0].MoteIDs))
	}
}

func TestLucidLog_RecordBatchFailure(t *testing.T) {
	ll := NewLucidLog(2000)
	ll.RecordBatchFailure(0, "timeout")

	if len(ll.Interrupts) != 1 {
		t.Errorf("expected 1 interrupt, got %d", len(ll.Interrupts))
	}
	if ll.Metadata.BatchCount != 1 {
		t.Error("expected batch count incremented")
	}
}

func TestLucidLog_Serialize(t *testing.T) {
	ll := NewLucidLog(2000)
	ll.ObservedPatterns = []Pattern{{PatternID: "p1", Description: "test"}}
	s := ll.Serialize()
	if s == "" {
		t.Error("serialize should return non-empty JSON")
	}
}

func TestLucidLog_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lucid_log.json")

	ll := NewLucidLog(2000)
	ll.ObservedPatterns = []Pattern{
		{PatternID: "p1", Description: "saved", MoteIDs: []string{"m1"}, Strength: 5.0},
	}
	if err := ll.Save(path); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("file should exist after save")
	}

	loaded := LoadLucidLog(path, 2000)
	if len(loaded.ObservedPatterns) != 1 {
		t.Errorf("expected 1 pattern after load, got %d", len(loaded.ObservedPatterns))
	}
	if loaded.ObservedPatterns[0].Strength != 5.0 {
		t.Errorf("expected strength 5, got %g", loaded.ObservedPatterns[0].Strength)
	}
}

func TestLucidLog_LoadMissing(t *testing.T) {
	ll := LoadLucidLog("/nonexistent/path", 2000)
	if ll == nil {
		t.Fatal("should return empty log for missing file")
	}
	if len(ll.ObservedPatterns) != 0 {
		t.Error("should have no patterns")
	}
}

func TestLucidLog_PruneIfOverLimit(t *testing.T) {
	ll := NewLucidLog(100) // small budget forces pruning
	for i := 0; i < 20; i++ {
		ll.Update(LucidLogUpdates{
			ObservedPatterns: []Pattern{{PatternID: "p", Description: "long description to inflate tokens significantly"}},
			VisionsSummary:   []VisionSummary{{Type: "link_suggestion", Batch: i}},
		})
	}
	// Pruning should have trimmed patterns to <= 3 and visions to <= 5
	if len(ll.ObservedPatterns) > 3 {
		t.Errorf("expected patterns pruned to <=3, got %d", len(ll.ObservedPatterns))
	}
	if len(ll.VisionsSummary) > 5 {
		t.Errorf("expected visions pruned to <=5, got %d", len(ll.VisionsSummary))
	}
}

func TestNewLucidLog_ZeroBudget(t *testing.T) {
	ll := NewLucidLog(0)
	if ll.maxTokens != 2000 {
		t.Errorf("expected default 2000, got %d", ll.maxTokens)
	}
}

func TestTension_UnmarshalJSON_String(t *testing.T) {
	var ten Tension
	if err := ten.UnmarshalJSON([]byte(`"some tension"`)); err != nil {
		t.Fatal(err)
	}
	if ten.Description != "some tension" {
		t.Errorf("expected description from string, got %q", ten.Description)
	}
}

func TestVisionSummary_UnmarshalJSON_String(t *testing.T) {
	var vs VisionSummary
	if err := vs.UnmarshalJSON([]byte(`"link_suggestion"`)); err != nil {
		t.Fatal(err)
	}
	if vs.Type != "link_suggestion" {
		t.Errorf("expected type from string, got %q", vs.Type)
	}
}

func TestTension_UnmarshalJSON_Object(t *testing.T) {
	var ten Tension
	if err := ten.UnmarshalJSON([]byte(`{"tension_id":"t1","description":"conflict","mote_ids":["a"]}`)); err != nil {
		t.Fatal(err)
	}
	if ten.TensionID != "t1" {
		t.Errorf("expected tension_id t1, got %q", ten.TensionID)
	}
}

func TestVisionSummary_UnmarshalJSON_Object(t *testing.T) {
	var vs VisionSummary
	if err := vs.UnmarshalJSON([]byte(`{"type":"staleness","mote_ids":["m1"],"batch":2}`)); err != nil {
		t.Fatal(err)
	}
	if vs.Batch != 2 {
		t.Errorf("expected batch 2, got %d", vs.Batch)
	}
}

func TestPatternMerge(t *testing.T) {
	p := Pattern{PatternID: "p1", MoteIDs: []string{"a", "b"}, Strength: 2.0}
	other := Pattern{PatternID: "p1", MoteIDs: []string{"b", "c"}, Strength: 3.0}
	p.Merge(other)

	if p.Strength != 5.0 {
		t.Errorf("expected strength 5, got %g", p.Strength)
	}
	if len(p.MoteIDs) != 3 {
		t.Errorf("expected 3 mote IDs (deduplicated), got %d", len(p.MoteIDs))
	}
}
