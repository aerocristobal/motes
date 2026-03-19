package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkScoreEngine_Score(b *testing.B) {
	se := defaultScoreEngine()

	accessed := time.Now().Add(-20 * 24 * time.Hour)
	m := &Mote{
		Weight:      0.7,
		Status:      "active",
		Origin:      "failure",
		Type:        "task",
		LastAccessed: &accessed,
		AccessCount: 3,
		Tags:        []string{"oauth", "api"},
	}
	ctx := ScoringContext{
		MatchedTags:          []string{"oauth"},
		EdgeType:             "relates_to",
		ActiveContradictions: 1,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		se.Score(m, ctx)
	}
}

func setupBenchMemory(b *testing.B) (string, *MoteManager) {
	b.Helper()
	dir := b.TempDir()
	root := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(filepath.Join(root, "nodes"), 0755); err != nil {
		b.Fatal(err)
	}
	return root, NewMoteManager(root)
}

func BenchmarkGraphTraverser_Traverse(b *testing.B) {
	b.StopTimer()

	root, mm := setupBenchMemory(b)
	im := NewIndexManager(root)
	recent := time.Now().Add(-1 * time.Hour)

	// Create 100 motes with varied weights
	tags := []string{"auth", "api", "security", "oauth", "database"}
	ids := make([]string, 100)
	for i := 0; i < 100; i++ {
		tag := tags[i%len(tags)]
		w := 0.3 + float64(i%7)*0.1
		m, _ := mm.Create("task", fmt.Sprintf("Mote-%d", i), CreateOpts{
			Weight: w,
			Tags:   []string{tag, tags[(i+1)%len(tags)]},
		})
		mm.Update(m.ID, UpdateOpts{LastAccessed: &recent})
		ids[i] = m.ID
	}

	// Create ~400 edges (4 per mote)
	for i := 0; i < 100; i++ {
		for j := 1; j <= 4; j++ {
			target := (i + j*7) % 100
			if target == i {
				target = (i + 1) % 100
			}
			edgeType := "relates_to"
			if j == 1 {
				edgeType = "builds_on"
			}
			mm.Link(ids[i], edgeType, ids[target], im)
		}
	}

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se := NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	// Pick 3 seeds
	seeds := make([]*Mote, 3)
	for i := 0; i < 3; i++ {
		seeds[i], _ = mm.Read(ids[i])
	}

	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)

	b.ReportAllocs()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		gt.Traverse(seeds, []string{"auth", "security"}, func(id string) (*Mote, error) {
			return mm.Read(id)
		})
	}
}

func BenchmarkSeedSelector_SelectSeeds(b *testing.B) {
	tagPool := []string{
		"auth", "api", "security", "oauth", "database",
		"logging", "testing", "docker", "ci", "cache",
		"metrics", "search", "scoring", "graph", "config",
		"strata", "dream", "seed", "traversal", "index",
	}

	motes := make([]*Mote, 200)
	for i := 0; i < 200; i++ {
		nTags := 2 + (i % 2) // 2 or 3 tags
		tags := make([]string, nTags)
		for j := 0; j < nTags; j++ {
			tags[j] = tagPool[(i+j*3)%len(tagPool)]
		}
		motes[i] = &Mote{
			ID:     fmt.Sprintf("m%d", i),
			Title:  fmt.Sprintf("Mote %d", i),
			Tags:   tags,
			Weight: 0.3 + float64(i%7)*0.1,
		}
	}

	tagStats := make(map[string]int)
	for _, m := range motes {
		for _, t := range m.Tags {
			tagStats[t]++
		}
	}

	ss := NewSeedSelector(motes, tagStats, nil, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ss.SelectSeeds("auth security oauth", nil)
	}
}
