// SPDX-License-Identifier: MIT
//
// STORY-EXPLAIN-001 — integration coverage for `mote ls --ready --explain`.
//
// The story's 9 scenarios map to tests below; pure-data shape testing lives
// in internal/core/ready_explanation_test.go. These tests exercise the
// wiring: flag plumbing, mode composition (JSON/plain/pretty), envelope
// behavior, empty-state preservation, and the "explain doesn't filter"
// invariant.
//
// Reuses harness from cmd_ls_empty_state_test.go (setupIntegrationTest,
// runLsViaCobra, resetLsFlags, captureBothStreams) and the envelope helper
// resetJsonenvForTest from cmd_ls_envelope_test.go.
package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"motes/internal/core"
	"motes/internal/jsonenv"
)

// --- SCENARIO 6: --explain without --ready hard-errors with exit 2 ---

func TestLsExplain_WithoutReady_ReturnsExit2(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)

	err := runLsViaCobra([]string{"ls", "--explain"})
	if err == nil {
		t.Fatal("expected error when --explain is used without --ready")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Fatalf("expected exit code 2, got %d", ec.code)
	}
	if !strings.Contains(err.Error(), "--explain requires --ready") {
		t.Errorf("error message should name the constraint, got %q", err.Error())
	}
}

// --- SCENARIO 1: pretty mode shows three labelled lines per mote ---

func TestLsExplain_AddsJustificationLines(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	resetModeFlags(t)

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "no-blocker task", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var rerr error
	stdout, _ := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--explain"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0, got %v", rerr)
	}
	for _, marker := range []string{"ready because:", "freshness:"} {
		if !strings.Contains(stdout, marker) {
			t.Errorf("expected output to contain %q, got:\n%s", marker, stdout)
		}
	}
}

// --- SCENARIO 4: JSON envelope mode includes ready_explanation per mote ---

func TestLsExplain_JSON_IncludesReadyExplanation(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "json-explain task", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0, got %v", rerr)
	}

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v (stdout=%q)", err, stdout)
	}
	if len(parsed.Motes) != 1 {
		t.Fatalf("want 1 mote, got %d", len(parsed.Motes))
	}
	re := parsed.Motes[0].ReadyExplanation
	if re == nil {
		t.Fatalf("ready_explanation field must be populated when --explain is set")
	}
	if re.Reason == "" {
		t.Errorf("ready_explanation.reason must be non-empty, got %q", re.Reason)
	}
	if re.Freshness == nil {
		t.Errorf("ready_explanation.freshness must always be present")
	}
}

// --- SCENARIO 4 envelope: --explain --json wraps via JSCHEMA-001 envelope ---

func TestLsExplain_JSON_EnvelopeMode(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "envelope explain task", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0, got %v", rerr)
	}
	if stderr != "" {
		t.Errorf("envelope mode must not emit deprecation notice, got: %q", stderr)
	}

	var got struct {
		SchemaVersion int      `json:"schema_version"`
		Data          LsOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope JSON must parse: %v (stdout=%q)", err, stdout)
	}
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if len(got.Data.Motes) != 1 || got.Data.Motes[0].ReadyExplanation == nil {
		t.Errorf("envelope-wrapped output must still carry ready_explanation, got %+v", got.Data)
	}
}

// --- SCENARIO 7: empty ready set preserves the legacy / envelope shape ---

func TestLsExplain_EmptyReadySet_PreservesLegacyEmptyState(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0, got %v", rerr)
	}
	if strings.TrimSpace(stdout) != `{"motes":[]}` {
		t.Errorf("legacy empty-state must be exactly {\"motes\":[]}, got %q", stdout)
	}
	// Story §2 Scenario 7: stderr is empty. The JSCHEMA-001 deprecation
	// notice is the one tolerated exception — strip it before asserting.
	if rest := stripJSONDeprecationNotice(stderr); strings.TrimSpace(rest) != "" {
		t.Errorf("Scenario 7: stderr must be empty (modulo JSCHEMA-001 notice), got %q", stderr)
	}
}

func TestLsExplain_EmptyReadySet_PreservesEnvelopeEmptyState(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected exit 0, got %v", rerr)
	}
	if stderr != "" {
		t.Errorf("envelope empty must emit nothing on stderr (no deprecation notice in envelope mode), got %q", stderr)
	}
	var got struct {
		SchemaVersion int      `json:"schema_version"`
		Data          LsOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope empty must parse: %v (%q)", err, stdout)
	}
	if got.Data.Motes == nil || len(got.Data.Motes) != 0 {
		t.Errorf("envelope empty must be [] (not null, not populated), got %+v", got.Data.Motes)
	}
}

