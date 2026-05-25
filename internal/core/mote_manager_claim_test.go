// SPDX-License-Identifier: MIT
package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- HAPPY PATH (Scenario 1) ---

func TestClaim_ActiveTask_Succeeds(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, err := mm.Create("task", "claimable", CreateOpts{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := mm.Claim(m.ID, "claude-alpha")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if res == nil || !res.Claimed {
		t.Fatalf("expected Claimed=true, got %+v", res)
	}
	if res.ClaimedBy != "claude-alpha" {
		t.Errorf("ClaimedBy: got %q, want claude-alpha", res.ClaimedBy)
	}

	got, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Status != "in_progress" {
		t.Errorf("on-disk status: got %q, want in_progress", got.Status)
	}
	if got.ClaimedBy != "claude-alpha" {
		t.Errorf("on-disk claimed_by: got %q, want claude-alpha", got.ClaimedBy)
	}
	if got.StatusChangedAt == nil {
		t.Error("StatusChangedAt should be set after a claim")
	}
}

// --- BOUNDARY (Scenario 3) ---

func TestClaim_LastReadyMote_SucceedsAndQueueEmpties(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, err := mm.Create("task", "lone-ready", CreateOpts{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Sanity: it's the only ready task before claim.
	ready, _ := mm.List(ListFilters{Ready: true, Type: "task"})
	if len(ready) != 1 || ready[0].ID != m.ID {
		t.Fatalf("expected 1 ready task before claim, got %d", len(ready))
	}

	if _, err := mm.Claim(m.ID, "claude-alpha"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	readyAfter, _ := mm.List(ListFilters{Ready: true, Type: "task"})
	if len(readyAfter) != 0 {
		t.Errorf("expected 0 ready tasks after claim, got %d", len(readyAfter))
	}
}

// --- ERROR: already claimed (Scenario 4) ---

func TestClaim_AlreadyInProgress_ReturnsErrAlreadyClaimed(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, _ := mm.Create("task", "twice", CreateOpts{})

	if _, err := mm.Claim(m.ID, "codex-beta"); err != nil {
		t.Fatalf("first Claim: %v", err)
	}

	res, err := mm.Claim(m.ID, "claude-alpha")
	if err == nil {
		t.Fatal("expected error on second Claim, got nil")
	}
	if !errors.Is(err, ErrAlreadyClaimed) {
		t.Errorf("expected ErrAlreadyClaimed, got %v", err)
	}
	if res == nil || res.Claimed {
		t.Fatalf("expected non-nil ClaimResult with Claimed=false, got %+v", res)
	}
	if res.CurrentClaimedBy != "codex-beta" {
		t.Errorf("CurrentClaimedBy: got %q, want codex-beta", res.CurrentClaimedBy)
	}
	if res.CurrentStatus != "in_progress" {
		t.Errorf("CurrentStatus: got %q, want in_progress", res.CurrentStatus)
	}

	// On-disk mote retains the first claimer.
	got, _ := mm.Read(m.ID)
	if got.ClaimedBy != "codex-beta" {
		t.Errorf("on-disk claimed_by changed: got %q, want codex-beta", got.ClaimedBy)
	}
}

// --- ERROR: terminal status (Scenario 5) ---

func TestClaim_TerminalStatus_ReturnsErrNotClaimable(t *testing.T) {
	cases := []string{"completed", "archived", "deprecated"}
	for _, status := range cases {
		t.Run(status, func(t *testing.T) {
			_, mm := setupTestMemory(t)
			m, _ := mm.Create("task", "terminal", CreateOpts{})
			if err := mm.Update(m.ID, UpdateOpts{Status: StringPtr(status)}); err != nil {
				t.Fatalf("setup Update: %v", err)
			}

			_, err := mm.Claim(m.ID, "claude-alpha")
			if err == nil {
				t.Fatal("expected error for terminal status, got nil")
			}
			if !errors.Is(err, ErrNotClaimable) {
				t.Errorf("expected ErrNotClaimable, got %v", err)
			}
			if !strings.Contains(err.Error(), "status="+status) {
				t.Errorf("expected error to mention status=%s, got %q", status, err.Error())
			}

			got, _ := mm.Read(m.ID)
			if got.Status != status {
				t.Errorf("status mutated: got %q, want %q", got.Status, status)
			}
			if got.ClaimedBy != "" {
				t.Errorf("ClaimedBy should not be stamped on rejection, got %q", got.ClaimedBy)
			}
		})
	}
}

// --- ERROR: non-task type ---

func TestClaim_NonTaskType_ReturnsErrNotClaimable(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, err := mm.Create("lesson", "not a task", CreateOpts{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = mm.Claim(m.ID, "claude-alpha")
	if err == nil {
		t.Fatal("expected error for non-task type, got nil")
	}
	if !errors.Is(err, ErrNotClaimable) {
		t.Errorf("expected ErrNotClaimable, got %v", err)
	}
	if !strings.Contains(err.Error(), "type=lesson") {
		t.Errorf("expected error to mention type=lesson, got %q", err.Error())
	}
}

// --- ERROR: unfinished blockers (Scenario 6) ---

func TestClaim_UnfinishedBlockers_ReturnsErrNotReady(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)

	blocker, _ := mm.Create("task", "blocker", CreateOpts{})
	target, _ := mm.Create("task", "blocked", CreateOpts{})
	if err := mm.Link(target.ID, "depends_on", blocker.ID, im); err != nil {
		t.Fatalf("Link: %v", err)
	}

	_, err := mm.Claim(target.ID, "claude-alpha")
	if err == nil {
		t.Fatal("expected error when blockers unfinished, got nil")
	}
	if !errors.Is(err, ErrNotReady) {
		t.Errorf("expected ErrNotReady, got %v", err)
	}
	if !strings.Contains(err.Error(), "1 unfinished blocker") {
		t.Errorf("expected blocker count in error, got %q", err.Error())
	}

	got, _ := mm.Read(target.ID)
	if got.Status != "active" {
		t.Errorf("status mutated despite ready check failure: got %q", got.Status)
	}
	if got.ClaimedBy != "" {
		t.Errorf("ClaimedBy stamped despite failure: got %q", got.ClaimedBy)
	}
}

// --- BUSINESS RULE EDGE: concurrent claim (Scenario 8) ---

func TestClaim_ConcurrentAttempts_ExactlyOneWins(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, _ := mm.Create("task", "race", CreateOpts{})

	var (
		wg            sync.WaitGroup
		mu            sync.Mutex
		winners       int
		losers        int
		winnerAgent   string
		loserGotErrIs bool
	)

	claimers := []string{"claude-alpha", "codex-beta"}
	wg.Add(len(claimers))
	for _, agent := range claimers {
		agent := agent
		go func() {
			defer wg.Done()
			res, err := mm.Claim(m.ID, agent)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				winners++
				winnerAgent = res.ClaimedBy
				return
			}
			if errors.Is(err, ErrAlreadyClaimed) {
				losers++
				loserGotErrIs = true
			}
		}()
	}
	wg.Wait()

	if winners != 1 {
		t.Errorf("expected exactly 1 winner, got %d", winners)
	}
	if losers != 1 {
		t.Errorf("expected exactly 1 loser, got %d", losers)
	}
	if !loserGotErrIs {
		t.Error("loser should receive ErrAlreadyClaimed-wrapped error")
	}

	got, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("Read after race: %v", err)
	}
	if got.Status != "in_progress" {
		t.Errorf("status: got %q, want in_progress", got.Status)
	}
	if got.ClaimedBy != winnerAgent {
		t.Errorf("on-disk ClaimedBy=%q, winner reported %q", got.ClaimedBy, winnerAgent)
	}
	// Either agent is a valid winner; just confirm it's one of them.
	if winnerAgent != "claude-alpha" && winnerAgent != "codex-beta" {
		t.Errorf("winner is neither claimer: %q", winnerAgent)
	}
}

// --- AUDIT TRAIL ---

func TestClaim_WritesAuditEvent(t *testing.T) {
	root, mm := setupTestMemory(t)
	m, _ := mm.Create("task", "audit me", CreateOpts{})

	t.Setenv("MOTE_AGENT_ID", "claude-alpha")
	if _, err := mm.Claim(m.ID, "claude-alpha"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	auditPath := filepath.Join(root, "audit.jsonl")
	f, err := os.Open(auditPath)
	if err != nil {
		t.Fatalf("open audit.jsonl: %v", err)
	}
	defer func() { _ = f.Close() }()

	var found bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Operation != "claim" || entry.MoteID != m.ID {
			continue
		}
		found = true
		if entry.AgentID != "claude-alpha" {
			t.Errorf("audit AgentID: got %q, want claude-alpha", entry.AgentID)
		}
		gotFields := strings.Join(entry.FieldsSet, ",")
		if !strings.Contains(gotFields, "status") || !strings.Contains(gotFields, "claimed_by") {
			t.Errorf("audit FieldsSet: got %v, want both status and claimed_by", entry.FieldsSet)
		}
		if entry.Timestamp == "" {
			t.Error("audit Timestamp should be set")
		}
	}
	if !found {
		t.Error("no claim audit entry found in audit.jsonl")
	}
}
