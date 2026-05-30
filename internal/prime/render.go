// SPDX-License-Identifier: MIT
package prime

import (
	"fmt"
	"strings"

	"motes/internal/core"
	"motes/internal/format"
)

// TruncationDirective is prepended to every successful `mote prime` output
// (as a leading text line for text/hook paths, and as the truncation_notice
// field for --json). Agent hosts (Claude Code, Codex, Gemini CLI) silently
// truncate hook output for display; this line tells the agent there is more
// context on disk and where to find it.
//
// Originally lived in cmd/mote/cmd_prime.go (Sprint 1, STORY-BR-23-2). Moved
// here in STORY-ADAPRIME-001 so the new MCP-mode renderer can share it
// without a cycle between cmd/mote and internal/prime.
const TruncationDirective = "[mote prime] If this output is truncated by your host, " +
	"read the full persisted output at .memory/last_prime.txt before continuing; " +
	"it may contain project memories and session rules not visible in the preview."

// LastPrimeFilename is the basename of the persisted prime output (relative
// to .memory/). The cmd layer writes captured stdout here so agents whose
// host preview is truncated can read the full body back via this path,
// pointed at by TruncationDirective.
const LastPrimeFilename = "last_prime.txt"

// MCPNoticeLine closes the brief MCP-mode payload. It points the agent at
// the canonical tool names exposed by the (future) mote MCP wrapper so
// the agent doesn't have to guess what's available without the full CLI
// command reference.
const MCPNoticeLine = "Use the mote MCP server tools (mote_ls, mote_search, mote_show) for full context."

// MCPModeTokenBudget is the documented soft upper bound on tokens emitted
// by the MCP-mode prime payload (STORY-ADAPRIME-001 Q3). Enforced as a
// unit-test assertion against a representative fixture; not enforced at
// runtime because user memories can vary arbitrarily.
const MCPModeTokenBudget = 75

// CLIModeTokenBudget is the documented soft upper bound for the full
// CLI-mode payload. Surfaced in docs/agents-guide.md so operators wiring
// `mote prime` into a hook context know roughly how much preview budget
// to allocate. Not enforced as a test assertion — the CLI-mode pipeline
// is the established baseline (see docs/version-history.md v0.4.22).
const CLIModeTokenBudget = 2500

// EstimateTokens approximates token count as wordCount * 1.3.
//
// Identical to internal/strata/tokenizer.go's estimateTokens (unexported
// there). Duplicated rather than depended-on to keep internal/prime free
// of strata-specific imports. If we ever swap to a real tokenizer it
// should be a one-line change here.
func EstimateTokens(text string) int {
	return int(float64(len(strings.Fields(text))) * 1.3)
}

// RenderMCPModeText builds the brief MCP-mode payload as a text body:
//
//	[TruncationDirective]
//
//	<prose preamble — only when non-empty>
//
//	## Persistent memories
//
//	  key1   body1
//	  key2   body2
//
//	[MCPNoticeLine]
//
// The prose argument carries an optional preamble — typically the
// resolved PRIME.md override (STORY-PRIMEOVR-001). Pass "" when no
// override applies; the empty case is treated identically to the
// pre-story payload (no extra blank lines, no marker).
//
// The memories section is omitted entirely when there are no memories,
// mirroring the CLI-mode renderer's behaviour. Caller should pin the
// total under MCPModeTokenBudget for the typical memory set; large
// memory sets — and large prose preambles — may exceed the budget and
// the docs flag this.
func RenderMCPModeText(memories []core.MemoryRecord, prose string) []byte {
	var b strings.Builder
	b.WriteString(TruncationDirective)
	b.WriteString("\n\n")
	if prose != "" {
		b.WriteString(prose)
		b.WriteString("\n\n")
	}
	if len(memories) > 0 {
		b.WriteString("## Persistent memories\n\n")
		keyWidth := 0
		for _, m := range memories {
			if len(m.Key) > keyWidth {
				keyWidth = len(m.Key)
			}
		}
		for _, m := range memories {
			fmt.Fprintf(&b, "  %-*s  %s\n", keyWidth, m.Key, format.Truncate(m.Body, 200))
		}
		b.WriteString("\n")
	}
	b.WriteString(MCPNoticeLine)
	b.WriteString("\n")
	return []byte(b.String())
}
