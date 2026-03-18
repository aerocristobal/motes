package core

import (
	"math"
	"testing"
	"time"
)

func defaultScoreEngine() *ScoreEngine {
	cfg := DefaultConfig()
	tagStats := map[string]int{"oauth": 5, "api": 20, "rare": 1}
	return NewScoreEngine(cfg.Scoring, tagStats)
}

func TestScore_BaseWeight(t *testing.T) {
	se := defaultScoreEngine()

	recent := time.Now().Add(-1 * time.Hour)
	m := &Mote{Weight: 0.8, Origin: "normal", LastAccessed: &recent}
	score := se.Score(m, ScoringContext{})

	// base=0.8, no edge bonus, no penalty, recency=1.0, salience(normal)=0.0
	// raw=0.8, final=0.8×1.0=0.8
	if math.Abs(score-0.8) > 0.01 {
		t.Errorf("expected ~0.8, got %f", score)
	}
}

func TestScore_EdgeBonus(t *testing.T) {
	se := defaultScoreEngine()

	recent := time.Now().Add(-1 * time.Hour)
	m := &Mote{Weight: 0.5, Origin: "normal", LastAccessed: &recent}

	buildsOn := se.Score(m, ScoringContext{EdgeType: "builds_on"})
	relatesTo := se.Score(m, ScoringContext{EdgeType: "relates_to"})
	noEdge := se.Score(m, ScoringContext{EdgeType: "seed"})

	// builds_on: +0.3, relates_to: +0.1, seed: +0.0
	if buildsOn <= relatesTo {
		t.Errorf("builds_on (%f) should score higher than relates_to (%f)", buildsOn, relatesTo)
	}
	if relatesTo <= noEdge {
		t.Errorf("relates_to (%f) should score higher than seed (%f)", relatesTo, noEdge)
	}
}

func TestScore_StatusPenalty(t *testing.T) {
	se := defaultScoreEngine()

	recent := time.Now().Add(-1 * time.Hour)
	active := &Mote{Weight: 0.5, Status: "active", Origin: "normal", LastAccessed: &recent}
	deprecated := &Mote{Weight: 0.5, Status: "deprecated", Origin: "normal", LastAccessed: &recent}

	activeScore := se.Score(active, ScoringContext{})
	deprecatedScore := se.Score(deprecated, ScoringContext{})

	// Deprecated gets -0.5 penalty
	diff := activeScore - deprecatedScore
	if math.Abs(diff-0.5) > 0.01 {
		t.Errorf("expected ~0.5 difference, got %f (active=%f, deprecated=%f)",
			diff, activeScore, deprecatedScore)
	}
}

func TestScore_RecencyDecay(t *testing.T) {
	se := defaultScoreEngine()

	m := &Mote{Weight: 1.0, Origin: "normal"}

	// <7 days → ×1.0
	recent := time.Now().Add(-3 * 24 * time.Hour)
	m.LastAccessed = &recent
	score7 := se.Score(m, ScoringContext{})

	// <30 days → ×0.85
	month := time.Now().Add(-15 * 24 * time.Hour)
	m.LastAccessed = &month
	score30 := se.Score(m, ScoringContext{})

	// <90 days → ×0.65
	quarter := time.Now().Add(-60 * 24 * time.Hour)
	m.LastAccessed = &quarter
	score90 := se.Score(m, ScoringContext{})

	// nil → ×0.4
	m.LastAccessed = nil
	scoreNil := se.Score(m, ScoringContext{})

	if score7 <= score30 || score30 <= score90 || score90 <= scoreNil {
		t.Errorf("recency should decay: 7d=%f, 30d=%f, 90d=%f, nil=%f",
			score7, score30, score90, scoreNil)
	}

	// Verify exact factors: weight=1.0, so score ≈ factor
	if math.Abs(score7-1.0) > 0.01 {
		t.Errorf("7d score should be ~1.0, got %f", score7)
	}
	if math.Abs(score30-0.85) > 0.01 {
		t.Errorf("30d score should be ~0.85, got %f", score30)
	}
}

func TestScore_RetrievalStrength(t *testing.T) {
	se := defaultScoreEngine()

	recent := time.Now().Add(-1 * time.Hour)
	m := &Mote{Weight: 0.5, Origin: "normal", LastAccessed: &recent, AccessCount: 5}

	score := se.Score(m, ScoringContext{})
	// base=0.5 + retrieval=min(5×0.03, 0.15)=0.15, recency=1.0
	// raw=0.65, final=0.65
	if math.Abs(score-0.65) > 0.01 {
		t.Errorf("expected ~0.65, got %f", score)
	}

	// Test cap: access_count=10 → min(10×0.03, 0.15) = 0.15 (capped)
	m.AccessCount = 10
	scoreCapped := se.Score(m, ScoringContext{})
	if math.Abs(scoreCapped-0.65) > 0.01 {
		t.Errorf("capped should also be ~0.65, got %f", scoreCapped)
	}
}

