// SPDX-License-Identifier: MIT
// STORY-DIVRG-001 — instruction-doc reconciliation

package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

// writeFiles writes the map of relative-path -> content into a fresh
// t.TempDir() and returns the directory.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func containsString(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

// --- Scenario 1: All four docs agree ---

func Test_CheckInstructionDocs_AllAgree_ReturnsOK(t *testing.T) {
	section := "## Landing the Plane\n\nstep 1\nstep 2\n"
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n" + section,
		"AGENTS.md": "# a\n" + section,
		"CODEX.md":  "# x\n" + section,
		"GEMINI.md": "# g\n" + section,
	})
	cfg := core.InstructionDocsConfig{
		SharedSections: []string{"## Landing the Plane"},
	}
	verbose, err := CheckInstructionDocs(root, cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !containsString(verbose, "CLAUDE.md ## Landing the Plane: authoritative") {
		t.Errorf("verbose missing authoritative line: %v", verbose)
	}
	if !containsString(verbose, "AGENTS.md ## Landing the Plane: matches authoritative") {
		t.Errorf("verbose missing match line: %v", verbose)
	}
}

// --- Scenario 2: Marker silences divergence ---

func Test_CheckInstructionDocs_MarkerSilencesDivergence(t *testing.T) {
	base := "## Build & Development Commands\n\nbase content\n"
	diverged := "## Build & Development Commands\n" + DivergenceMarker + "\n\ndifferent content\n"
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n" + base,
		"AGENTS.md": "# a\n" + base,
		"CODEX.md":  "# x\n" + base,
		"GEMINI.md": "# g\n" + diverged,
	})
	cfg := core.InstructionDocsConfig{
		SharedSections: []string{"## Build & Development Commands"},
	}
	verbose, err := CheckInstructionDocs(root, cfg)
	if err != nil {
		t.Fatalf("expected marker to silence drift, got %v", err)
	}
	wantLine := "GEMINI.md ## Build & Development Commands: divergence-ok (marker present)"
	if !containsString(verbose, wantLine) {
		t.Errorf("verbose missing marker line, got %v", verbose)
	}
}

// --- Scenario 3: No config is skipped ---

func Test_CheckInstructionDocs_NoConfig_IsSkipped(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n## Whatever\n",
		"AGENTS.md": "# a\n",
		"CODEX.md":  "# x\n",
		"GEMINI.md": "# g\n",
	})
	cfg := core.InstructionDocsConfig{}
	_, err := CheckInstructionDocs(root, cfg)
	if !errors.Is(err, ErrSkipped) {
		t.Fatalf("expected ErrSkipped, got %v", err)
	}
}

// --- Scenario 4: Drift without marker returns DriftError ---

func Test_CheckInstructionDocs_DriftWithoutMarker_ReturnsDriftError(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n## Landing the Plane\nstep 1\nstep 2\nstep 3\n",
		"AGENTS.md": "# a\n## Landing the Plane\nstep 1\nstep 2\n",
		"CODEX.md":  "# x\n## Landing the Plane\nstep 1\nstep 2\nstep 3\n",
		"GEMINI.md": "# g\n## Landing the Plane\nstep 1\nstep 2\nstep 3\n",
	})
	cfg := core.InstructionDocsConfig{
		SharedSections:    []string{"## Landing the Plane"},
		AuthoritativeFile: "CLAUDE.md",
	}
	_, err := CheckInstructionDocs(root, cfg)
	var drift *DriftError
	if !errors.As(err, &drift) {
		t.Fatalf("expected *DriftError, got %T: %v", err, err)
	}
	if drift.Section != "## Landing the Plane" {
		t.Errorf("wrong section: %q", drift.Section)
	}
	if drift.AuthoritativeFile != "CLAUDE.md" {
		t.Errorf("wrong authoritative: %q", drift.AuthoritativeFile)
	}
	if !containsString(drift.DivergedFiles, "AGENTS.md") {
		t.Errorf("AGENTS.md not in diverged list: %v", drift.DivergedFiles)
	}
	// Error message contains the key cues from Scenario 4.
	msg := err.Error()
	for _, want := range []string{"instruction-doc drift detected", "## Landing the Plane", "CLAUDE.md", "AGENTS.md", DivergenceMarker} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}

// --- Scenario 5: Fix performs a surgical rewrite ---

