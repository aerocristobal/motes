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
	// A named lens with no template yet (ML-2 not wired) falls back to all-in-one
	m := &core.Mote{ID: "m1", Title: "T", Tags: []string{"t"}}
	reader := func(id string) (*core.Mote, error) { return m, nil }
	pb := NewPromptBuilder(reader)
	batch := Batch{Phase: "clustered", MoteIDs: []string{"m1"}, Tasks: []string{"link_review"}}
	ll := NewLucidLog(2000)
	result := pb.BuildBatchPrompt(batch, ll, "survivorship_bias")
	if result == "" {
		t.Error("expected non-empty prompt even for unimplemented lens (fallback to all-in-one)")
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
