// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

// --- Unit tests for pure helper functions ---

func TestGithubTypeToMote(t *testing.T) {
	tests := []struct {
		name       string
		labels     []githubLabel
		wantType   string
		wantOrigin string
	}{
		{"bug label", []githubLabel{{Name: "bug"}}, "lesson", "failure"},
		{"bug with others", []githubLabel{{Name: "verification"}, {Name: "bug"}}, "lesson", "failure"},
		{"no bug", []githubLabel{{Name: "verification"}}, "task", "normal"},
		{"empty labels", nil, "task", "normal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moteType, origin := githubTypeToMote(tt.labels)
			if moteType != tt.wantType {
				t.Errorf("type = %q, want %q", moteType, tt.wantType)
			}
			if origin != tt.wantOrigin {
				t.Errorf("origin = %q, want %q", origin, tt.wantOrigin)
			}
		})
	}
}

func TestGithubPriorityToWeight(t *testing.T) {
	tests := []struct {
		name   string
		labels []githubLabel
		want   float64
	}{
		{"critical", []githubLabel{{Name: "priority:critical"}}, 1.0},
		{"high", []githubLabel{{Name: "priority:high"}}, 0.8},
		{"medium", []githubLabel{{Name: "priority:medium"}}, 0.5},
		{"no priority", []githubLabel{{Name: "verification"}}, 0.5},
		{"empty", nil, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := githubPriorityToWeight(tt.labels)
			if got != tt.want {
				t.Errorf("weight = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGithubLabelsToTags(t *testing.T) {
	labels := []githubLabel{
		{Name: "verification"},
		{Name: "security"},
		{Name: "complete"},
		{Name: "in progress"},
		{Name: "priority:high"},
		{Name: "phase:1-mvp"},
	}
	tags := githubLabelsToTags(labels)

	// Should include domain + phase (sanitized), exclude status + priority
	expected := map[string]bool{
		"verification": true,
		"security":     true,
		"phase-1-mvp":  true,
	}
	if len(tags) != len(expected) {
		t.Fatalf("got %d tags %v, want %d", len(tags), tags, len(expected))
	}
	for _, tag := range tags {
		if !expected[tag] {
			t.Errorf("unexpected tag %q", tag)
		}
	}
}

func TestParseAcceptanceCriteria(t *testing.T) {
	body := `## Requirements
- [ ] First unchecked item
- [x] Second checked item
- [ ] Third unchecked item
Some other text
- [X] Fourth checked (uppercase X)
`
	criteria, met := parseAcceptanceCriteria(body)
	if len(criteria) != 4 {
		t.Fatalf("got %d criteria, want 4: %v", len(criteria), criteria)
	}

	wantCriteria := []string{"First unchecked item", "Second checked item", "Third unchecked item", "Fourth checked (uppercase X)"}
	wantMet := []bool{false, true, false, true}

	for i := range criteria {
		if criteria[i] != wantCriteria[i] {
			t.Errorf("criteria[%d] = %q, want %q", i, criteria[i], wantCriteria[i])
		}
		if met[i] != wantMet[i] {
			t.Errorf("met[%d] = %v, want %v", i, met[i], wantMet[i])
		}
	}
}

func TestParseAcceptanceCriteria_NoChecklist(t *testing.T) {
	criteria, met := parseAcceptanceCriteria("Just some text without checklists")
	if len(criteria) != 0 || len(met) != 0 {
		t.Errorf("expected empty results, got %v / %v", criteria, met)
	}
}

func TestGithubSourceIssueKey(t *testing.T) {
	key := githubSourceIssueKey("owner/repo", "42")
	if key != "github:owner/repo#42" {
		t.Errorf("key = %q, want %q", key, "github:owner/repo#42")
	}
}

func TestDetectPhases(t *testing.T) {
	issues := []githubIssue{
		{Labels: []githubLabel{{Name: "phase:2-enhanced"}, {Name: "verification"}}},
		{Labels: []githubLabel{{Name: "phase:1-mvp"}}},
		{Labels: []githubLabel{{Name: "phase:3-scale"}}},
		{Labels: []githubLabel{{Name: "phase:1-mvp"}}}, // duplicate
		{Labels: []githubLabel{{Name: "security"}}},     // no phase
	}
	phases := detectPhases(issues)
	if len(phases) != 3 {
		t.Fatalf("got %d phases, want 3: %v", len(phases), phases)
	}
	// Should be sorted
	want := []string{"phase:1-mvp", "phase:2-enhanced", "phase:3-scale"}
	for i, p := range phases {
		if p != want[i] {
			t.Errorf("phases[%d] = %q, want %q", i, p, want[i])
		}
	}
}

func TestPhaseTitle(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"phase:1-mvp", "Phase 1: MVP"},
		{"phase:2-enhanced", "Phase 2: ENHANCED"},
		{"phase:3-scale", "Phase 3: SCALE"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := phaseTitle(tt.label)
			if got != tt.want {
				t.Errorf("phaseTitle(%q) = %q, want %q", tt.label, got, tt.want)
			}
		})
	}
}

func TestHasLabel(t *testing.T) {
	labels := []githubLabel{{Name: "bug"}, {Name: "security"}}
	if !hasLabel(labels, "bug") {
		t.Error("expected hasLabel(bug) = true")
	}
	if hasLabel(labels, "feature") {
		t.Error("expected hasLabel(feature) = false")
	}
}

func TestIsPhaseLabel(t *testing.T) {
	if !isPhaseLabel("phase:1-mvp") {
		t.Error("expected true for phase:1-mvp")
	}
	if isPhaseLabel("verification") {
		t.Error("expected false for verification")
	}
}

func TestIsStatusLabel(t *testing.T) {
	if !isStatusLabel("complete") {
		t.Error("expected true for complete")
	}
	if !isStatusLabel("in progress") {
		t.Error("expected true for 'in progress'")
	}
	if isStatusLabel("bug") {
		t.Error("expected false for bug")
	}
}

func TestIsPriorityLabel(t *testing.T) {
	if !isPriorityLabel("priority:high") {
		t.Error("expected true for priority:high")
	}
	if isPriorityLabel("security") {
		t.Error("expected false for security")
	}
}

func TestSanitizeTag(t *testing.T) {
	if got := sanitizeTag("phase:1-mvp"); got != "phase-1-mvp" {
		t.Errorf("got %q, want %q", got, "phase-1-mvp")
	}
	if got := sanitizeTag("In Progress"); got != "in-progress" {
		t.Errorf("got %q, want %q", got, "in-progress")
	}
}

// --- Integration tests ---

func TestGithubImport_Idempotency(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	repo := "test/repo"

	// Pre-create a mote with SourceIssue matching issue #1
	_, err := mm.Create("task", "Existing issue", core.CreateOpts{
		SourceIssue: githubSourceIssueKey(repo, "1"),
	})
	if err != nil {
		t.Fatal(err)
	}

	issues := []githubIssue{
		{Number: 1, Title: "Already imported", State: "OPEN", Labels: nil, Body: "body1"},
		{Number: 2, Title: "New issue", State: "OPEN", Labels: nil, Body: "body2"},
	}

	existing := buildSourceIssueSet(mm)
	created, skipped, errCount := importGithubIssues(mm, im, repo, issues, existing)

	if created != 1 {
		t.Errorf("created = %d, want 1", created)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if errCount != 0 {
		t.Errorf("errCount = %d, want 0", errCount)
	}
}

func TestGithubImport_PhaseParents(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	// Enable phase parents
	ghImportPhaseParents = true
	defer func() { ghImportPhaseParents = false }()

	repo := "test/repo"
	issues := []githubIssue{
		{Number: 1, Title: "MVP task", State: "OPEN", Labels: []githubLabel{{Name: "phase:1-mvp"}}},
		{Number: 2, Title: "Enhanced task", State: "OPEN", Labels: []githubLabel{{Name: "phase:2-enhanced"}}},
		{Number: 3, Title: "Scale task", State: "OPEN", Labels: []githubLabel{{Name: "phase:3-scale"}}},
	}

	existing := buildSourceIssueSet(mm)
	created, _, _ := importGithubIssues(mm, im, repo, issues, existing)

	// 3 phase parents + 3 issues = 6 created
	if created != 6 {
		t.Errorf("created = %d, want 6", created)
	}

	// Verify phase parents exist and are chained
	allMotes, _ := mm.ReadAllParallel()
	var phaseParents []*core.Mote
	for _, m := range allMotes {
		if m.Size == "l" && m.SourceIssue != "" && strings.Contains(m.SourceIssue, "phase:") {
			phaseParents = append(phaseParents, m)
		}
	}
	if len(phaseParents) != 3 {
		t.Fatalf("got %d phase parents, want 3", len(phaseParents))
	}

	// Verify child issues have parents set
	for _, m := range allMotes {
		if m.Title == "MVP task" && m.Parent == "" {
			t.Error("MVP task should have a parent")
		}
	}
}

func TestGithubImport_CompletedStatus(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	ghImportIncludeClosed = true
	defer func() { ghImportIncludeClosed = false }()

	repo := "test/repo"
	issues := []githubIssue{
		{Number: 1, Title: "Closed issue", State: "CLOSED", Labels: nil, Body: "done"},
		{Number: 2, Title: "Complete label", State: "OPEN", Labels: []githubLabel{{Name: "complete"}}, Body: "also done"},
		{Number: 3, Title: "Active issue", State: "OPEN", Labels: nil, Body: "wip"},
	}

	existing := buildSourceIssueSet(mm)
	importGithubIssues(mm, im, repo, issues, existing)

	allMotes, _ := mm.ReadAllParallel()
	for _, m := range allMotes {
		switch m.Title {
		case "Closed issue":
			if m.Status != "completed" {
				t.Errorf("Closed issue status = %q, want completed", m.Status)
			}
		case "Complete label":
			if m.Status != "completed" {
				t.Errorf("Complete label status = %q, want completed", m.Status)
			}
		case "Active issue":
			if m.Status != "active" {
				t.Errorf("Active issue status = %q, want active", m.Status)
			}
		}
	}
}

func TestGithubImport_AcceptanceCriteria(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	repo := "test/repo"
	issues := []githubIssue{
		{
			Number: 1,
			Title:  "Issue with checklist",
			State:  "OPEN",
			Labels: nil,
			Body:   "## Tasks\n- [ ] Do thing A\n- [x] Do thing B\n- [ ] Do thing C",
		},
	}

	existing := buildSourceIssueSet(mm)
	importGithubIssues(mm, im, repo, issues, existing)

	allMotes, _ := mm.ReadAllParallel()
	var found *core.Mote
	for _, m := range allMotes {
		if m.Title == "Issue with checklist" {
			found = m
			break
		}
	}
	if found == nil {
		t.Fatal("imported mote not found")
	}

	if len(found.Acceptance) != 3 {
		t.Fatalf("got %d acceptance criteria, want 3", len(found.Acceptance))
	}
	if found.Acceptance[0] != "Do thing A" {
		t.Errorf("acceptance[0] = %q, want %q", found.Acceptance[0], "Do thing A")
	}
	if len(found.AcceptanceMet) != 3 {
		t.Fatalf("got %d acceptance_met, want 3", len(found.AcceptanceMet))
	}
	if found.AcceptanceMet[0] != false || found.AcceptanceMet[1] != true || found.AcceptanceMet[2] != false {
		t.Errorf("acceptance_met = %v, want [false, true, false]", found.AcceptanceMet)
	}
}