func Test_FixInstructionDocs_CopiesSection_SurgicalRewrite(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n## Landing the Plane\nfresh\n\n## Other Section\nclaude-other\n",
		"AGENTS.md": "# header\n\n## Landing the Plane\nstale\n\n## Other Section\nuntouched\n",
		"CODEX.md":  "# x\n## Landing the Plane\nfresh\n",
		"GEMINI.md": "# g\n## Landing the Plane\nfresh\n",
	})
	cfg := core.InstructionDocsConfig{
		SharedSections:    []string{"## Landing the Plane"},
		AuthoritativeFile: "CLAUDE.md",
	}
	verbose, err := FixInstructionDocs(root, cfg)
	if err != nil {
		t.Fatalf("fix returned error: %v", err)
	}
	got := mustReadFile(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(got, "## Landing the Plane\nfresh") {
		t.Fatalf("section was not rewritten:\n%s", got)
	}
	if !strings.Contains(got, "## Other Section\nuntouched") {
		t.Fatalf("surgical rewrite contract violated:\n%s", got)
	}
	// AGENTS.md original header preserved.
	if !strings.HasPrefix(got, "# header\n\n") {
		t.Fatalf("file prefix changed:\n%s", got)
	}
	// Verbose log names the file and approximate byte count.
	foundLog := false
	for _, line := range verbose {
		if strings.Contains(line, "AGENTS.md") && strings.Contains(line, "rewrote") {
			foundLog = true
			break
		}
	}
	if !foundLog {
		t.Errorf("verbose log missing rewrite for AGENTS.md: %v", verbose)
	}

	// Second run is a no-op (idempotency).
	beforeSecond := mustReadFile(t, filepath.Join(root, "AGENTS.md"))
	if _, err := FixInstructionDocs(root, cfg); err != nil {
		t.Fatalf("second fix returned error: %v", err)
	}
	afterSecond := mustReadFile(t, filepath.Join(root, "AGENTS.md"))
	if beforeSecond != afterSecond {
		t.Errorf("second fix was not a no-op:\nbefore:\n%s\nafter:\n%s", beforeSecond, afterSecond)
	}
}

// --- Scenario 6: Fix skips marked section ---

func Test_FixInstructionDocs_SkipsMarkedSection(t *testing.T) {
	geminiOriginal := "# g\n## Build & Development Commands\n" + DivergenceMarker + "\n\ngemini-only timeout note\n"
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n## Build & Development Commands\ncanonical\n",
		"AGENTS.md": "# a\n## Build & Development Commands\ncanonical\n",
		"CODEX.md":  "# x\n## Build & Development Commands\ncanonical\n",
		"GEMINI.md": geminiOriginal,
	})
	cfg := core.InstructionDocsConfig{
		SharedSections:    []string{"## Build & Development Commands"},
		AuthoritativeFile: "CLAUDE.md",
	}
	verbose, err := FixInstructionDocs(root, cfg)
	if err != nil {
		t.Fatalf("fix returned error: %v", err)
	}
	got := mustReadFile(t, filepath.Join(root, "GEMINI.md"))
	if got != geminiOriginal {
		t.Fatalf("marked file should not be modified.\nwant: %q\ngot:  %q", geminiOriginal, got)
	}
	foundLog := false
	for _, line := range verbose {
		if strings.Contains(line, "GEMINI.md") && strings.Contains(line, "marker present") {
			foundLog = true
			break
		}
	}
	if !foundLog {
		t.Errorf("verbose log missing marker-skip line: %v", verbose)
	}
}

// --- Scenario 7: Missing section in some peers ---

func Test_CheckInstructionDocs_MissingFromOthers_IsTreatedAsDrift(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n## Active Task Enforcement\ndo the thing\n",
		"AGENTS.md": "# a\n",
		"CODEX.md":  "# x\n",
		"GEMINI.md": "# g\n",
	})
	cfg := core.InstructionDocsConfig{
		SharedSections:    []string{"## Active Task Enforcement"},
		AuthoritativeFile: "CLAUDE.md",
	}
	_, err := CheckInstructionDocs(root, cfg)
	var missing *MissingSectionError
	if !errors.As(err, &missing) {
		t.Fatalf("expected *MissingSectionError, got %T: %v", err, err)
	}
	for _, want := range []string{"AGENTS.md", "CODEX.md", "GEMINI.md"} {
		if !containsString(missing.MissingFrom, want) {
			t.Errorf("missing file not reported: %v", missing.MissingFrom)
		}
	}
}

// --- Scenario 7 (fix): Missing sections are appended ---

