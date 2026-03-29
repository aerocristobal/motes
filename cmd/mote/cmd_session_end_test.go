package main

import (
	"testing"
	"time"

	"motes/internal/core"
)

// --- countLingeringTasks tests ---

func TestCountLingeringTasks_FilterLogic(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)    // 10 days ago
	recent := time.Now().AddDate(0, 0, -3)  // 3 days ago — too new
	crystallized := time.Now()

	motes := []*core.Mote{
		// Qualifies: task, completed, >7d, no crystallized_at
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
		// Does not qualify: recent
		{Type: "task", Status: "completed", CreatedAt: recent},
		// Does not qualify: crystallized
		{Type: "task", Status: "completed", CreatedAt: old, CrystallizedAt: &crystallized},
		// Does not qualify: wrong type
		{Type: "decision", Status: "completed", CreatedAt: old},
		// Does not qualify: wrong status
		{Type: "task", Status: "active", CreatedAt: old},
		// Does not qualify: archived (not completed)
		{Type: "task", Status: "archived", CreatedAt: old},
	}

	count := countLingeringTasks(motes)
	if count != 2 {
		t.Errorf("countLingeringTasks: got %d, want 2", count)
	}
}

func TestCountLingeringTasks_EmptySlice(t *testing.T) {
	if countLingeringTasks(nil) != 0 {
		t.Error("expected 0 for nil motes")
	}
	if countLingeringTasks([]*core.Mote{}) != 0 {
		t.Error("expected 0 for empty motes")
	}
}

// --- lastSessionHadNudge tests ---

func TestLastSessionHadNudge_Empty(t *testing.T) {
	if lastSessionHadNudge(nil) {
		t.Error("expected false for nil stats")
	}
	if lastSessionHadNudge([]core.PrimeSessionStats{}) {
		t.Error("expected false for empty stats")
	}
}

func TestLastSessionHadNudge_LastTrue(t *testing.T) {
	stats := []core.PrimeSessionStats{
		{SessionAt: "2026-01-01T00:00:00Z", LingeringNudge: false},
		{SessionAt: "2026-01-02T00:00:00Z", LingeringNudge: true},
	}
	if !lastSessionHadNudge(stats) {
		t.Error("expected true when last session had nudge")
	}
}

func TestLastSessionHadNudge_LastFalse(t *testing.T) {
	stats := []core.PrimeSessionStats{
		{SessionAt: "2026-01-01T00:00:00Z", LingeringNudge: true},
		{SessionAt: "2026-01-02T00:00:00Z", LingeringNudge: false},
	}
	if lastSessionHadNudge(stats) {
		t.Error("expected false when last session did not have nudge")
	}
}

// --- shouldShowLingeringNudge integration tests ---

func TestLingeringNudge_ThrottleSkipsIfLastSessionShown(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	motes := []*core.Mote{
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
	}
	stats := []core.PrimeSessionStats{
		{SessionAt: "2026-01-01T00:00:00Z", LingeringNudge: true},
	}

	count := countLingeringTasks(motes)
	show := count >= 3 && !lastSessionHadNudge(stats)
	if show {
		t.Error("nudge should be suppressed when last session already showed it")
	}
}

func TestLingeringNudge_ShowsWhenLastSessionClear(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	motes := []*core.Mote{
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
	}
	stats := []core.PrimeSessionStats{
		{SessionAt: "2026-01-01T00:00:00Z", LingeringNudge: false},
	}

	count := countLingeringTasks(motes)
	show := count >= 3 && !lastSessionHadNudge(stats)
	if !show {
		t.Error("nudge should be shown when last session was clear")
	}
}

func TestLingeringNudge_InsufficientTasks(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	motes := []*core.Mote{
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
		// only 2 — below threshold of 3
	}
	count := countLingeringTasks(motes)
	show := count >= 3 && !lastSessionHadNudge(nil)
	if show {
		t.Error("nudge should not show with fewer than 3 lingering tasks")
	}
}

func TestLingeringNudge_ShowsWithNoHistory(t *testing.T) {
	old := time.Now().AddDate(0, 0, -10)
	motes := []*core.Mote{
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
		{Type: "task", Status: "completed", CreatedAt: old},
	}

	count := countLingeringTasks(motes)
	show := count >= 3 && !lastSessionHadNudge(nil)
	if !show {
		t.Error("nudge should show on first session with 3+ lingering tasks")
	}
}
