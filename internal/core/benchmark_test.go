// SPDX-License-Identifier: AGPL-3.0-or-later
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
	mm := NewMoteManager(root)
	mm.SetGlobalRoot(root)
	return root, mm
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

func BenchmarkMoteManager_Create(b *testing.B) {
	_, mm := setupBenchMemory(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Reset for each iteration to avoid ID collision across runs
		b.StartTimer()
		mm.Create("task", fmt.Sprintf("bench-task-%d", b.N+i), CreateOpts{
			Tags:   []string{"bench", "test"},
			Weight: 0.5,
		})
	}
}

func BenchmarkMoteManager_Update(b *testing.B) {
	_, mm := setupBenchMemory(b)
	m, _ := mm.Create("task", "bench-update-target", CreateOpts{
		Tags:   []string{"bench"},
		Weight: 0.5,
	})

	w := 0.7
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.Update(m.ID, UpdateOpts{Weight: &w})
	}
}

func BenchmarkMoteManager_List(b *testing.B) {
	for _, n := range []int{50, 100, 500} {
		n := n
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			b.StopTimer()
			_, mm := setupBenchMemory(b)
			tags := []string{"auth", "api", "db", "cache", "test"}
			for i := 0; i < n; i++ {
				mm.Create("task", fmt.Sprintf("task-%d", i), CreateOpts{
					Tags:   []string{tags[i%len(tags)]},
					Weight: 0.3 + float64(i%7)*0.1,
				})
			}

			b.ReportAllocs()
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				mm.List(ListFilters{Type: "task", Status: "active"})
			}
		})
	}
}

func BenchmarkMoteManager_ListReady(b *testing.B) {
	b.StopTimer()
	root, mm := setupBenchMemory(b)
	im := NewIndexManager(root)

	// Create 50 tasks with a chain of dependencies
	ids := make([]string, 50)
	for i := 0; i < 50; i++ {
		m, _ := mm.Create("task", fmt.Sprintf("ready-task-%d", i), CreateOpts{
			Tags:   []string{"bench"},
			Weight: 0.5,
		})
		ids[i] = m.ID
	}
	// Chain: task[i] depends_on task[i-1] for first 25
	for i := 1; i < 25; i++ {
		mm.Link(ids[i], "depends_on", ids[i-1], im)
	}
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	b.ReportAllocs()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		mm.List(ListFilters{Ready: true, Type: "task"})
	}
}

func BenchmarkIndexManager_Rebuild(b *testing.B) {
	for _, n := range []int{50, 200} {
		n := n
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			b.StopTimer()
			root, mm := setupBenchMemory(b)
			im := NewIndexManager(root)

			ids := make([]string, n)
			for i := 0; i < n; i++ {
				m, _ := mm.Create("task", fmt.Sprintf("idx-task-%d", i), CreateOpts{
					Tags: []string{"bench", fmt.Sprintf("t%d", i%10)},
				})
				ids[i] = m.ID
			}
			for i := 0; i < n; i++ {
				target := (i + 3) % n
				if target != i {
					mm.Link(ids[i], "relates_to", ids[target], im)
				}
			}
			motes, _ := mm.ReadAllParallel()

			b.ReportAllocs()
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				im.Rebuild(motes)
			}
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
