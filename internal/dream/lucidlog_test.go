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
			{PatternID: "p1", Description: "test", MoteIDs: []string{"m1"}, Strength: 1},
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
		{PatternID: "p1", Description: "first", MoteIDs: []string{"m1"}, Strength: 1},
	}

	updates := LucidLogUpdates{
		ObservedPatterns: []Pattern{
			{PatternID: "p1", Description: "updated", MoteIDs: []string{"m2"}, Strength: 2},
		},
	}
	ll.Update(updates)

	if len(ll.ObservedPatterns) != 1 {
		t.Errorf("expected 1 merged pattern, got %d", len(ll.ObservedPatterns))
	}
	if ll.ObservedPatterns[0].Strength != 3 {
		t.Errorf("expected merged strength 3, got %d", ll.ObservedPatterns[0].Strength)
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
		{PatternID: "p1", Description: "saved", MoteIDs: []string{"m1"}, Strength: 5},
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
	if loaded.ObservedPatterns[0].Strength != 5 {
		t.Errorf("expected strength 5, got %d", loaded.ObservedPatterns[0].Strength)
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

func TestPatternMerge(t *testing.T) {
	p := Pattern{PatternID: "p1", MoteIDs: []string{"a", "b"}, Strength: 2}
	other := Pattern{PatternID: "p1", MoteIDs: []string{"b", "c"}, Strength: 3}
	p.Merge(other)

	if p.Strength != 5 {
		t.Errorf("expected strength 5, got %d", p.Strength)
	}
	if len(p.MoteIDs) != 3 {
		t.Errorf("expected 3 mote IDs (deduplicated), got %d", len(p.MoteIDs))
	}
}
