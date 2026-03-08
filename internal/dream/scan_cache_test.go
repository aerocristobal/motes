package dream

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"motes/internal/core"
)

func TestScanCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	os.MkdirAll(filepath.Join(root, "dream"), 0755)

	// First load should return empty cache
	cache := LoadScanCache(root)
	if len(cache.Hashes) != 0 {
		t.Fatalf("expected empty cache, got %d entries", len(cache.Hashes))
	}

	// Add entries
	cache.Hashes["mote-1"] = "abc123"
	cache.Hashes["mote-2"] = "def456"

	// Save
	if err := SaveScanCache(root, cache); err != nil {
		t.Fatal(err)
	}

	// Reload
	cache2 := LoadScanCache(root)
	if len(cache2.Hashes) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cache2.Hashes))
	}
	if cache2.Hashes["mote-1"] != "abc123" {
		t.Errorf("expected abc123, got %s", cache2.Hashes["mote-1"])
	}
}

func TestComputeMoteHashDeterministic(t *testing.T) {
	m := &core.Mote{
		ID:        "test-123",
		Type:      "lesson",
		Status:    "active",
		Title:     "Test Mote",
		Tags:      []string{"a", "b"},
		Weight:    0.5,
		Origin:    "normal",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Body:      "Hello world",
	}

	h1 := ComputeMoteHash(m)
	h2 := ComputeMoteHash(m)

	if h1 != h2 {
		t.Errorf("hash not deterministic: %s != %s", h1, h2)
	}

	// Modify body -> hash should change
	m.Body = "Changed body"
	h3 := ComputeMoteHash(m)
	if h1 == h3 {
		t.Error("hash should change when body changes")
	}
}

func TestFilterChanged(t *testing.T) {
	now := time.Now().UTC()
	motes := []*core.Mote{
		{ID: "m1", Type: "lesson", Status: "active", Title: "One", Weight: 0.5, Origin: "normal", CreatedAt: now, Body: "body1"},
		{ID: "m2", Type: "lesson", Status: "active", Title: "Two", Weight: 0.5, Origin: "normal", CreatedAt: now, Body: "body2"},
		{ID: "m3", Type: "lesson", Status: "active", Title: "Three", Weight: 0.5, Origin: "normal", CreatedAt: now, Body: "body3"},
	}

	// First run: all should be changed
	cache := &ScanCache{Hashes: map[string]string{}}
	changed := FilterChanged(motes, cache)
	if len(changed) != 3 {
		t.Fatalf("first run: expected 3 changed, got %d", len(changed))
	}
	if len(cache.Hashes) != 3 {
		t.Fatalf("cache should have 3 entries, got %d", len(cache.Hashes))
	}

	// Second run: nothing changed
	changed = FilterChanged(motes, cache)
	if len(changed) != 0 {
		t.Fatalf("second run: expected 0 changed, got %d", len(changed))
	}

	// Modify one mote
	motes[1].Body = "modified body"
	changed = FilterChanged(motes, cache)
	if len(changed) != 1 {
		t.Fatalf("after modify: expected 1 changed, got %d", len(changed))
	}
	if changed[0].ID != "m2" {
		t.Errorf("expected m2 changed, got %s", changed[0].ID)
	}

	// Remove a mote -> should prune from cache
	motes = motes[:2]
	_ = FilterChanged(motes, cache)
	if _, ok := cache.Hashes["m3"]; ok {
		t.Error("deleted mote m3 should be pruned from cache")
	}
}
