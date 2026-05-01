// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

func TestBuildBatchPrompt_RendersTemplate(t *testing.T) {
	m := &core.Mote{
		ID:    "test-m1",
		Type:  "decision",
		Title: "Test Decision",
		Tags:  []string{"auth", "api"},
		Body:  "Decision body content",
	}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)

	batch := Batch{
		Phase:          "clustered",
		PrimaryCluster: "auth",
		MoteIDs:        []string{"test-m1"},
		Tasks:          []string{"link_review"},
	}
	ll := NewLucidLog(2000)

	result := pb.BuildBatchPrompt(batch, ll, "")
	for _, want := range []string{"test-m1", "Test Decision", "auth, api", "clustered", "link_review"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected prompt to contain %q", want)
		}
	}
}

func TestBuildBatchPrompt_SkipsMissingMote(t *testing.T) {
	reader := func(id string) (*core.Mote, error) {
		if id == "good" {
			return &core.Mote{ID: "good", Title: "Good Mote", Tags: []string{"x"}}, nil
		}
		return nil, fmt.Errorf("not found")
	}
	pb := NewPromptBuilder(reader)

	batch := Batch{
		Phase:   "interleaved",
		MoteIDs: []string{"bad", "good"},
		Tasks:   []string{"staleness_review"},
	}
	ll := NewLucidLog(2000)

	result := pb.BuildBatchPrompt(batch, ll, "")
	if !strings.Contains(result, "Good Mote") {
		t.Error("expected prompt to contain the good mote")
	}
	if strings.Contains(result, "bad") {
		t.Error("bad mote ID should not appear in rendered motes")
	}
}

func TestBuildBatchPrompt_ContentLinkTask(t *testing.T) {
	m := &core.Mote{ID: "m1", Title: "Mote", Tags: []string{"t"}}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)

	batch := Batch{
		Phase:   "clustered",
		MoteIDs: []string{"m1"},
		Tasks:   []string{"content_link_review"},
	}
	ll := NewLucidLog(2000)

	result := pb.BuildBatchPrompt(batch, ll, "")
	if !strings.Contains(result, "Content Similarity") {
		t.Error("expected content similarity section for content_link_review task")
	}
}

func TestBuildReconciliationPrompt(t *testing.T) {
	ll := NewLucidLog(2000)
	ll.Update(LucidLogUpdates{
		ObservedPatterns: []Pattern{{PatternID: "p1", Description: "test pattern"}},
	})

	reader := func(id string) (*core.Mote, error) { return nil, fmt.Errorf("unused") }
	pb := NewPromptBuilder(reader)

	result := pb.BuildReconciliationPrompt(ll)
	if !strings.Contains(result, "reconciliation") {
		t.Error("expected reconciliation instruction in prompt")
	}
	if !strings.Contains(result, "test pattern") {
		t.Error("expected lucid log content in prompt")
	}
}

func TestBuildBatchPrompt_LensFallback(t *testing.T) {
	// An unimplemented lens (e.g. "inversion") falls back to the all-in-one template
	m := &core.Mote{ID: "m1", Title: "T", Tags: []string{"t"}}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)
	batch := Batch{Phase: "clustered", MoteIDs: []string{"m1"}, Tasks: []string{"link_review"}}
	ll := NewLucidLog(2000)
	result := pb.BuildBatchPrompt(batch, ll, "inversion")
	if result == "" {
		t.Error("expected non-empty prompt even for unimplemented lens (fallback to all-in-one)")
	}
}

