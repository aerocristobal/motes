// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"math"
	"testing"
)

func TestVisionKey_LinkSuggestion(t *testing.T) {
	v := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		SourceMotes: []string{"b", "a"},
		TargetMotes: []string{"d", "c"},
		LinkType:    "relates_to",
	}
	key := visionKey(v)
	expected := "link_suggestion|add_link|a,b|c,d|relates_to"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

func TestVisionKey_NonLink(t *testing.T) {
	v := Vision{
		Type:        "staleness",
		Action:      "deprecate",
		SourceMotes: []string{"c", "a", "b"},
	}
	key := visionKey(v)
	expected := "staleness|deprecate|a,b,c"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

func TestVoteVisions_Unanimous(t *testing.T) {
	v := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		SourceMotes: []string{"a"},
		TargetMotes: []string{"b"},
		LinkType:    "relates_to",
		Rationale:   "shared theme",
		Severity:    "medium",
	}
	candidates := [][]Vision{{v}, {v}, {v}}
	result := VoteVisions(candidates, 0.5)

	if len(result) != 1 {
		t.Fatalf("expected 1 vision, got %d", len(result))
	}
	if math.Abs(result[0].Agreement-1.0) > 0.001 {
		t.Errorf("expected agreement 1.0, got %.4f", result[0].Agreement)
	}
}

func TestVoteVisions_Majority(t *testing.T) {
	common := Vision{
		Type:        "link_suggestion",
		Action:      "add_link",
		SourceMotes: []string{"a"},
		TargetMotes: []string{"b"},
		LinkType:    "relates_to",
		Rationale:   "shared theme",
		Severity:    "medium",
	}
	outlier := Vision{
		Type:        "staleness",
		Action:      "deprecate",
		SourceMotes: []string{"x"},
		Rationale:   "old",
		Severity:    "low",
	}
	candidates := [][]Vision{
		{common, outlier},
		{common},
		{common},
	}
	result := VoteVisions(candidates, 0.5)

	// common appears in 3/3, outlier in 1/3 — only common survives
	if len(result) != 1 {
		t.Fatalf("expected 1 vision, got %d", len(result))
	}
	if result[0].Type != "link_suggestion" {
		t.Errorf("expected link_suggestion, got %s", result[0].Type)
	}
}

func TestVoteVisions_NoAgreement(t *testing.T) {
	v1 := Vision{Type: "staleness", Action: "deprecate", SourceMotes: []string{"a"}, Rationale: "old", Severity: "low"}
	v2 := Vision{Type: "staleness", Action: "deprecate", SourceMotes: []string{"b"}, Rationale: "old", Severity: "low"}
	v3 := Vision{Type: "staleness", Action: "deprecate", SourceMotes: []string{"c"}, Rationale: "old", Severity: "low"}
	candidates := [][]Vision{{v1}, {v2}, {v3}}
	result := VoteVisions(candidates, 0.5)

	if len(result) != 0 {
		t.Errorf("expected 0 visions when no agreement, got %d", len(result))
	}
}

func TestVoteVisions_SingleRun(t *testing.T) {
	v := Vision{Type: "staleness", Action: "deprecate", SourceMotes: []string{"a"}, Rationale: "old", Severity: "low"}
	result := VoteVisions([][]Vision{{v}}, 0.5)

	if len(result) != 1 {
		t.Fatalf("expected 1 vision for single run, got %d", len(result))
	}
	if math.Abs(result[0].Agreement-1.0) > 0.001 {
		t.Errorf("single run agreement should be 1.0, got %.4f", result[0].Agreement)
	}
}

func TestVoteVisions_MergesSeverity(t *testing.T) {
	v1 := Vision{Type: "link_suggestion", Action: "add_link", SourceMotes: []string{"a"}, TargetMotes: []string{"b"}, LinkType: "relates_to", Rationale: "short", Severity: "low"}
	v2 := Vision{Type: "link_suggestion", Action: "add_link", SourceMotes: []string{"a"}, TargetMotes: []string{"b"}, LinkType: "relates_to", Rationale: "a much longer rationale that explains things better", Severity: "high"}
	candidates := [][]Vision{{v1}, {v2}}
	result := VoteVisions(candidates, 0.5)

	if len(result) != 1 {
		t.Fatalf("expected 1 merged vision, got %d", len(result))
	}
	if result[0].Severity != "high" {
		t.Errorf("expected highest severity 'high', got %q", result[0].Severity)
	}
}

func TestVoteVisions_Empty(t *testing.T) {
	result := VoteVisions(nil, 0.5)
	if result != nil {
		t.Errorf("expected nil for empty candidates, got %v", result)
	}
}

func TestMergeAgreedVisions_UnionsMotes(t *testing.T) {
	group := []Vision{
		{Type: "link_suggestion", Action: "add_link", SourceMotes: []string{"a", "b"}, TargetMotes: []string{"c"}, Rationale: "reason1", Severity: "medium"},
		{Type: "link_suggestion", Action: "add_link", SourceMotes: []string{"b", "d"}, TargetMotes: []string{"c", "e"}, Rationale: "reason2", Severity: "medium"},
	}
	merged := mergeAgreedVisions(group, 3)

	if len(merged.SourceMotes) != 3 { // a, b, d
		t.Errorf("expected 3 source motes, got %d: %v", len(merged.SourceMotes), merged.SourceMotes)
	}
	if len(merged.TargetMotes) != 2 { // c, e
		t.Errorf("expected 2 target motes, got %d: %v", len(merged.TargetMotes), merged.TargetMotes)
	}
}
