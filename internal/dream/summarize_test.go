// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"testing"
	"time"

	"motes/internal/core"
)

func TestFindSummarizationCandidates(t *testing.T) {
	now := time.Now().UTC()
	baseMote := func(id string, tags []string) *core.Mote {
		return &core.Mote{
			ID:        id,
			Type:      "lesson",
			Status:    "completed",
			Title:     "Mote " + id,
			Tags:      tags,
			Weight:    0.5,
			Origin:    "normal",
			CreatedAt: now,
		}
	}

	// 6 completed motes sharing "go" and "testing" tags
	motes := []*core.Mote{
		baseMote("m1", []string{"go", "testing", "unit"}),
		baseMote("m2", []string{"go", "testing", "integration"}),
		baseMote("m3", []string{"go", "testing", "mock"}),
		baseMote("m4", []string{"go", "testing", "bench"}),
		baseMote("m5", []string{"go", "testing", "coverage"}),
		baseMote("m6", []string{"go", "testing", "fuzz"}),
		// 2 active motes that shouldn't be included
		{ID: "m7", Type: "lesson", Status: "active", Title: "Active", Tags: []string{"go", "testing"}, Weight: 0.5, Origin: "normal", CreatedAt: now},
	}

	idx := &core.EdgeIndex{}
	ps := &PreScanner{config: core.DreamConfig{}}
	clusters := ps.findSummarizationCandidates(motes, idx)

	if len(clusters) == 0 {
		t.Fatal("expected at least 1 summarization cluster")
	}

	found := false
	for _, c := range clusters {
		if len(c.MoteIDs) >= 5 {
			found = true
			// Verify shared tags include go and testing
			hasGo, hasTesting := false, false
			for _, tag := range c.SharedTags {
				if tag == "go" {
					hasGo = true
				}
				if tag == "testing" {
					hasTesting = true
				}
			}
			if !hasGo || !hasTesting {
				t.Errorf("expected shared tags [go, testing], got %v", c.SharedTags)
			}
		}
	}
	if !found {
		t.Error("expected a cluster with 5+ members")
	}
}

func TestSummarizationExcludesAlreadySummarized(t *testing.T) {
	now := time.Now().UTC()
	baseMote := func(id string, tags []string) *core.Mote {
		return &core.Mote{
			ID:        id,
			Type:      "lesson",
			Status:    "completed",
			Title:     "Mote " + id,
			Tags:      tags,
			Weight:    0.5,
			Origin:    "normal",
			CreatedAt: now,
		}
	}

	motes := []*core.Mote{
		baseMote("m1", []string{"go", "testing"}),
		baseMote("m2", []string{"go", "testing"}),
		baseMote("m3", []string{"go", "testing"}),
		baseMote("m4", []string{"go", "testing"}),
		baseMote("m5", []string{"go", "testing"}),
		// Summary mote that already links to m1-m3
		{
			ID: "summary1", Type: "context", Status: "active",
			Title: "Summary of testing", Tags: []string{"go", "testing"},
			Weight: 0.6, Origin: "normal", CreatedAt: now,
			BuildsOn: []string{"m1", "m2", "m3"},
		},
	}

	idx := &core.EdgeIndex{}
	ps := &PreScanner{config: core.DreamConfig{}}
	clusters := ps.findSummarizationCandidates(motes, idx)

	// After excluding m1, m2, m3, only m4 and m5 remain (< 5), so no clusters
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters (already summarized), got %d", len(clusters))
	}
}
