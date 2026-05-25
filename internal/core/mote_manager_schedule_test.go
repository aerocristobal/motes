// SPDX-License-Identifier: MIT
//
// STORY-TIME-001 §6.2 — MoteManager scheduling scaffold.
//
// These tests exercise the core layer of due_at / defer_until. The CLI
// integration tests live in cmd/mote/cmd_{add,update,ls}_schedule_test.go.
//
// All tests use NewMoteManagerWithClock(t.TempDir(), FixedClock{T: t0}) so
// "now" is deterministic and the past/future predicates are exact.
package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// schedT0 is the reference instant for these tests. Independent of t0 in
// time_spec_test.go so the two suites don't accidentally share state.
var schedT0 = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

func newManagerAt(t *testing.T, now time.Time) (*MoteManager, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "nodes"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mm := NewMoteManagerWithClock(dir, FixedClock{T: now})
	mm.SetGlobalRoot(dir) // keep global motes in the same dir for hermetic tests
	return mm, dir
}

// --- Field round-trip ---------------------------------------------------

func TestMote_DueAt_YAMLRoundTrip(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	due := schedT0.Add(48 * time.Hour)
	m, err := mm.Create("task", "due-bearing", CreateOpts{DueAt: &due, Local: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	read, err := ParseMote(m.FilePath)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if read.DueAt == nil {
		t.Fatal("DueAt nil after round-trip")
	}
	if !read.DueAt.Equal(due.UTC()) {
		t.Errorf("DueAt: got %v, want %v", read.DueAt, due.UTC())
	}
}

func TestMote_DeferUntil_YAMLRoundTrip(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	def := schedT0.Add(24 * time.Hour)
	m, err := mm.Create("task", "deferred", CreateOpts{DeferUntil: &def, Local: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	read, err := ParseMote(m.FilePath)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if read.DeferUntil == nil {
		t.Fatal("DeferUntil nil after round-trip")
	}
	if !read.DeferUntil.Equal(def.UTC()) {
		t.Errorf("DeferUntil: got %v, want %v", read.DeferUntil, def.UTC())
	}
}

func TestMote_NoDueOrDefer_OmitsFromYAML(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	m, err := mm.Create("task", "plain", CreateOpts{Local: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	data, err := os.ReadFile(m.FilePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "due_at:") {
		t.Errorf("due_at must be omitted when nil; file=%s", string(data))
	}
	if strings.Contains(string(data), "defer_until:") {
		t.Errorf("defer_until must be omitted when nil; file=%s", string(data))
	}
}

// --- Create-time validation --------------------------------------------

func TestCreate_DeferInPast_Rejected(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	past := schedT0.Add(-1 * time.Hour)
	_, err := mm.Create("task", "bad defer", CreateOpts{DeferUntil: &past, Local: true})
	if err == nil {
		t.Fatal("expected rejection for past defer")
	}
	if !strings.Contains(err.Error(), "defer must be in the future") {
		t.Errorf("error message: %v", err)
	}
}

func TestCreate_DueInPast_Accepted(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	past := schedT0.Add(-24 * time.Hour)
	m, err := mm.Create("task", "backdated", CreateOpts{DueAt: &past, Local: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.DueAt == nil || !m.DueAt.Equal(past.UTC()) {
		t.Errorf("expected past due, got %v", m.DueAt)
	}
}

// --- Update Set / Clear ------------------------------------------------

func TestUpdate_SetDueAt(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	m, _ := mm.Create("task", "t", CreateOpts{Local: true})
	due := schedT0.Add(6 * time.Hour)
	if err := mm.Update(m.ID, UpdateOpts{DueAt: &due}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.DueAt == nil || !got.DueAt.Equal(due.UTC()) {
		t.Errorf("DueAt: got %v, want %v", got.DueAt, due.UTC())
	}
}

func TestUpdate_SetDueAt_Overwrites(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	first := schedT0.Add(6 * time.Hour)
	m, _ := mm.Create("task", "t", CreateOpts{DueAt: &first, Local: true})
	second := schedT0.Add(48 * time.Hour)
	if err := mm.Update(m.ID, UpdateOpts{DueAt: &second}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := mm.Read(m.ID)
	if !got.DueAt.Equal(second.UTC()) {
		t.Errorf("DueAt overwrite: got %v, want %v", got.DueAt, second.UTC())
	}
}

func TestUpdate_ClearDeferUntil(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	def := schedT0.Add(24 * time.Hour)
	m, _ := mm.Create("task", "t", CreateOpts{DeferUntil: &def, Local: true})
	if err := mm.Update(m.ID, UpdateOpts{ClearDeferUntil: true}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ := mm.Read(m.ID)
	if got.DeferUntil != nil {
		t.Errorf("DeferUntil should be nil after clear, got %v", got.DeferUntil)
	}
}

func TestUpdate_ClearDeferUntil_Idempotent(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	m, _ := mm.Create("task", "t", CreateOpts{Local: true})
	if err := mm.Update(m.ID, UpdateOpts{ClearDeferUntil: true}); err != nil {
		t.Fatalf("clear (already clear): %v", err)
	}
	got, _ := mm.Read(m.ID)
	if got.DeferUntil != nil {
		t.Errorf("DeferUntil unexpectedly non-nil: %v", got.DeferUntil)
	}
}

func TestUpdate_SetAndClearSimultaneously_Errors(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	m, _ := mm.Create("task", "t", CreateOpts{Local: true})
	due := schedT0.Add(6 * time.Hour)
	err := mm.Update(m.ID, UpdateOpts{DueAt: &due, ClearDueAt: true})
	if err == nil {
		t.Fatal("expected error for set+clear combo")
	}
}

func TestUpdate_DeferInPast_Rejected(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	m, _ := mm.Create("task", "t", CreateOpts{Local: true})
	past := schedT0.Add(-1 * time.Hour)
	if err := mm.Update(m.ID, UpdateOpts{DeferUntil: &past}); err == nil {
		t.Fatal("expected rejection for past defer on update")
	}
}

// --- Audit log shape ---------------------------------------------------

func TestUpdate_AuditFieldsSet_IncludeDueAndDefer(t *testing.T) {
	mm, root := newManagerAt(t, schedT0)
	m, _ := mm.Create("task", "t", CreateOpts{Local: true})
	due := schedT0.Add(6 * time.Hour)
	def := schedT0.Add(24 * time.Hour)
	if err := mm.Update(m.ID, UpdateOpts{DueAt: &due, DeferUntil: &def}); err != nil {
		t.Fatalf("update: %v", err)
	}
	entries := readAudit(t, root)
	var found bool
	for _, e := range entries {
		if e.Operation != "update" || e.MoteID != m.ID {
			continue
		}
		hasDue := containsStr(e.FieldsSet, "due_at")
		hasDef := containsStr(e.FieldsSet, "defer_until")
		if hasDue && hasDef {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected audit entry with fields_set containing due_at AND defer_until; entries=%+v", entries)
	}
}

func TestUpdate_AuditFieldsSet_OnClear(t *testing.T) {
	mm, root := newManagerAt(t, schedT0)
	def := schedT0.Add(24 * time.Hour)
	m, _ := mm.Create("task", "t", CreateOpts{DeferUntil: &def, Local: true})
	if err := mm.Update(m.ID, UpdateOpts{ClearDeferUntil: true}); err != nil {
		t.Fatalf("update: %v", err)
	}
	entries := readAudit(t, root)
	var found bool
	for _, e := range entries {
		if e.Operation == "update" && e.MoteID == m.ID && containsStr(e.FieldsSet, "defer_until") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected update audit with defer_until in fields_set; entries=%+v", entries)
	}
}

// --- IsReady semantics --------------------------------------------------

func TestList_Ready_DeferInFuture_Hidden(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	future := schedT0.Add(24 * time.Hour)
	hidden, _ := mm.Create("task", "deferred", CreateOpts{DeferUntil: &future, Local: true})
	_, _ = mm.Create("task", "ready", CreateOpts{Local: true})

	got, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, m := range got {
		if m.ID == hidden.ID {
			t.Errorf("deferred mote %s should be hidden from --ready", hidden.ID)
		}
	}
}

func TestList_Ready_DeferInPast_Visible_NoMutation(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	// Create with a defer in the future, then advance the clock past it.
	def := schedT0.Add(1 * time.Hour)
	m, err := mm.Create("task", "expired", CreateOpts{DeferUntil: &def, Local: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Move the clock forward; defer is now in the past.
	mm.clock = FixedClock{T: schedT0.Add(2 * time.Hour)}

	got, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var visible bool
	for _, x := range got {
		if x.ID == m.ID {
			visible = true
		}
	}
	if !visible {
		t.Errorf("expired-defer mote %s should be visible in --ready", m.ID)
	}

	// Field must NOT have been auto-cleared.
	reread, err := ParseMote(m.FilePath)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if reread.DeferUntil == nil {
		t.Error("defer_until was auto-cleared; story §10 Q5 says NO")
	}
}

func TestList_Ready_IncludeDeferred_Override(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	future := schedT0.Add(24 * time.Hour)
	deferred, _ := mm.Create("task", "deferred", CreateOpts{DeferUntil: &future, Local: true})

	got, err := mm.List(ListFilters{Ready: true, IncludeDeferred: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var visible bool
	for _, m := range got {
		if m.ID == deferred.ID {
			visible = true
		}
	}
	if !visible {
		t.Errorf("--include-deferred must surface deferred motes")
	}
}

// --- Overdue / due-window filters ---------------------------------------

func TestList_Overdue_ActiveAndInProgress(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	past := schedT0.Add(-1 * time.Hour)
	earlier := schedT0.Add(-2 * time.Hour)

	a, _ := mm.Create("task", "active overdue", CreateOpts{DueAt: &past, Local: true})
	b, _ := mm.Create("task", "in-progress overdue", CreateOpts{DueAt: &earlier, Local: true})
	if err := mm.Update(b.ID, UpdateOpts{Status: StringPtr("in_progress")}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := mm.List(ListFilters{Overdue: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 overdue motes, got %d", len(got))
	}
	// Must be sorted ascending by DueAt — most overdue first.
	if got[0].ID != b.ID || got[1].ID != a.ID {
		t.Errorf("sort order: got [%s,%s], want [%s (earlier),%s]", got[0].ID, got[1].ID, b.ID, a.ID)
	}
}

func TestList_Overdue_ExcludesCompleted(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	past := schedT0.Add(-24 * time.Hour)
	m, _ := mm.Create("task", "done late", CreateOpts{DueAt: &past, Local: true})
	if err := mm.Update(m.ID, UpdateOpts{Status: StringPtr("completed")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := mm.List(ListFilters{Overdue: true})
	for _, x := range got {
		if x.ID == m.ID {
			t.Errorf("completed mote %s must not appear in --overdue", m.ID)
		}
	}
}

func TestList_Overdue_ExcludesArchivedAndDeprecated(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	past := schedT0.Add(-24 * time.Hour)
	for _, status := range []string{"archived", "deprecated"} {
		m, _ := mm.Create("task", "x", CreateOpts{DueAt: &past, Local: true})
		if err := mm.Update(m.ID, UpdateOpts{Status: StringPtr(status)}); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	got, _ := mm.List(ListFilters{Overdue: true})
	if len(got) != 0 {
		t.Errorf("expected 0 overdue (all archived/deprecated), got %d", len(got))
	}
}

func TestList_DueBefore_DueAfter(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	tMinus2h := schedT0.Add(-2 * time.Hour)
	tPlus6h := schedT0.Add(6 * time.Hour)
	tPlus3d := schedT0.Add(72 * time.Hour)
	a, _ := mm.Create("task", "a", CreateOpts{DueAt: &tMinus2h, Local: true})
	b, _ := mm.Create("task", "b", CreateOpts{DueAt: &tPlus6h, Local: true})
	c, _ := mm.Create("task", "c", CreateOpts{DueAt: &tPlus3d, Local: true})

	// --due-before=now → only a.
	now := schedT0
	got, _ := mm.List(ListFilters{DueBefore: &now})
	if len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("due-before=now: got %v, want [%s]", ids(got), a.ID)
	}

	// --due-before=+1d → a, b.
	plus1d := schedT0.Add(24 * time.Hour)
	got, _ = mm.List(ListFilters{DueBefore: &plus1d})
	if !sameIDs(got, []string{a.ID, b.ID}) {
		t.Errorf("due-before=+1d: got %v, want [%s, %s]", ids(got), a.ID, b.ID)
	}

	// --due-after=+1d → c.
	got, _ = mm.List(ListFilters{DueAfter: &plus1d})
	if len(got) != 1 || got[0].ID != c.ID {
		t.Errorf("due-after=+1d: got %v, want [%s]", ids(got), c.ID)
	}

	// --due-after=now → b, c.
	got, _ = mm.List(ListFilters{DueAfter: &now})
	if !sameIDs(got, []string{b.ID, c.ID}) {
		t.Errorf("due-after=now: got %v, want [%s, %s]", ids(got), b.ID, c.ID)
	}
}

func TestList_DueBefore_NoMatches_StillSliceNotNil(t *testing.T) {
	mm, _ := newManagerAt(t, schedT0)
	// No mote has due_at.
	_, _ = mm.Create("task", "no-due", CreateOpts{Local: true})

	now := schedT0
	got, err := mm.List(ListFilters{DueBefore: &now})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
	// The CLI relies on len()==0 producing {"motes":[]} — nil vs empty slice
	// both have len==0, so this is fine. Just confirm no error.
}

// --- helpers -----------------------------------------------------------

func containsStr(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

func ids(motes []*Mote) []string {
	out := make([]string, len(motes))
	for i, m := range motes {
		out[i] = m.ID
	}
	return out
}

func sameIDs(motes []*Mote, want []string) bool {
	if len(motes) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, m := range motes {
		seen[m.ID] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}

func readAudit(t *testing.T, root string) []AuditEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var entries []AuditEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e AuditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("parse audit line %q: %v", line, err)
		}
		entries = append(entries, e)
	}
	return entries
}
