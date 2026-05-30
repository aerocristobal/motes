// SPDX-License-Identifier: MIT
// STORY-ADAPRIME-001 — MCP-mode renderer + token budget.
package prime_test

import (
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/prime"
)

// Scenario 1 — MCP-mode payload must include the truncation directive,
// the memories section, and the MCP notice line, but NOT any CLI-mode
// section markers (no "## Active work", "## Ready to start", etc.).
func TestRenderMCPModeText_IncludesDirectiveMemoriesAndNotice(t *testing.T) {
	memories := []core.MemoryRecord{
		{Key: "test-flag", Body: "use -race"},
		{Key: "fmt", Body: "always gofmt"},
	}
	got := string(prime.RenderMCPModeText(memories, ""))

	if !strings.Contains(got, prime.TruncationDirective) {
		t.Errorf("expected TruncationDirective in MCP-mode output:\n%s", got)
	}
	if !strings.Contains(got, "## Persistent memories") {
		t.Errorf("expected '## Persistent memories' section:\n%s", got)
	}
	if !strings.Contains(got, "test-flag") || !strings.Contains(got, "use -race") {
		t.Errorf("memories not rendered:\n%s", got)
	}
	if !strings.Contains(got, prime.MCPNoticeLine) {
		t.Errorf("expected MCPNoticeLine in MCP-mode output:\n%s", got)
	}

	// Scenario 1: MUST NOT contain any CLI-mode section markers.
	bannedMarkers := []string{
		"## Active work",
		"## Ready to start",
		"## Relevant decisions",
		"## Key lessons",
		"## Prior explorations",
		"## Code context",
		"## Available strata",
	}
	for _, banned := range bannedMarkers {
		if strings.Contains(got, banned) {
			t.Errorf("MCP-mode output must not contain %q; got:\n%s", banned, got)
		}
	}
}

// Scenario 1 (boundary) — token count of a representative payload stays
// within the documented budget. Uses small but realistic content; user
// memory sets that exceed this budget are not enforced at runtime, only
// documented in docs/agents-guide.md.
func TestRenderMCPModeText_StaysWithinTokenBudget(t *testing.T) {
	memories := []core.MemoryRecord{
		{Key: "race", Body: "use -race"},
		{Key: "fmt", Body: "always gofmt"},
	}
	out := prime.RenderMCPModeText(memories, "")
	tokens := prime.EstimateTokens(string(out))
	if tokens > prime.MCPModeTokenBudget {
		t.Errorf("MCP-mode output exceeds budget: %d tokens > %d budget\noutput:\n%s",
			tokens, prime.MCPModeTokenBudget, string(out))
	}
}

// Empty memory store: section is omitted but directive + notice still emit.
func TestRenderMCPModeText_NoMemories(t *testing.T) {
	out := string(prime.RenderMCPModeText(nil, ""))
	if !strings.Contains(out, prime.TruncationDirective) {
		t.Errorf("directive missing:\n%s", out)
	}
	if strings.Contains(out, "## Persistent memories") {
		t.Errorf("expected no memories section when memories empty:\n%s", out)
	}
	if !strings.Contains(out, prime.MCPNoticeLine) {
		t.Errorf("notice line missing:\n%s", out)
	}
}

// STORY-PRIMEOVR-001 — when a prose preamble is supplied it is inserted
// between the truncation directive and the persistent-memories section,
// separated by a blank line on each side.
func TestRenderMCPModeText_InsertsProseBetweenDirectiveAndMemories(t *testing.T) {
	memories := []core.MemoryRecord{{Key: "k", Body: "v"}}
	got := string(prime.RenderMCPModeText(memories, "Project rules: run make test"))

	if !strings.Contains(got, "Project rules: run make test") {
		t.Errorf("prose preamble missing from output:\n%s", got)
	}
	dirIdx := strings.Index(got, prime.TruncationDirective)
	proseIdx := strings.Index(got, "Project rules:")
	memIdx := strings.Index(got, "## Persistent memories")
	if dirIdx >= proseIdx || proseIdx >= memIdx {
		t.Errorf("expected order: directive < prose < memories; got dirIdx=%d proseIdx=%d memIdx=%d", dirIdx, proseIdx, memIdx)
	}
}

// Sanity: the directive constant has not drifted (Sprint 1 baseline).
func TestTruncationDirective_IsExactConstant(t *testing.T) {
	want := "[mote prime] If this output is truncated by your host, " +
		"read the full persisted output at .memory/last_prime.txt before continuing; " +
		"it may contain project memories and session rules not visible in the preview."
	if prime.TruncationDirective != want {
		t.Errorf("TruncationDirective drift\n got:  %q\n want: %q", prime.TruncationDirective, want)
	}
}

// EstimateTokens is the documented heuristic (wordCount * 1.3); regress
// here so the budget assertions above remain meaningful.
func TestEstimateTokens_HeuristicMatchesDocumentedFormula(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"one", 1},     // 1 word * 1.3 = 1.3, truncated to 1
		{"one two", 2}, // 2 * 1.3 = 2.6, truncated to 2
		{"one two three four five six seven eight nine ten", 13}, // 10 * 1.3 = 13
	}
	for _, c := range cases {
		got := prime.EstimateTokens(c.input)
		if got != c.want {
			t.Errorf("EstimateTokens(%q) = %d; want %d", c.input, got, c.want)
		}
	}
}
