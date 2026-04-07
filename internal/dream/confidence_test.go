// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"math"
	"testing"
)

func TestScoreConfidence_ColdStart(t *testing.T) {
	v := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		Severity:    "medium",
		SourceMotes: []string{"a"},
		TargetMotes: []string{"b"},
		LinkType:    "relates_to",
		Rationale:   "These motes share a common theme and should be linked together for context",
	}
	score := ScoreConfidence(v, nil, nil)
	// Cold start: historical=0.5, structure should be high, severity=0.6, mote quality=0.5
	if score < 0.4 || score > 0.8 {
		t.Errorf("cold start well-formed vision: expected 0.4-0.8, got %.4f", score)
	}
}

func TestScoreConfidence_HighPerformingType(t *testing.T) {
	stats := map[string]*FeedbackStats{
		"link_suggestion": {
			Total:       10,
			Checked:     10,
			Persisted:   9,
			Reverted:    1,
			AvgDelta:    0.15,
			PositivePct: 80.0,
		},
	}
	v := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		Severity:    "high",
		SourceMotes: []string{"a"},
		TargetMotes: []string{"b"},
		LinkType:    "relates_to",
		Rationale:   "Strong thematic overlap between these motes warrants a direct link",
	}
	preScores := map[string]float64{"a": 1.2, "b": 1.0}
	score := ScoreConfidence(v, stats, preScores)
	if score < 0.7 {
		t.Errorf("high-performing type with high severity: expected >= 0.7, got %.4f", score)
	}
}

func TestScoreConfidence_LowPerformingType(t *testing.T) {
	stats := map[string]*FeedbackStats{
		"staleness": {
			Total:       10,
			Checked:     10,
			Persisted:   2,
			Reverted:    8,
			AvgDelta:    -0.2,
			PositivePct: 10.0,
		},
	}
	v := Vision{
		Type:        "staleness",
		Action:      "deprecate",
		Severity:    "low",
		SourceMotes: []string{"a"},
		Rationale:   "old",
	}
	score := ScoreConfidence(v, stats, nil)
	// With rebalanced weights (agreement=1.0 when voting disabled), score shifts slightly higher
	if score > 0.55 {
		t.Errorf("low-performing type with low severity: expected < 0.55, got %.4f", score)
	}
}

func TestScoreStructure_LinkSuggestion(t *testing.T) {
	complete := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		SourceMotes: []string{"a"},
		TargetMotes: []string{"b"},
		LinkType:    "relates_to",
		Rationale:   "These two motes discuss the same architectural pattern and should be cross-referenced",
	}
	incomplete := Vision{
		Type: "link_suggestion",
	}

	sc := scoreStructure(complete)
	si := scoreStructure(incomplete)

	if sc <= si {
		t.Errorf("complete (%.4f) should score higher than incomplete (%.4f)", sc, si)
	}
	if sc < 0.8 {
		t.Errorf("fully complete link_suggestion should score >= 0.8, got %.4f", sc)
	}
}

func TestScoreStructure_TagRefinement(t *testing.T) {
	v := Vision{
		Type:        "tag_refinement",
		Action:      "split_tag",
		SourceMotes: []string{"a"},
		Tags:        []string{"auth", "oauth2"},
		Rationale:   "The auth tag is overloaded; splitting into specific auth subtypes improves retrieval",
	}
	score := scoreStructure(v)
	if score < 0.8 {
		t.Errorf("complete tag_refinement should score >= 0.8, got %.4f", score)
	}
}

func TestScoreSeverity(t *testing.T) {
	tests := []struct {
		severity string
		expected float64
	}{
		{"high", 0.9},
		{"medium", 0.6},
		{"low", 0.3},
		{"", 0.5},
		{"unknown", 0.5},
	}
	for _, tt := range tests {
		got := scoreSeverity(tt.severity)
		if math.Abs(got-tt.expected) > 0.001 {
			t.Errorf("severity %q: expected %.1f, got %.4f", tt.severity, tt.expected, got)
		}
	}
}

func TestScoreMoteQuality(t *testing.T) {
	v := Vision{SourceMotes: []string{"a", "b"}}

	// High quality motes
	high := scoreMoteQuality(v, map[string]float64{"a": 1.5, "b": 1.8})
	// Low quality motes
	low := scoreMoteQuality(v, map[string]float64{"a": 0.2, "b": 0.3})
	// No scores
	neutral := scoreMoteQuality(v, nil)

	if high <= low {
		t.Errorf("high quality (%.4f) should exceed low quality (%.4f)", high, low)
	}
	if math.Abs(neutral-0.5) > 0.001 {
		t.Errorf("no scores should return 0.5, got %.4f", neutral)
	}
}

func TestScoreConfidence_ThresholdBoundary(t *testing.T) {
	// A well-formed high-severity vision with no history should pass default 0.6 threshold
	v := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		Severity:    "high",
		SourceMotes: []string{"a"},
		TargetMotes: []string{"b"},
		LinkType:    "relates_to",
		Rationale:   "Strong thematic overlap between these motes warrants a direct link for navigation",
	}
	score := ScoreConfidence(v, nil, map[string]float64{"a": 1.0, "b": 1.0})
	if score < 0.6 {
		t.Errorf("well-formed high-severity vision should pass 0.6 threshold, got %.4f", score)
	}
}

func TestScoreConfidence_LowSeverityIncomplete(t *testing.T) {
	// A low-severity incomplete vision should score below 0.6
	v := Vision{
		Type:     "staleness",
		Severity: "low",
		Rationale: "old",
	}
	score := ScoreConfidence(v, nil, nil)
	if score >= 0.6 {
		t.Errorf("low-severity incomplete vision should be below 0.6 threshold, got %.4f", score)
	}
}

func TestScoreConfidence_Clamped(t *testing.T) {
	// Score should always be in [0, 1]
	v := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		Severity:    "high",
		SourceMotes: []string{"a"},
		TargetMotes: []string{"b"},
		LinkType:    "relates_to",
		Rationale:   "Very strong evidence for linking these motes together based on shared context",
	}
	stats := map[string]*FeedbackStats{
		"link_suggestion": {
			Total: 100, Checked: 100, Persisted: 100,
			AvgDelta: 0.5, PositivePct: 100.0,
		},
	}
	score := ScoreConfidence(v, stats, map[string]float64{"a": 2.0, "b": 2.0})
	if score < 0.0 || score > 1.0 {
		t.Errorf("score should be clamped to [0, 1], got %.4f", score)
	}
}
