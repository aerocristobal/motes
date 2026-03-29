package dream

import (
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

func makeImpactCtx(motes []*core.Mote) *impactContext {
	cfg := core.DefaultConfig()
	im := core.NewIndexManager("")
	idx := im.BuildInMemory(motes)
	scorer := core.NewScoreEngine(cfg.Scoring, idx.ConceptStats)
	return &impactContext{motes: motes, idx: idx, scorer: scorer, cfg: cfg}
}

func TestLinkImpact_ShowsEdgeBonus(t *testing.T) {
	ic := makeImpactCtx(nil)
	v := Vision{
		Type:        "link_suggestion",
		LinkType:    "relates_to",
		SourceMotes: []string{"motes-Abc"},
		TargetMotes: []string{"motes-Xyz"},
	}
	result := computeVisionImpact(v, ic)
	if result == "" {
		t.Fatal("expected non-empty impact for link_suggestion")
	}
	if !strings.Contains(result, "+0.10") {
		t.Errorf("expected +0.10 bonus for relates_to, got: %s", result)
	}
	if !strings.Contains(result, "motes-Abc") || !strings.Contains(result, "motes-Xyz") {
		t.Errorf("expected both mote IDs in impact, got: %s", result)
	}
}

func TestLinkImpact_BuildsOnBonus(t *testing.T) {
	ic := makeImpactCtx(nil)
	v := Vision{
		Type:        "link_suggestion",
		LinkType:    "builds_on",
		SourceMotes: []string{"motes-A"},
		TargetMotes: []string{"motes-B"},
	}
	result := computeVisionImpact(v, ic)
	if !strings.Contains(result, "+0.30") {
		t.Errorf("expected +0.30 bonus for builds_on, got: %s", result)
	}
}

func TestDeprecateImpact_ShowsNextInLine(t *testing.T) {
	now := time.Now()
	motes := []*core.Mote{
		{ID: "motes-High", Status: "active", Weight: 0.9, CreatedAt: now},
		{ID: "motes-Mid", Status: "active", Weight: 0.6, CreatedAt: now},
		{ID: "motes-Low", Status: "active", Weight: 0.3, CreatedAt: now},
	}
	ic := makeImpactCtx(motes)
	v := Vision{
		Type:        "staleness",
		Action:      "deprecate",
		SourceMotes: []string{"motes-Mid"},
	}
	result := computeVisionImpact(v, ic)
	if result == "" {
		t.Fatal("expected non-empty impact for staleness/deprecate")
	}
	if !strings.Contains(result, "motes-Mid") {
		t.Errorf("expected source mote in impact, got: %s", result)
	}
	// Next-in-line should be motes-Low (ranked below motes-Mid)
	if !strings.Contains(result, "motes-Low") {
		t.Errorf("expected next-in-line motes-Low in impact, got: %s", result)
	}
}

func TestDeprecateImpact_NoNextInLine(t *testing.T) {
	now := time.Now()
	motes := []*core.Mote{
		{ID: "motes-Only", Status: "active", Weight: 0.5, CreatedAt: now},
	}
	ic := makeImpactCtx(motes)
	v := Vision{
		Type:        "staleness",
		Action:      "deprecate",
		SourceMotes: []string{"motes-Only"},
	}
	result := computeVisionImpact(v, ic)
	if !strings.Contains(result, "No active mote") {
		t.Errorf("expected 'No active mote' message, got: %s", result)
	}
}

func TestConstellationImpact_ShowsMemberCount(t *testing.T) {
	ic := makeImpactCtx(nil)
	v := Vision{
		Type:        "constellation",
		Tags:        []string{"dependency-injection"},
		SourceMotes: []string{"motes-A", "motes-B", "motes-C"},
	}
	result := computeVisionImpact(v, ic)
	if result == "" {
		t.Fatal("expected non-empty impact for constellation")
	}
	if !strings.Contains(result, "3 motes") {
		t.Errorf("expected '3 motes' in impact, got: %s", result)
	}
	if !strings.Contains(result, "dependency-injection") {
		t.Errorf("expected theme in impact, got: %s", result)
	}
}

func TestComputeVisionImpact_OtherTypesReturnEmpty(t *testing.T) {
	ic := makeImpactCtx(nil)
	for _, vType := range []string{"tag_refinement", "compression", "merge_suggestion", "summarize", "action_extraction", "signal", "dominant_mote_review", "decay_risk"} {
		v := Vision{Type: vType, Action: "something", SourceMotes: []string{"motes-X"}}
		if result := computeVisionImpact(v, ic); result != "" {
			t.Errorf("expected empty impact for type %q, got: %s", vType, result)
		}
	}
}