func TestScore_Salience(t *testing.T) {
	se := defaultScoreEngine()

	recent := time.Now().Add(-1 * time.Hour)
	failure := &Mote{Weight: 0.5, Origin: "failure", LastAccessed: &recent}
	normal := &Mote{Weight: 0.5, Origin: "normal", LastAccessed: &recent}
	explore := &Mote{Weight: 0.5, Origin: "normal", Type: "explore", LastAccessed: &recent}

	failScore := se.Score(failure, ScoringContext{})
	normalScore := se.Score(normal, ScoringContext{})
	exploreScore := se.Score(explore, ScoringContext{})

	// failure: +0.2, explore type: +0.1
	if failScore <= normalScore {
		t.Errorf("failure (%f) should score higher than normal (%f)", failScore, normalScore)
	}
	if exploreScore <= normalScore {
		t.Errorf("explore (%f) should score higher than normal (%f)", exploreScore, normalScore)
	}
}

func TestScore_TagSpecificity(t *testing.T) {
	se := defaultScoreEngine()

	recent := time.Now().Add(-1 * time.Hour)
	m := &Mote{Weight: 0.5, Origin: "normal", LastAccessed: &recent}

	// rare tag (count=1) should give higher specificity than common tag (count=20)
	rareScore := se.Score(m, ScoringContext{MatchedTags: []string{"rare"}})
	commonScore := se.Score(m, ScoringContext{MatchedTags: []string{"api"}})
	noTagScore := se.Score(m, ScoringContext{})

	if rareScore <= commonScore {
		t.Errorf("rare tag (%f) should score higher than common tag (%f)", rareScore, commonScore)
	}
	if commonScore <= noTagScore {
		t.Errorf("any tag match (%f) should score higher than none (%f)", commonScore, noTagScore)
	}
}

func TestScore_InterferencePenalty(t *testing.T) {
	se := defaultScoreEngine()

	recent := time.Now().Add(-1 * time.Hour)
	m := &Mote{Weight: 0.5, Origin: "normal", LastAccessed: &recent}

	noContradict := se.Score(m, ScoringContext{ActiveContradictions: 0})
	oneContradict := se.Score(m, ScoringContext{ActiveContradictions: 1})
	twoContradict := se.Score(m, ScoringContext{ActiveContradictions: 2})

	// Each contradiction: -0.1
	if math.Abs((noContradict-oneContradict)-0.1) > 0.01 {
		t.Errorf("one contradiction should reduce by 0.1: no=%f, one=%f", noContradict, oneContradict)
	}
	if math.Abs((noContradict-twoContradict)-0.2) > 0.01 {
		t.Errorf("two contradictions should reduce by 0.2: no=%f, two=%f", noContradict, twoContradict)
	}
}

func TestScore_FullFormula(t *testing.T) {
	se := defaultScoreEngine()

	accessed := time.Now().Add(-20 * 24 * time.Hour) // <30d → ×0.85
	m := &Mote{
		Weight:      0.7,
		Status:      "active",
		Origin:      "failure",
		Type:        "task",
		LastAccessed: &accessed,
		AccessCount: 3,
	}

	ctx := ScoringContext{
		MatchedTags:          []string{"oauth"},
		EdgeType:             "relates_to",
		ActiveContradictions: 1,
	}

	score := se.Score(m, ctx)

	// Manual calculation:
	// base = 0.7
	// edge_bonus = 0.1 (relates_to)
	// status_penalty = 0.0 (active)
	// retrieval = min(3×0.03, 0.15) = 0.09
	// salience = 0.2 (failure)
	// tag_spec = (1/log2(5+2)) × 0.2 = (1/log2(7)) × 0.2 ≈ (1/2.807) × 0.2 ≈ 0.0713
	// interference = 1 × -0.1 = -0.1
	// raw = 0.7 + 0.1 + 0.0 + 0.09 + 0.2 + 0.0713 - 0.1 = 1.0613
	// recency = 0.85
	// final = 1.0613 × 0.85 ≈ 0.9021
	expected := 0.9021
	if math.Abs(score-expected) > 0.02 {
		t.Errorf("expected ~%f, got %f", expected, score)
	}
}

func TestScore_BodyRefEdgeBonus(t *testing.T) {
	se := defaultScoreEngine()
	recent := time.Now().Add(-1 * time.Hour)
	m := &Mote{Weight: 0.5, Origin: "normal", LastAccessed: &recent}

	bodyRef := se.Score(m, ScoringContext{EdgeType: "body_ref"})
	builtByRef := se.Score(m, ScoringContext{EdgeType: "built_by_ref"})
	noEdge := se.Score(m, ScoringContext{EdgeType: "seed"})

	if bodyRef <= noEdge {
		t.Errorf("body_ref (%f) should score higher than seed (%f)", bodyRef, noEdge)
	}
	if builtByRef <= noEdge {
		t.Errorf("built_by_ref (%f) should score higher than seed (%f)", builtByRef, noEdge)
	}
}