// --- SCENARIO 9: --explain is additive, never filters ---

func TestLsExplain_DoesNotChangeMoteSet(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	for _, title := range []string{"task A", "task B", "task C"} {
		if _, err := mm.Create("task", title, core.CreateOpts{}); err != nil {
			t.Fatalf("seed %q: %v", title, err)
		}
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	var err1, err2 error
	stdoutNoExplain := captureStdout(func() {
		err1 = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})
	stdoutExplain := captureStdout(func() {
		err2 = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})
	if err1 != nil || err2 != nil {
		t.Fatalf("expected exit 0 from both runs (err1=%v, err2=%v)", err1, err2)
	}

	idsBefore := extractIDs(t, stdoutNoExplain)
	idsAfter := extractIDs(t, stdoutExplain)
	if !sameIDs(idsBefore, idsAfter) {
		t.Errorf("--explain changed the mote set: before=%v after=%v", idsBefore, idsAfter)
	}
}

// --- SCENARIO 5: plain mode is ANSI-free with two-space indent ---

func TestLsExplain_PlainMode_NoANSI(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "plain explain task", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stdout := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain", "--plain"})
	})
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("--explain --plain leaked ANSI escapes: %q", stdout)
	}
	// Story §2 Scenario 5: "no Tufte-style separator dashes". Plain mode
	// drops the chrome from default/pretty mode — no `---` rule, no `===`,
	// no column header.
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "===") {
			t.Errorf("plain output must not emit separator dashes, found %q", line)
		}
		if strings.HasPrefix(line, "ID ") || strings.HasPrefix(line, "ID\t") {
			t.Errorf("plain output must not emit a header row, found %q", line)
		}
	}
}

func TestLsExplain_PlainMode_TwoSpaceIndent(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "plain explain task", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stdout := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain", "--plain"})
	})

	// The explanation lines must start with exactly two spaces. The mote
	// row itself starts with a non-space character (the id), so checking
	// that at least one "  ready because:" line exists in the stream is
	// sufficient evidence the indent contract holds.
	if !strings.Contains(stdout, "\n  ready because:") &&
		!strings.HasPrefix(stdout, "  ready because:") {
		// The mote row prints first; "  ready because:" should be the
		// FIRST character of the next line.
		if !strings.Contains(stdout, "\n  ready because:") {
			t.Errorf("expected two-space-indented 'ready because:' line, got:\n%s", stdout)
		}
	}
}

// --- SCENARIO 2: blocker history reads "N of N blocking deps closed (...)" ---