func Test_FixInstructionDocs_MissingSection_IsAppended(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n## Active Task Enforcement\ndo the thing\n",
		"AGENTS.md": "# a\nfoo\n",
		"CODEX.md":  "# x\nbar\n",
		"GEMINI.md": "# g\nbaz\n",
	})
	cfg := core.InstructionDocsConfig{
		SharedSections:    []string{"## Active Task Enforcement"},
		AuthoritativeFile: "CLAUDE.md",
	}
	if _, err := FixInstructionDocs(root, cfg); err != nil {
		t.Fatalf("fix returned error: %v", err)
	}
	agents := mustReadFile(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(agents, "## Active Task Enforcement\ndo the thing") {
		t.Fatalf("section not appended to AGENTS.md:\n%s", agents)
	}
	if !strings.HasPrefix(agents, "# a\nfoo\n") {
		t.Fatalf("existing content of AGENTS.md was disturbed:\n%s", agents)
	}
	// Blank-line separator between prior content and new section.
	if !strings.Contains(agents, "foo\n\n## Active Task Enforcement") {
		t.Fatalf("blank-line separator missing:\n%q", agents)
	}
}

// --- Gemini @AGENTS.md import exclusion ---

func Test_CheckInstructionDocs_GeminiImportsAgents_NotDrift(t *testing.T) {
	section := "## Workflow contract\nshared rules\n"
	gemini := "# g\n\n@AGENTS.md\n\n## Gemini specifics\nlocal-only\n"
	root := writeFiles(t, map[string]string{
		"CLAUDE.md": "# c\n" + section,
		"AGENTS.md": "# a\n" + section,
		"CODEX.md":  "# x\n" + section,
		"GEMINI.md": gemini,
	})
	cfg := core.InstructionDocsConfig{
		SharedSections:    []string{"## Workflow contract"},
		AuthoritativeFile: "CLAUDE.md",
	}
	verbose, err := CheckInstructionDocs(root, cfg)
	if err != nil {
		t.Fatalf("expected GEMINI.md @AGENTS.md import to silence drift, got %v", err)
	}
	wantLine := "GEMINI.md ## Workflow contract: imported via @AGENTS.md"
	if !containsString(verbose, wantLine) {
		t.Errorf("verbose missing import-exclusion line, got %v", verbose)
	}
}

// --- Parser unit test ---

func Test_findSectionRange(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		heading   string
		want      string
		wantFound bool
	}{
		{
			name:      "single section, EOF body",
			body:      "## Heading\ncontent\n",
			heading:   "## Heading",
			want:      "## Heading\ncontent\n",
			wantFound: true,
		},
		{
			name:      "two sections, returns first only",
			body:      "## A\nbody A\n\n## B\nbody B\n",
			heading:   "## A",
			want:      "## A\nbody A\n\n",
			wantFound: true,
		},
		{
			name:      "second of two sections",
			body:      "## A\nbody A\n\n## B\nbody B\n",
			heading:   "## B",
			want:      "## B\nbody B\n",
			wantFound: true,
		},
		{
			name:      "absent heading",
			body:      "## Other\nbody\n",
			heading:   "## Missing",
			want:      "",
			wantFound: false,
		},
		{
			name:      "heading at EOF without trailing newline",
			body:      "## Final\nlast line",
			heading:   "## Final",
			want:      "## Final\nlast line",
			wantFound: true,
		},
		{
			name:      "H3 nested under H2 stays in H2",
			body:      "## H2\nintro\n### H3\nsub\n\n## Next\nx\n",
			heading:   "## H2",
			want:      "## H2\nintro\n### H3\nsub\n\n",
			wantFound: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := findSectionRange([]byte(tc.body), tc.heading)
			if ok != tc.wantFound {
				t.Fatalf("found = %v, want %v", ok, tc.wantFound)
			}
			if !ok {
				return
			}
			got := tc.body[start:end]
			if got != tc.want {
				t.Errorf("section mismatch:\nwant: %q\ngot:  %q", tc.want, got)
			}
		})
	}
}

func Test_hasAgentsImport(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{"@AGENTS.md\n", true},
		{"hello\n@AGENTS.md\nworld\n", true},
		{"  @AGENTS.md  \n", true},
		{"text @AGENTS.md text\n", false},
		{"# header\n\nno import here\n", false},
	}
	for _, tc := range cases {
		got := hasAgentsImport([]byte(tc.body))
		if got != tc.want {
			t.Errorf("hasAgentsImport(%q) = %v, want %v", tc.body, got, tc.want)
		}
	}
}
