// SPDX-License-Identifier: MIT
//
// Package docflags is the pure-logic layer of STORY-DOCFLAGS-001 — the
// CI gate that fails the build when any documentation file references a
// CLI flag that does not exist in the live Cobra tree.
//
// The package has no Cobra dependency: the cmd/mote layer walks the
// real Cobra tree, lowers it into a CLISurface, and hands it to Check.
package docflags

// CLISurface is the per-build snapshot of the mote CLI: every command's
// local-plus-own-persistent flags, plus the root persistent flags
// collected once. Inherited intermediate persistents are NOT recorded
// here because no intermediate-level persistent flags exist in the tree
// today; if they are added later, the walker grows by one Visit call.
type CLISurface struct {
	Commands   []CommandSpec `json:"commands"`
	Persistent []FlagSpec    `json:"persistent_flags"`
}

// CommandSpec records one command's accepted flag set. Name is the
// full command path joined by spaces ("ls", "compliance export").
type CommandSpec struct {
	Name  string     `json:"name"`
	Flags []FlagSpec `json:"flags"`
}

// FlagSpec records one flag. Deprecated mirrors Cobra's Deprecated
// string being non-empty; deprecated flags are still valid references
// (Cobra still accepts them), they just print a notice.
type FlagSpec struct {
	Name       string `json:"name"`
	Deprecated bool   `json:"deprecated"`
}

// Reference is one extracted `mote <cmd...> --<flag>` mention located
// at Path:Line. Command is the literal token sequence between `mote`
// and the first --flag on that line; the checker resolves it against
// the surface via longest-prefix matching to handle positional args.
type Reference struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Command string `json:"command"`
	Flag    string `json:"flag"`
}

// Violation is one bad Reference plus a human-readable reason that
// names what was wrong (unknown command, unknown flag for command,
// removed flag outside an allowlisted context).
type Violation struct {
	Reference Reference `json:"reference"`
	Reason    string    `json:"reason"`
}

// DefaultTokens is the case-insensitive allowlist token set: a flag
// reference whose nearest H2 heading OR enclosing paragraph contains
// any of these substrings is exempt from the removed-flag check.
// Confirmed during STORY-DOCFLAGS-001 grooming.
var DefaultTokens = []string{"changelog", "removed", "deprecated", "historical"}

// SuppressionMarker is the inline HTML-comment directive that
// suppresses doc-flag checking for the next non-blank line, or for
// the entire next fenced code block if that line opens one.
const SuppressionMarker = "<!-- doc-flags: ignore-next -->"
