package dream

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motes/internal/core"
)

func setupTestMotes(t *testing.T) (string, *core.MoteManager, *core.IndexManager) {
	t.Helper()
	root := t.TempDir()
	nodesDir := filepath.Join(root, "nodes")
	os.MkdirAll(nodesDir, 0755)

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	return root, mm, im
}

func createTestMote(t *testing.T, mm *core.MoteManager, moteType, title string, tags []string) *core.Mote {
	t.Helper()
	m, err := mm.Create(moteType, title, core.CreateOpts{Tags: tags})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPreScanner_EmptyNebula(t *testing.T) {
	root, mm, im := setupTestMotes(t)
	ps := NewPreScanner(root, mm, im, core.DefaultConfig().Dream)

	result, err := ps.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if result.HasWork() {
		t.Error("empty nebula should have no work")
	}
}

func TestPreScanner_StaleMotes(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create mote with old last_accessed
	m := createTestMote(t, mm, "context", "Old context", []string{"stale"})
	old := time.Now().Add(-200 * 24 * time.Hour)
	mm.Update(m.ID, map[string]interface{}{
		"last_accessed": old,
	})

	// Create recent mote
	recent := createTestMote(t, mm, "task", "Fresh task", []string{"active"})
	now := time.Now()
	mm.Update(recent.ID, map[string]interface{}{
		"last_accessed": now,
	})

	ps := NewPreScanner(root, mm, im, core.DefaultConfig().Dream)
	result, err := ps.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.StaleMotes) != 1 {
		t.Errorf("expected 1 stale mote, got %d", len(result.StaleMotes))
	}
	if len(result.StaleMotes) > 0 && result.StaleMotes[0] != m.ID {
		t.Errorf("expected stale mote %s, got %s", m.ID, result.StaleMotes[0])
	}
}

func TestPreScanner_OverloadedTags(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create 16 motes with the same tag
	for i := 0; i < 16; i++ {
		createTestMote(t, mm, "context", "Mote", []string{"overloaded-tag"})
	}

	// Rebuild index to populate tag stats
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	ps := NewPreScanner(root, mm, im, core.DefaultConfig().Dream)
	result, err := ps.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.OverloadedTags) != 1 {
		t.Errorf("expected 1 overloaded tag, got %d", len(result.OverloadedTags))
	}
}

func TestPreScanner_CompressionCandidates(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create a verbose mote
	longBody := ""
	for i := 0; i < 350; i++ {
		longBody += "word "
	}
	mm.Create("context", "Verbose mote", core.CreateOpts{Body: longBody})

	// Create a normal mote
	mm.Create("context", "Short mote", core.CreateOpts{Body: "Brief body."})

	ps := NewPreScanner(root, mm, im, core.DefaultConfig().Dream)
	result, err := ps.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.CompressionCandidates) != 1 {
		t.Errorf("expected 1 compression candidate, got %d", len(result.CompressionCandidates))
	}
}

func TestPreScanner_UncrystallizedIssues(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create a completed mote without crystallized counterpart
	m := createTestMote(t, mm, "task", "Done task", []string{"done"})
	mm.Update(m.ID, map[string]interface{}{"status": "completed"})

	ps := NewPreScanner(root, mm, im, core.DefaultConfig().Dream)
	result, err := ps.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.UncrystallizedIssues) != 1 {
		t.Errorf("expected 1 uncrystallized issue, got %d", len(result.UncrystallizedIssues))
	}
}

func TestPreScanner_LinkCandidates(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create two motes with 3+ shared tags but no link
	createTestMote(t, mm, "decision", "Choice A", []string{"auth", "oauth", "security", "api"})
	createTestMote(t, mm, "lesson", "Lesson B", []string{"auth", "oauth", "security", "tokens"})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	ps := NewPreScanner(root, mm, im, core.DefaultConfig().Dream)
	result, err := ps.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.LinkCandidates) != 1 {
		t.Errorf("expected 1 link candidate, got %d", len(result.LinkCandidates))
	}
}

func TestPreScanner_ConstellationEvolution(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create constellation mote and some tagged motes
	hub := createTestMote(t, mm, "constellation", "Constellation: auth", []string{"auth"})
	m1 := createTestMote(t, mm, "decision", "Auth decision", []string{"auth"})
	m2 := createTestMote(t, mm, "lesson", "Auth lesson", []string{"auth"})

	// Write constellations.jsonl recording only 2 members
	cPath := filepath.Join(root, "constellations.jsonl")
	record := fmt.Sprintf(`{"tag":"auth","constellation_mote_id":"%s","member_mote_ids":["%s","%s"]}`, hub.ID, m1.ID, m2.ID)
	os.WriteFile(cPath, []byte(record+"\n"), 0644)

	// Add 2 more motes with "auth" tag (total 5 including hub, 4 non-constellation)
	// Growth = (5 - 2) / 2 * 100 = 150% which exceeds 30% threshold
	createTestMote(t, mm, "context", "Auth context 1", []string{"auth"})
	createTestMote(t, mm, "context", "Auth context 2", []string{"auth"})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	ps := NewPreScanner(root, mm, im, core.DefaultConfig().Dream)
	result, err := ps.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.ConstellationEvolution) != 1 {
		t.Fatalf("expected 1 constellation evolution, got %d", len(result.ConstellationEvolution))
	}
	ce := result.ConstellationEvolution[0]
	if ce.ConstellationID != hub.ID {
		t.Errorf("expected constellation ID %s, got %s", hub.ID, ce.ConstellationID)
	}
	if ce.Tag != "auth" {
		t.Errorf("expected tag auth, got %s", ce.Tag)
	}
	if ce.OldCount != 2 {
		t.Errorf("expected old count 2, got %d", ce.OldCount)
	}
}

func TestPreScanner_HasWork(t *testing.T) {
	sr := &ScanResult{}
	if sr.HasWork() {
		t.Error("empty ScanResult should return false")
	}

	sr.StaleMotes = []string{"mote-1"}
	if !sr.HasWork() {
		t.Error("ScanResult with stale motes should return true")
	}
}
