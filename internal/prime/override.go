// SPDX-License-Identifier: MIT
// STORY-PRIMEOVR-001 — Customizable PRIME.md override with three-tier
// resolution + `mote prime --export`.
//
// This file owns the prose-preamble override resolved at session start
// and the hand-crafted starter template emitted by `mote prime --export`.
// The override is *prose* (replaces nothing else); data sections
// (memories, ready tasks, decisions, lessons, ...) always render below it.
package prime

import (
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// Tier identifies which of the three resolution slots produced a result.
//
// Resolution order, highest-priority first:
//
//	TierClone        ← <memoryRoot>/PRIME.md            (per-developer; gitignore by convention)
//	TierWorkspace    ← <filepath.Dir(memoryRoot)>/PRIME.md  (committed)
//	TierUserGlobal   ← <homeDir>/.motes/PRIME.md        (applies to every project this user opens)
//
// Tier 1 lives *inside* `.memory/` (alongside other clone-specific state
// like `last_prime.txt` and `.access_batch.jsonl`); tier 2 lives one
// directory up (the resolved workspace root). Tier 3 mirrors the
// existing `~/.motes/` global-layer convention rather than XDG (decision
// locked at story interview).
type Tier int

const (
	TierNone Tier = iota
	TierClone
	TierWorkspace
	TierUserGlobal
)

// String renders the tier as a human-readable label used in --debug
// messages, e.g. "tier-clone (.memory/PRIME.md)". Intentionally short.
func (t Tier) String() string {
	switch t {
	case TierClone:
		return "tier-clone"
	case TierWorkspace:
		return "tier-workspace"
	case TierUserGlobal:
		return "tier-user-global"
	default:
		return "tier-none"
	}
}

// PrimeMdFilename is the basename mote looks for at each tier.
const PrimeMdFilename = "PRIME.md"

// PrimeMdMaxBytes is the hard cap on bytes read from a PRIME.md file.
// Files larger than this are truncated and a footer marker is appended
// to the returned content (see LoadPrimeMd). 16 KiB is generous for a
// project-rules preamble and well within typical agent-host preview
// budgets even when concatenated with the rest of the prime body.
const PrimeMdMaxBytes = 16 * 1024

// utf8BOM is the 3-byte UTF-8 byte-order mark stripped from PRIME.md
// contents if present. Some editors (notably Windows Notepad) prepend
// it; leaving it in would corrupt the first rune of the prose body.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ResolveResult is what ResolveOverride returns when *any* tier produces
// a usable PRIME.md. A nil pointer means no tier produced a result —
// the baked-in default (i.e., no prose preamble) should render.
//
// Content has already been BOM-stripped, UTF-8-validated, and size-
// truncated (with the truncation marker appended); the caller renders
// it verbatim.
type ResolveResult struct {
	Tier          Tier
	Path          string
	Content       string
	OriginalSize  int      // bytes on disk before any truncation
	TruncatedAt   int      // 0 when Content was not truncated
	DebugMessages []string // per-tier tried-and-skipped reasons (surfaced only under --debug)
}

// ResolveOverride walks the three tiers in priority order and returns
// the first usable PRIME.md, or nil if none resolved.
//
// memoryRoot is the resolved `.memory/` directory (i.e., the value
// returned by findMemoryRoot in cmd/mote/helpers.go). The workspace
// root used for tier 2 is filepath.Dir(memoryRoot).
//
// homeDir is the user's HOME directory; passing "" disables tier 3.
//
// Tiers are stop-at-first-hit, NOT merge. A tier that *exists* but
// fails to load (read error, non-UTF-8, broken symlink) records a
// debug message and falls through to the next tier — mirroring the
// Sprint 1 silent-failure policy (STORY-BR-23-4).
func ResolveOverride(memoryRoot, homeDir string) *ResolveResult {
	candidates := []struct {
		tier Tier
		path string
	}{
		{TierClone, filepath.Join(memoryRoot, PrimeMdFilename)},
		{TierWorkspace, filepath.Join(filepath.Dir(memoryRoot), PrimeMdFilename)},
	}
	if homeDir != "" {
		candidates = append(candidates, struct {
			tier Tier
			path string
		}{TierUserGlobal, filepath.Join(homeDir, ".motes", PrimeMdFilename)})
	}

	var debugMsgs []string
	for _, c := range candidates {
		// Stat first so a missing tier doesn't pollute debug output.
		// Lstat (not Stat) so we *see* a broken symlink as "exists but
		// won't read", and report it as a failed load — fall through is
		// the correct behavior regardless.
		if _, err := os.Lstat(c.path); err != nil {
			if !os.IsNotExist(err) {
				debugMsgs = append(debugMsgs, fmt.Sprintf("%s stat %s: %v", c.tier, c.path, err))
			}
			continue
		}
		content, originalSize, truncatedAt, err := LoadPrimeMd(c.path)
		if err != nil {
			debugMsgs = append(debugMsgs, fmt.Sprintf("%s load %s: %v", c.tier, c.path, err))
			continue
		}
		return &ResolveResult{
			Tier:          c.tier,
			Path:          c.path,
			Content:       content,
			OriginalSize:  originalSize,
			TruncatedAt:   truncatedAt,
			DebugMessages: debugMsgs,
		}
	}
	return nil
}

// LoadPrimeMd reads one candidate PRIME.md file, strips a leading UTF-8
// BOM, validates UTF-8, and caps the body at PrimeMdMaxBytes (appending
// a truncation marker when the cap fires).
//
// Returns (content, originalSize, truncatedAt, err). On success err is
// nil even when the file was truncated — truncation is a documented
// operation, not an error. On any read or validation failure the
// returned content is empty and the caller should fall through to the
// next tier.
func LoadPrimeMd(path string) (string, int, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, 0, err
	}
	originalSize := len(data)

	// BOM strip — applied before UTF-8 validation so a BOM doesn't
	// trip the strict utf8.Valid check below.
	if len(data) >= len(utf8BOM) && data[0] == utf8BOM[0] && data[1] == utf8BOM[1] && data[2] == utf8BOM[2] {
		data = data[len(utf8BOM):]
	}

	if !utf8.Valid(data) {
		return "", originalSize, 0, fmt.Errorf("invalid utf-8")
	}

	truncatedAt := 0
	if len(data) > PrimeMdMaxBytes {
		data = data[:PrimeMdMaxBytes]
		truncatedAt = PrimeMdMaxBytes
	}

	content := string(data)
	if truncatedAt > 0 {
		marker := fmt.Sprintf("\n[PRIME.md truncated at %d bytes — see %s]", truncatedAt, path)
		content += marker
	}

	return content, originalSize, truncatedAt, nil
}