func TestLsExplain_BlockerHistory_Lists2Of2(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	blocker1, err := mm.Create("task", "blocker one", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed blocker1: %v", err)
	}
	blocker2, err := mm.Create("task", "blocker two", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed blocker2: %v", err)
	}
	if err := mm.Update(blocker1.ID, core.UpdateOpts{Status: core.StringPtr("completed")}); err != nil {
		t.Fatalf("close blocker1: %v", err)
	}
	if err := mm.Update(blocker2.ID, core.UpdateOpts{Status: core.StringPtr("completed")}); err != nil {
		t.Fatalf("close blocker2: %v", err)
	}
	dependent, err := mm.Create("task", "unblocked dependent", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed dependent: %v", err)
	}
	if err := mm.Link(dependent.ID, "depends_on", blocker1.ID, im); err != nil {
		t.Fatalf("link 1: %v", err)
	}
	if err := mm.Link(dependent.ID, "depends_on", blocker2.ID, im); err != nil {
		t.Fatalf("link 2: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	stdout := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})
	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v (%q)", err, stdout)
	}
	// Find the dependent in the output (the closed blockers won't appear in
	// --ready because they're not active tasks).
	var found *LsMoteEntry
	for i := range parsed.Motes {
		if parsed.Motes[i].ID == dependent.ID {
			found = &parsed.Motes[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("dependent %s missing from --ready set: %+v", dependent.ID, parsed.Motes)
	}
	if found.ReadyExplanation == nil {
		t.Fatalf("ready_explanation must be set on the dependent")
	}
	if !strings.Contains(found.ReadyExplanation.Reason, "2 of 2 blocking deps closed") {
		t.Errorf("reason should be '2 of 2 blocking deps closed (...)', got %q", found.ReadyExplanation.Reason)
	}
	if len(found.ReadyExplanation.ClearedBlockers) != 2 {
		t.Errorf("want 2 cleared blockers, got %d", len(found.ReadyExplanation.ClearedBlockers))
	}
}

// --- SCENARIO 8: closed parent shows "(CLOSED — completed)" highlight ---

func TestLsExplain_ClosedParentHighlighted(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)

	parent, err := mm.Create("task", "parent epic", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if err := mm.Update(parent.ID, core.UpdateOpts{Status: core.StringPtr("completed")}); err != nil {
		t.Fatalf("close parent: %v", err)
	}
	child, err := mm.Create("task", "orphaned child", core.CreateOpts{Parent: parent.ID})
	if err != nil {
		t.Fatalf("seed child: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	stdout := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v (%q)", err, stdout)
	}
	var found *LsMoteEntry
	for i := range parsed.Motes {
		if parsed.Motes[i].ID == child.ID {
			found = &parsed.Motes[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("child %s missing from --ready set", child.ID)
	}
	if found.ReadyExplanation.Parent == nil {
		t.Fatalf("parent ref must be populated for child with Parent=%s", parent.ID)
	}
	if !found.ReadyExplanation.Parent.IsClosed {
		t.Errorf("completed parent should set is_closed=true, got %+v", found.ReadyExplanation.Parent)
	}

	// Now verify the human render highlights it.
	stdoutPretty := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain"})
	})
	if !strings.Contains(stdoutPretty, "CLOSED — completed") {
		t.Errorf("pretty render of closed parent should include 'CLOSED — completed', got:\n%s", stdoutPretty)
	}
}

// --- SCENARIO 1 (full): all three labelled lines appear under a mote with a parent ---

// The original TestLsExplain_AddsJustificationLines only covered a parentless
// mote so it could not assert the `parent epic:` line. This test seeds a child
// task under an active epic and verifies all three lines render.
func TestLsExplain_PrettyMode_AllThreeLabelsForParentedMote(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)

	mm := core.NewMoteManager(root)
	parent, err := mm.Create("task", "epic", core.CreateOpts{Body: "epic body"})
	if err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if _, err := mm.Create("task", "child of epic", core.CreateOpts{Body: "child body", Parent: parent.ID}); err != nil {
		t.Fatalf("seed child: %v", err)
	}

	stdout := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain"})
	})
	for _, marker := range []string{"ready because:", "parent epic:", "freshness:"} {
		if !strings.Contains(stdout, marker) {
			t.Errorf("expected pretty output to contain %q, got:\n%s", marker, stdout)
		}
	}
	// Active parent renders as "(active)" (Scenario 8 partial — the
	// not-closed case complements the closed-parent assertion below).
	if !strings.Contains(stdout, "parent epic: "+parent.ID+" (active)") {
		t.Errorf("expected active-parent line 'parent epic: %s (active)', got:\n%s", parent.ID, stdout)
	}
}

// --- SCENARIO 3: freshness rendering — fresh, stale, never (integration) ---
//
// Unit-level coverage of the data model lives in
// internal/core/ready_explanation_test.go. This test pins the *rendered*
// strings the story specifies in §2 Scenario 3 ("Nd (fresh)", "Nd (stale —
// not touched in 14d)", "never accessed") so a future refactor of the
// renderer can't silently drift them.

func TestLsExplain_Freshness_RenderingShapes(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)

	mm := core.NewMoteManager(root)

	freshTask, err := mm.Create("task", "5d-old fresh task", core.CreateOpts{Body: "fresh"})
	if err != nil {
		t.Fatalf("seed fresh: %v", err)
	}
	staleTask, err := mm.Create("task", "21d-old stale task", core.CreateOpts{Body: "stale"})
	if err != nil {
		t.Fatalf("seed stale: %v", err)
	}
	// neverTask intentionally has no LastAccessed update.
	neverTask, err := mm.Create("task", "never-accessed task", core.CreateOpts{Body: "never"})
	if err != nil {
		t.Fatalf("seed never: %v", err)
	}

	now := time.Now()
	fresh5d := now.Add(-5 * 24 * time.Hour)
	stale21d := now.Add(-21 * 24 * time.Hour)
	if err := mm.Update(freshTask.ID, core.UpdateOpts{LastAccessed: &fresh5d}); err != nil {
		t.Fatalf("set fresh LastAccessed: %v", err)
	}
	if err := mm.Update(staleTask.ID, core.UpdateOpts{LastAccessed: &stale21d}); err != nil {
		t.Fatalf("set stale LastAccessed: %v", err)
	}

	stdout := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain", "--plain"})
	})

	type expectation struct {
		moteID string
		want   string
	}
	cases := []expectation{
		{freshTask.ID, "  freshness: 5d (fresh)"},
		{staleTask.ID, "  freshness: 21d (stale — not touched in 14d)"},
		{neverTask.ID, "  freshness: never accessed"},
	}
	for _, c := range cases {
		// Find the line following the mote's row and assert it matches.
		idx := strings.Index(stdout, c.moteID)
		if idx < 0 {
			t.Errorf("mote %s not found in output:\n%s", c.moteID, stdout)
			continue
		}
		segment := stdout[idx:]
		if !strings.Contains(segment, c.want) {
			t.Errorf("for mote %s, expected to find %q in segment\n%s", c.moteID, c.want, segment)
		}
	}
}

