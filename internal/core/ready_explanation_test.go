// SPDX-License-Identifier: MIT
//
// STORY-EXPLAIN-001 — unit tests for BuildReadyExplanation. Pure function,
// no I/O, no clock — all timestamps are explicit. Mirrors the test scaffold
// in the story's §6.
package core

import (
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

// --- HAPPY PATH (Scenario 1) ---

func TestBuildReadyExplanation_NoBlockers_ReturnsCleanReason(t *testing.T) {
	parent := &Mote{ID: "epic-foo", Status: "in_progress"}
	accessed := fixedNow.Add(-2 * 24 * time.Hour)
	m := &Mote{
		ID:           "node-mote-a",
		Status:       "active",
		Type:         "task",
		LastAccessed: &accessed,
		Parent:       "epic-foo",
	}
	exp := BuildReadyExplanation(m, []*Mote{m, parent}, fixedNow)

	if exp.Reason != "no blockers" {
		t.Fatalf("want 'no blockers', got %q", exp.Reason)
	}
	if exp.Parent == nil || exp.Parent.ID != "epic-foo" {
		t.Fatalf("parent block missing or wrong id: %+v", exp.Parent)
	}
	if exp.Parent.Status != "in_progress" {
		t.Fatalf("want parent status in_progress, got %q", exp.Parent.Status)
	}
	if exp.Parent.IsClosed {
		t.Fatalf("in_progress parent must not be IsClosed=true")
	}
	if exp.Freshness == nil {
		t.Fatalf("freshness must always be populated")
	}
	if exp.Freshness.Stale {
		t.Fatalf("2-day-fresh mote should not be marked stale")
	}
	if exp.Freshness.NeverAccessed {
		t.Fatalf("mote with LastAccessed set should not be NeverAccessed")
	}
}

// --- BLOCKER ENUMERATION (Scenario 2) ---

func TestBuildReadyExplanation_WithClearedBlockers_ListsThemWithTimes(t *testing.T) {
	cleared1 := fixedNow.Add(-3 * 24 * time.Hour)
	cleared2 := fixedNow.Add(-21 * 24 * time.Hour)
	blk1 := &Mote{ID: "node-blk-1", Status: "completed", StatusChangedAt: &cleared1}
	blk2 := &Mote{ID: "node-blk-2", Status: "completed", StatusChangedAt: &cleared2}
	m := &Mote{
		ID:        "node-mote-x",
		Status:    "active",
		DependsOn: []string{"node-blk-1", "node-blk-2"},
	}

	exp := BuildReadyExplanation(m, []*Mote{m, blk1, blk2}, fixedNow)

	if len(exp.ClearedBlockers) != 2 {
		t.Fatalf("want 2 cleared blockers, got %d", len(exp.ClearedBlockers))
	}
	if exp.ClearedBlockers[0].ID != "node-blk-1" {
		t.Fatalf("blocker order: want node-blk-1 first, got %q", exp.ClearedBlockers[0].ID)
	}
	if exp.ClearedBlockers[0].ClearedAt == nil || !exp.ClearedBlockers[0].ClearedAt.Equal(cleared1) {
		t.Fatalf("blocker ClearedAt mismatch: %+v", exp.ClearedBlockers[0].ClearedAt)
	}
	want := "2 of 2 blocking deps closed (node-blk-1 3d ago, node-blk-2 21d ago)"
	if exp.Reason != want {
		t.Fatalf("reason mismatch:\n  want: %q\n  got:  %q", want, exp.Reason)
	}
}

// --- BLOCKER WITH NO StatusChangedAt (legacy data) ---

func TestBuildReadyExplanation_BlockerMissingStatusChangedAt_OmitsTime(t *testing.T) {
	blk := &Mote{ID: "node-blk-legacy", Status: "completed", StatusChangedAt: nil}
	m := &Mote{ID: "node-mote-y", Status: "active", DependsOn: []string{"node-blk-legacy"}}

	exp := BuildReadyExplanation(m, []*Mote{m, blk}, fixedNow)

	if len(exp.ClearedBlockers) != 1 {
		t.Fatalf("want 1 cleared blocker, got %d", len(exp.ClearedBlockers))
	}
	if exp.ClearedBlockers[0].ClearedAt != nil {
		t.Fatalf("legacy blocker without StatusChangedAt should have nil ClearedAt")
	}
	// Reason should not contain "ago" for a blocker with no timestamp.
	if strings.Contains(exp.Reason, "ago") {
		t.Fatalf("reason must not include 'ago' for legacy blocker without ClearedAt, got %q", exp.Reason)
	}
	if !strings.Contains(exp.Reason, "node-blk-legacy") {
		t.Fatalf("reason must still name the legacy blocker, got %q", exp.Reason)
	}
}

// --- MISSING BLOCKER (graph gap) ---

func TestBuildReadyExplanation_MissingBlockerInGraph_RecordsIdOnly(t *testing.T) {
	m := &Mote{ID: "node-mote-z", Status: "active", DependsOn: []string{"node-blk-missing"}}
	exp := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if len(exp.ClearedBlockers) != 1 {
		t.Fatalf("want 1 cleared blocker entry even for missing blocker, got %d", len(exp.ClearedBlockers))
	}
	if exp.ClearedBlockers[0].ClearedAt != nil {
		t.Fatalf("missing blocker must have nil ClearedAt")
	}
	if exp.ClearedBlockers[0].ID != "node-blk-missing" {
		t.Fatalf("missing blocker id not preserved: %+v", exp.ClearedBlockers[0])
	}
}

// --- FRESHNESS (Scenario 3) ---

func TestBuildReadyExplanation_FreshnessNever_WhenLastAccessedNil(t *testing.T) {
	m := &Mote{ID: "node-never", Status: "active", LastAccessed: nil}
	exp := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if !exp.Freshness.NeverAccessed {
		t.Fatalf("nil LastAccessed should set Freshness.NeverAccessed")
	}
	if !exp.Freshness.Stale {
		t.Fatalf("never-accessed mote should also be marked stale")
	}
	if exp.Freshness.SecondsSinceLastAccess != 0 {
		t.Fatalf("never-accessed must have 0 seconds_since_last_access, got %d", exp.Freshness.SecondsSinceLastAccess)
	}
}

func TestBuildReadyExplanation_FreshnessStale_When21DaysOld(t *testing.T) {
	old := fixedNow.Add(-21 * 24 * time.Hour)
	m := &Mote{ID: "node-stale", Status: "active", LastAccessed: &old}
	exp := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if !exp.Freshness.Stale {
		t.Fatalf("21d-old mote should be marked stale at default threshold")
	}
	if exp.Freshness.NeverAccessed {
		t.Fatalf("21d-old mote with LastAccessed set must not be NeverAccessed")
	}
	wantSecs := int64((21 * 24 * time.Hour) / time.Second)
	if exp.Freshness.SecondsSinceLastAccess != wantSecs {
		t.Fatalf("seconds_since_last_access: want %d, got %d", wantSecs, exp.Freshness.SecondsSinceLastAccess)
	}
}

func TestBuildReadyExplanation_FreshnessFresh_When5DaysOld(t *testing.T) {
	fresh := fixedNow.Add(-5 * 24 * time.Hour)
	m := &Mote{ID: "node-fresh", Status: "active", LastAccessed: &fresh}
	exp := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if exp.Freshness.Stale {
		t.Fatalf("5d-old mote should NOT be marked stale at default threshold")
	}
}

func TestBuildReadyExplanation_FreshnessExactlyAtThreshold_IsStale(t *testing.T) {
	// IsStale uses >= so a mote accessed exactly DefaultFreshnessThreshold ago is stale.
	old := fixedNow.Add(-DefaultFreshnessThreshold)
	m := &Mote{ID: "node-boundary", Status: "active", LastAccessed: &old}
	exp := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if !exp.Freshness.Stale {
		t.Fatalf("mote at exact threshold should be stale (boundary is >=)")
	}
}

// --- PARENT (Scenario 8) ---

func TestBuildReadyExplanation_ClosedParent_Highlighted(t *testing.T) {
	parent := &Mote{ID: "epic-bar", Status: "completed"}
	m := &Mote{ID: "node-task", Status: "active", Parent: "epic-bar"}
	exp := BuildReadyExplanation(m, []*Mote{m, parent}, fixedNow)
	if exp.Parent == nil {
		t.Fatalf("parent ref must be populated when parent exists")
	}
	if !exp.Parent.IsClosed {
		t.Fatalf("completed parent should set IsClosed=true")
	}
	if exp.Parent.Status != "completed" {
		t.Fatalf("parent status: want 'completed', got %q", exp.Parent.Status)
	}
}

func TestBuildReadyExplanation_DeprecatedParent_IsClosed(t *testing.T) {
	parent := &Mote{ID: "epic-old", Status: "deprecated"}
	m := &Mote{ID: "node-task", Status: "active", Parent: "epic-old"}
	exp := BuildReadyExplanation(m, []*Mote{m, parent}, fixedNow)
	if !exp.Parent.IsClosed {
		t.Fatalf("deprecated parent should set IsClosed=true")
	}
}

func TestBuildReadyExplanation_MissingParent_ReturnsNilParent(t *testing.T) {
	m := &Mote{ID: "node-orphan", Status: "active", Parent: ""}
	exp := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if exp.Parent != nil {
		t.Fatalf("orphan mote should have nil Parent in explanation")
	}
}

func TestBuildReadyExplanation_ParentNotInGraph_ReturnsNilParent(t *testing.T) {
	// Mote names a parent but the parent id isn't in the supplied graph.
	// We tolerate this — return nil rather than synthesising a half-known ref.
	m := &Mote{ID: "node-dangling", Status: "active", Parent: "epic-vanished"}
	exp := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if exp.Parent != nil {
		t.Fatalf("parent missing from graph should yield nil Parent, got %+v", exp.Parent)
	}
}

// --- PURITY / DETERMINISM ---

func TestBuildReadyExplanation_IsDeterministic(t *testing.T) {
	m := &Mote{ID: "node-x", Status: "active"}
	exp1 := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	exp2 := BuildReadyExplanation(m, []*Mote{m}, fixedNow)
	if exp1.Reason != exp2.Reason {
		t.Fatalf("non-deterministic Reason: %q vs %q", exp1.Reason, exp2.Reason)
	}
	if exp1.Freshness.Stale != exp2.Freshness.Stale {
		t.Fatalf("non-deterministic Stale: %v vs %v", exp1.Freshness.Stale, exp2.Freshness.Stale)
	}
}

// --- IsStale helper ---

func TestIsStale_NilLastAccessedReturnsFalse(t *testing.T) {
	// The helper itself returns false for nil; BuildReadyExplanation handles
	// the "never-accessed-is-stale" rule separately. This keeps IsStale a
	// straightforward duration comparison.
	if IsStale(nil, fixedNow, DefaultFreshnessThreshold) {
		t.Fatalf("IsStale(nil, ...) must be false; the never-accessed rule lives in BuildReadyExplanation")
	}
}

func TestIsStale_OldEnough_True(t *testing.T) {
	old := fixedNow.Add(-20 * 24 * time.Hour)
	if !IsStale(&old, fixedNow, DefaultFreshnessThreshold) {
		t.Fatalf("20d > 14d threshold should be stale")
	}
}

func TestIsStale_RecentEnough_False(t *testing.T) {
	recent := fixedNow.Add(-3 * 24 * time.Hour)
	if IsStale(&recent, fixedNow, DefaultFreshnessThreshold) {
		t.Fatalf("3d < 14d threshold should not be stale")
	}
}
