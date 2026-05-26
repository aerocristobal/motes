// SPDX-License-Identifier: MIT
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findRepoRoot walks upward from the test's CWD looking for go.mod. The
// per-test t.Chdir from setupIntegrationTest is not in use here, but the
// package's default CWD is cmd/mote/, so we walk up twice. We tolerate any
// depth in case tests are reorganized.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", cwd)
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	root := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// TestAgentsGuide_ContractSection asserts that docs/agents-guide.md contains
// the exact contract block required by STORY-EREAD-001. These are doc-drift
// guards: if a future edit removes any of these phrases, the test fails and
// the author has to acknowledge they're changing the contract.
func TestAgentsGuide_ContractSection(t *testing.T) {
	content := readRepoFile(t, "docs/agents-guide.md")

	if !strings.Contains(content, "Read execution metadata before prose") {
		t.Error("agents-guide.md must contain the section title 'Read execution metadata before prose'")
	}
	if !strings.Contains(content, "mote show <id> --execution-only | jq .") {
		t.Error("agents-guide.md must contain the example invocation 'mote show <id> --execution-only | jq .'")
	}
	if !strings.Contains(content, "a running subagent cannot change its model or reasoning effort after launch") {
		t.Error("agents-guide.md must state 'a running subagent cannot change its model or reasoning effort after launch'")
	}
	if !strings.Contains(content, "authoritative when present") {
		t.Error("agents-guide.md must state that execution metadata is 'authoritative when present'")
	}
}

// TestAgentsMD_ReferencesContract asserts AGENTS.md links to the contract
// using the exact form specified by STORY-EREAD-001 Scenario 7:
// "Read execution metadata before prose — see `docs/agents-guide.md#read-execution-metadata-before-prose`".
func TestAgentsMD_ReferencesContract(t *testing.T) {
	content := readRepoFile(t, "AGENTS.md")
	want := "Read execution metadata before prose — see `docs/agents-guide.md#read-execution-metadata-before-prose`"
	if !strings.Contains(content, want) {
		t.Errorf("AGENTS.md must contain the exact reference form:\n%s", want)
	}
}

// TestSubagentSkill_MirrorsContract asserts the mote-subagent skill restates
// the contract from the subagent's perspective.
func TestSubagentSkill_MirrorsContract(t *testing.T) {
	content := readRepoFile(t, "skills/mote-subagent/SKILL.md")
	// Case-insensitive — the contract phrase may legitimately appear with
	// sentence-start capitalization in prose.
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "your model and reasoning effort were chosen from execution metadata at dispatch time and cannot be changed") {
		t.Error("skills/mote-subagent/SKILL.md must state 'your model and reasoning effort were chosen from execution metadata at dispatch time and cannot be changed'")
	}
	if !strings.Contains(content, "docs/agents-guide.md#read-execution-metadata-before-prose") {
		t.Error("skills/mote-subagent/SKILL.md must reference the contract section")
	}
}

// TestVendorMDs_ReferenceContract asserts that the three vendor entry-point
// files each contain a one-line reference to the contract section.
func TestVendorMDs_ReferenceContract(t *testing.T) {
	for _, file := range []string{"CLAUDE.md", "CODEX.md", "GEMINI.md"} {
		t.Run(file, func(t *testing.T) {
			content := readRepoFile(t, file)
			if !strings.Contains(content, "docs/agents-guide.md#read-execution-metadata-before-prose") {
				t.Errorf("%s must reference docs/agents-guide.md#read-execution-metadata-before-prose", file)
			}
		})
	}
}

// TestContractDrift_NoBodyBeforeExecutionInstruction is the soft doc-drift
// check from STORY-EREAD-001 §2 ("No documentation page instructs an agent to
// read body BEFORE execution metadata"). Bounded to files where the contract
// is meaningful: AGENTS.md, vendor MDs, docs/agents-guide.md, and skills.
//
// Heuristic: in any single file, if we see an instruction phrased as "read
// the body / read description / read notes" precede an instruction phrased
// as "read execution metadata / inspect execution / --execution-only", that
// is a drift. The check is intentionally narrow — only the same-file relative
// ordering matters; cross-file ordering is not enforced.
func TestContractDrift_NoBodyBeforeExecutionInstruction(t *testing.T) {
	files := []string{
		"AGENTS.md",
		"CLAUDE.md",
		"CODEX.md",
		"GEMINI.md",
		"docs/agents-guide.md",
		"skills/mote-subagent/SKILL.md",
	}
	// Phrases that, in this corpus, would represent an "agent: read body first"
	// instruction. They are written narrowly to avoid catching benign mentions
	// (e.g. "the body is the work scope" is fine; "read the body first" is not).
	bodyFirstPhrases := []string{
		"read the body first",
		"read body first",
		"read description first",
		"read notes first",
		"inspect body before",
		"inspect description before",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content := strings.ToLower(readRepoFile(t, file))
			for _, phrase := range bodyFirstPhrases {
				if strings.Contains(content, phrase) {
					t.Errorf("%s contains body-first instruction %q — violates 'read execution metadata before prose'", file, phrase)
				}
			}
		})
	}
}