// --- SCENARIO 4 (full): JSON shape covers all four sub-objects with required keys ---

func TestLsExplain_JSON_StructureMatchesSchema(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	blocker, err := mm.Create("task", "the blocker", core.CreateOpts{Body: "blk"})
	if err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	if err := mm.Update(blocker.ID, core.UpdateOpts{Status: core.StringPtr("completed")}); err != nil {
		t.Fatalf("close blocker: %v", err)
	}
	parent, err := mm.Create("task", "parent epic", core.CreateOpts{Body: "epic"})
	if err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	dep, err := mm.Create("task", "dependent", core.CreateOpts{Body: "dep", Parent: parent.ID})
	if err != nil {
		t.Fatalf("seed dep: %v", err)
	}
	if err := mm.Link(dep.ID, "depends_on", blocker.ID, im); err != nil {
		t.Fatalf("link: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	stdout := captureStdout(func() {
		_ = runLsViaCobra([]string{"ls", "--ready", "--explain", "--json"})
	})

	// Use a permissive struct that exactly mirrors the documented schema
	// (docs/JSON_SCHEMA.md §4.1). Decode and check every required path.
	var parsed struct {
		Motes []struct {
			ID               string `json:"id"`
			ReadyExplanation struct {
				Reason          string `json:"reason"`
				ClearedBlockers []struct {
					ID        string `json:"id"`
					ClearedAt string `json:"cleared_at,omitempty"`
				} `json:"cleared_blockers"`
				Parent *struct {
					ID       string `json:"id"`
					Status   string `json:"status"`
					IsClosed bool   `json:"is_closed"`
				} `json:"parent,omitempty"`
				Freshness *struct {
					SecondsSinceLastAccess int64 `json:"seconds_since_last_access"`
					Stale                  bool  `json:"stale"`
					NeverAccessed          bool  `json:"never_accessed"`
				} `json:"freshness"`
			} `json:"ready_explanation"`
		} `json:"motes"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("documented schema must decode cleanly: %v (%q)", err, stdout)
	}

	var depEntry *struct {
		ID               string `json:"id"`
		ReadyExplanation struct {
			Reason          string `json:"reason"`
			ClearedBlockers []struct {
				ID        string `json:"id"`
				ClearedAt string `json:"cleared_at,omitempty"`
			} `json:"cleared_blockers"`
			Parent *struct {
				ID       string `json:"id"`
				Status   string `json:"status"`
				IsClosed bool   `json:"is_closed"`
			} `json:"parent,omitempty"`
			Freshness *struct {
				SecondsSinceLastAccess int64 `json:"seconds_since_last_access"`
				Stale                  bool  `json:"stale"`
				NeverAccessed          bool  `json:"never_accessed"`
			} `json:"freshness"`
		} `json:"ready_explanation"`
	}
	for i := range parsed.Motes {
		if parsed.Motes[i].ID == dep.ID {
			depEntry = &parsed.Motes[i]
			break
		}
	}
	if depEntry == nil {
		t.Fatalf("dependent %s missing from --ready set", dep.ID)
	}
	re := depEntry.ReadyExplanation
	if re.Reason == "" {
		t.Errorf("reason must be non-empty")
	}
	if len(re.ClearedBlockers) != 1 || re.ClearedBlockers[0].ID != blocker.ID {
		t.Errorf("cleared_blockers must include {id=%s}, got %+v", blocker.ID, re.ClearedBlockers)
	}
	if re.ClearedBlockers[0].ClearedAt == "" {
		t.Errorf("cleared_blockers[0].cleared_at must be set for a blocker with StatusChangedAt, got empty")
	}
	if re.Parent == nil || re.Parent.ID != parent.ID || re.Parent.Status == "" {
		t.Errorf("parent must carry {id=%s, status=non-empty}, got %+v", parent.ID, re.Parent)
	}
	if re.Freshness == nil {
		t.Errorf("freshness must always be present")
	}
}

// --- helpers shared by the explain tests ---

func extractIDs(t *testing.T, jsonStr string) []string {
	t.Helper()
	var parsed LsOutput
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v (%q)", err, jsonStr)
	}
	out := make([]string, len(parsed.Motes))
	for i, m := range parsed.Motes {
		out[i] = m.ID
	}
	return out
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, id := range a {
		seen[id]++
	}
	for _, id := range b {
		seen[id]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