// DefaultExportTemplate returns the hand-crafted starter template
// emitted by `mote prime --export`. This is intentionally *not* the
// live prime body — it is a guidance document for users to customize
// and place at one of the three tiers.
//
// Keep this template short. Users will see it once (when they run
// --export), and noise in the template tends to survive forever as
// committed comment-cruft in their PRIME.md.
func DefaultExportTemplate() string {
	return `# Project rules for ` + "`mote prime`" + `

This file is emitted verbatim by ` + "`mote prime`" + ` between the truncation
directive and the data sections (persistent memories, active tasks,
decisions, lessons, ...). Use it to teach every agent that runs in this
project the rules they need to follow from session start.

` + "`mote prime`" + ` looks for ` + "`PRIME.md`" + ` at three locations, in order:

1. ` + "`./.memory/PRIME.md`" + `      — per-developer (gitignore by convention)
2. ` + "`<workspace>/PRIME.md`" + `    — per-project (committed)
3. ` + "`~/.motes/PRIME.md`" + `       — per-user (applies in every project)

The first file that exists wins; lower tiers do not contribute.

## Example rules

- Run ` + "`go test ./...`" + ` and ` + "`go vet ./...`" + ` before every commit.
- This repo uses snake_case for Go test fixture filenames.
- Don't edit ` + "`docs/internals.md`" + ` directly — it is generated.
- Assume the "build engineer" persona for any work under ` + "`scripts/`" + `.

## Notes

- The file is read as UTF-8 (BOM is stripped if present).
- Content over 16 KiB is truncated; a footer marker names the cut-off.
- ` + "`mote prime --memories-only`" + ` and ` + "`mote prime --export`" + ` both
  ignore this file. ` + "`mote prime --mcp`" + ` includes it but stays within
  the MCP token budget — keep this file concise.
`
}