func TestBuildBatchPrompt_StructuralLens(t *testing.T) {
	m := &core.Mote{ID: "m1", Title: "T", Tags: []string{"t"}}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)
	batch := Batch{Phase: "clustered", MoteIDs: []string{"m1"}, Tasks: []string{"link_review"}}
	ll := NewLucidLog(2000)
	result := pb.BuildBatchPrompt(batch, ll, "structural")

	for _, want := range []string{"Merge Detection", "Action Extraction", "Compression", "Contradiction Detection", "Tag Refinement"} {
		if !strings.Contains(result, want) {
			t.Errorf("structural lens: expected %q in prompt", want)
		}
	}
	for _, absent := range []string{"Survivorship Bias", "Feedback Loop", "Confirmation Bias"} {
		if strings.Contains(result, absent) {
			t.Errorf("structural lens: %q should not appear in prompt", absent)
		}
	}

	// Tag refinement section MUST instruct the model to populate the tags array,
	// otherwise the resulting vision fails apply (regression guard for v0.4.17).
	if !strings.Contains(result, "- tags:") {
		t.Error("structural lens: tag_refinement instructions must include `- tags:` field — visions without it cannot be applied")
	}
}

func TestBuildBatchPrompt_SurvivorshipBiasLens(t *testing.T) {
	m := &core.Mote{ID: "m1", Title: "T", Tags: []string{"t"}}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)
	batch := Batch{Phase: "clustered", MoteIDs: []string{"m1"}, Tasks: []string{"link_review"}}
	ll := NewLucidLog(2000)
	result := pb.BuildBatchPrompt(batch, ll, "survivorship_bias")

	if !strings.Contains(result, "Survivorship Bias") {
		t.Error("survivorship_bias lens: expected 'Survivorship Bias' in prompt")
	}
	if !strings.Contains(result, "survivorship_risk") {
		t.Error("survivorship_bias lens: expected 'survivorship_risk' link type in prompt")
	}
	for _, absent := range []string{"Merge Detection", "Feedback Loop", "Confirmation Bias"} {
		if strings.Contains(result, absent) {
			t.Errorf("survivorship_bias lens: %q should not appear in prompt", absent)
		}
	}
}

func TestBuildBatchPrompt_FeedbackLoopsLens(t *testing.T) {
	m := &core.Mote{ID: "m1", Title: "T", Tags: []string{"t"}}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)
	batch := Batch{Phase: "clustered", MoteIDs: []string{"m1"}, Tasks: []string{"link_review"}}
	ll := NewLucidLog(2000)
	result := pb.BuildBatchPrompt(batch, ll, "feedback_loops")

	for _, want := range []string{"Feedback Loop", "Reinforcing", "Balancing", "leverage point"} {
		if !strings.Contains(result, want) {
			t.Errorf("feedback_loops lens: expected %q in prompt", want)
		}
	}
	for _, absent := range []string{"Survivorship Bias", "Merge Detection", "Confirmation Bias"} {
		if strings.Contains(result, absent) {
			t.Errorf("feedback_loops lens: %q should not appear in prompt", absent)
		}
	}
}

func TestBuildBatchPrompt_ConfirmationBiasLens(t *testing.T) {
	m := &core.Mote{ID: "m1", Title: "T", Tags: []string{"t"}}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)
	batch := Batch{Phase: "clustered", MoteIDs: []string{"m1"}, Tasks: []string{"link_review"}}
	ll := NewLucidLog(2000)
	result := pb.BuildBatchPrompt(batch, ll, "confirmation_bias")

	for _, want := range []string{"Confirmation Bias", "one-sided", "contradiction"} {
		if !strings.Contains(result, want) {
			t.Errorf("confirmation_bias lens: expected %q in prompt", want)
		}
	}
	for _, absent := range []string{"Survivorship Bias", "Merge Detection", "Feedback Loop"} {
		if strings.Contains(result, absent) {
			t.Errorf("confirmation_bias lens: %q should not appear in prompt", absent)
		}
	}
}

func TestFormatTime_Nil(t *testing.T) {
	if got := formatTime(nil); got != "never" {
		t.Errorf("expected \"never\", got %q", got)
	}
}

func TestFormatTime_Valid(t *testing.T) {
	ts := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	if got := formatTime(&ts); got != "2025-03-15" {
		t.Errorf("expected \"2025-03-15\", got %q", got)
	}
}
