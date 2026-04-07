// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"testing"
)

func TestExtractJSON_Simple(t *testing.T) {
	input := `Here is the result: {"visions": [], "lucid_log_updates": {}}`
	got := extractJSON(input)
	if got != `{"visions": [], "lucid_log_updates": {}}` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestExtractJSON_NestedBraces(t *testing.T) {
	input := `Some text {"a": {"b": "c"}} more text`
	got := extractJSON(input)
	if got != `{"a": {"b": "c"}}` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestExtractJSON_StringWithBraces(t *testing.T) {
	input := `{"key": "value with {braces}"}`
	got := extractJSON(input)
	if got != `{"key": "value with {braces}"}` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	input := "No JSON here at all"
	got := extractJSON(input)
	if got != "" {
		t.Errorf("expected empty, got: %s", got)
	}
}

func TestExtractJSON_EscapedQuotes(t *testing.T) {
	input := `{"key": "value with \"escaped\" quotes"}`
	got := extractJSON(input)
	if got != `{"key": "value with \"escaped\" quotes"}` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestParseBatchResponse_Valid(t *testing.T) {
	input := `Here are my findings:
{"visions": [{"type": "link_suggestion", "action": "add_link", "source_motes": ["m1"], "target_motes": ["m2"], "link_type": "relates_to", "rationale": "shared concept", "severity": "medium"}], "lucid_log_updates": {"observed_patterns": [{"pattern_id": "p1", "description": "test", "mote_ids": ["m1"], "strength": 1}]}}`

	parser := NewResponseParser()
	visions, updates, err := parser.ParseBatchResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(visions) != 1 {
		t.Errorf("expected 1 vision, got %d", len(visions))
	}
	if visions[0].Type != "link_suggestion" {
		t.Errorf("expected link_suggestion, got %s", visions[0].Type)
	}
	if len(updates.ObservedPatterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(updates.ObservedPatterns))
	}
}

func TestParseBatchResponse_NoJSON(t *testing.T) {
	parser := NewResponseParser()
	_, _, err := parser.ParseBatchResponse("No JSON here")
	if err == nil {
		t.Error("expected error for no JSON")
	}
}

func TestParseBatchResponse_FiltersInvalid(t *testing.T) {
	input := `{"visions": [{"type": "", "rationale": "incomplete"}, {"type": "staleness", "rationale": "old content", "source_motes": ["m1"], "severity": "low"}], "lucid_log_updates": {}}`

	parser := NewResponseParser()
	visions, _, err := parser.ParseBatchResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(visions) != 1 {
		t.Errorf("expected 1 valid vision, got %d", len(visions))
	}
}

func TestParseReconciliationResponse_Valid(t *testing.T) {
	input := `{"visions": [{"type": "contradiction", "action": "flag", "source_motes": ["a", "b"], "rationale": "conflicting decisions", "severity": "high"}]}`

	parser := NewResponseParser()
	visions, err := parser.ParseReconciliationResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(visions) != 1 {
		t.Errorf("expected 1 vision, got %d", len(visions))
	}
}

func TestParseBatchResponse_FloatStrength(t *testing.T) {
	input := `{"visions": [], "lucid_log_updates": {"observed_patterns": [{"pattern_id": "p1", "description": "test", "mote_ids": ["m1"], "strength": 0.8}]}}`
	parser := NewResponseParser()
	_, updates, err := parser.ParseBatchResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates.ObservedPatterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(updates.ObservedPatterns))
	}
	if updates.ObservedPatterns[0].Strength != 0.8 {
		t.Errorf("expected strength 0.8, got %g", updates.ObservedPatterns[0].Strength)
	}
}

func TestParseBatchResponse_StringPattern(t *testing.T) {
	input := `{"visions": [], "lucid_log_updates": {"observed_patterns": "motes share a common theme"}}`
	parser := NewResponseParser()
	_, updates, err := parser.ParseBatchResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates.ObservedPatterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(updates.ObservedPatterns))
	}
	if updates.ObservedPatterns[0].Description != "motes share a common theme" {
		t.Errorf("unexpected description: %s", updates.ObservedPatterns[0].Description)
	}
}

func TestParseBatchResponse_MixedPatterns(t *testing.T) {
	input := `{"visions": [], "lucid_log_updates": {"observed_patterns": [{"pattern_id": "p1", "description": "first", "mote_ids": ["m1"], "strength": 1.5}, "bare string pattern"]}}`
	parser := NewResponseParser()
	_, updates, err := parser.ParseBatchResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates.ObservedPatterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(updates.ObservedPatterns))
	}
	if updates.ObservedPatterns[0].Strength != 1.5 {
		t.Errorf("expected strength 1.5, got %g", updates.ObservedPatterns[0].Strength)
	}
	if updates.ObservedPatterns[1].Description != "bare string pattern" {
		t.Errorf("unexpected description: %s", updates.ObservedPatterns[1].Description)
	}
}

func TestExtractJSON_FenceVariants(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"JSON uppercase", "```JSON\n{\"a\":1}\n```", `{"a":1}`},
		{"jsonc", "```jsonc\n{\"a\":1}\n```", `{"a":1}`},
		{"leading whitespace", "  \n```json\n{\"a\":1}\n```", `{"a":1}`},
		{"no lang marker", "```\n{\"a\":1}\n```", `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractJSON_TopLevelArray(t *testing.T) {
	input := `[{"visions":[]}]`
	got := extractJSON(input)
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestExtractJSON_ProseBeforeJSON(t *testing.T) {
	input := "Here is the result:\n\n{\"visions\": []}"
	got := extractJSON(input)
	if got != `{"visions": []}` {
		t.Errorf("got %q", got)
	}
}

func TestFilterValidVisions_DefaultSeverity(t *testing.T) {
	visions := []Vision{
		{Type: "staleness", Rationale: "old", SourceMotes: []string{"m1"}},
	}
	valid := filterValidVisions(visions)
	if len(valid) != 1 {
		t.Fatal("expected 1 valid vision")
	}
	if valid[0].Severity != "medium" {
		t.Errorf("expected default severity 'medium', got %s", valid[0].Severity)
	}
}
