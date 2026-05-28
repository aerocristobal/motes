// SPDX-License-Identifier: MIT
package docflags

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// flagTokenRe matches a flag name at the start of a token. It rejects
// trailing punctuation like the dot in "--json." (sentence end) by
// only capturing the alphabetic-and-hyphen run after the leading
// `--`. Single-dash short flags (`-x`) are intentionally not extracted
// because docs uniformly use long flags.
var flagTokenRe = regexp.MustCompile(`^--([a-z][a-z0-9-]*)`)

// commandTokenRe matches a bare-word subcommand token (no leading
// dash). Cobra subcommand names are lowercase with optional hyphens.
var commandTokenRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Scan reads every file in paths (relative to repoRoot) and returns
// every `mote <cmd> [--flag]` reference it finds, honoring the
// SuppressionMarker directive. fileLines is populated as a side effect
// so the checker can resolve allowlist context without re-reading.
//
// Error model: returns a non-nil error only on filesystem failure
// (missing file, unreadable file). Files with no references return
// cleanly with an empty slice. The caller is expected to validate
// paths exist before calling.
func Scan(repoRoot string, paths []string) ([]Reference, map[string][]string, error) {
	var refs []Reference
	fileLines := make(map[string][]string, len(paths))
	for _, p := range paths {
		full := filepath.Join(repoRoot, p)
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", p, err)
		}
		lines := strings.Split(string(data), "\n")
		fileLines[p] = lines
		refs = append(refs, scanLines(p, lines)...)
	}
	return refs, fileLines, nil
}

// scanLines walks one file's lines, honors the suppression marker,
// and emits a Reference for every (command, flag) pair it extracts.
// Two contexts emit references:
//   - lines inside a fenced code block (```...```): the whole line
//     is treated as code text.
//   - inline backtick spans on a prose line: only what is between a
//     matching pair of backticks is treated as code text.
//
// Prose outside these contexts is ignored, which is what stops
// sentences like " `mote` does not expose a `--format` flag " from
// being read as a fake `mote` clause that spans both backtick spans.
func scanLines(path string, lines []string) []Reference {
	var out []Reference
	inFence := false
	suppressNext := false
	skipUntilFenceClose := false

	for i, raw := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(raw)
		isFenceMarker := strings.HasPrefix(trimmed, "```")

		if skipUntilFenceClose {
			if isFenceMarker {
				skipUntilFenceClose = false
				// Mirror the inFence state so the matching closing
				// fence (or any subsequent fences) toggle correctly.
				inFence = !inFence
			}
			continue
		}

		// Suppression marker — flag the next non-blank line.
		if strings.Contains(raw, SuppressionMarker) {
			suppressNext = true
			continue
		}

		// Blank lines preserve a pending suppression.
		if trimmed == "" {
			continue
		}

		if suppressNext {
			suppressNext = false
			if isFenceMarker {
				// Suppression hops the whole fenced block. Toggle
				// fence state so the closing ``` toggles us back
				// out of fence mode.
				skipUntilFenceClose = true
				inFence = !inFence
			}
			// Either way, this line itself is suppressed.
			continue
		}

		if isFenceMarker {
			inFence = !inFence
			continue
		}

		var codeText []string
		if inFence {
			// Inside a fenced code block, the entire line is code.
			codeText = []string{raw}
		} else {
			// On prose lines, only inline-code spans are code.
			codeText = inlineCodeSpans(raw)
		}

		for _, ct := range codeText {
			for _, r := range extractFromCodeText(ct) {
				r.Path = path
				r.Line = lineNum
				out = append(out, r)
			}
		}
	}
	return out
}

// inlineCodeSpans returns the contents of every matched pair of
// backticks in line. An unmatched opening backtick (no closing tick
// on the same line) is treated as plain text and contributes no
// span — markdown's rule is that inline code must close on the same
// line, so we follow it.
func inlineCodeSpans(line string) []string {
	var spans []string
	inCode := false
	start := -1
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			continue
		}
		if !inCode {
			inCode = true
			start = i + 1
		} else {
			inCode = false
			spans = append(spans, line[start:i])
			start = -1
		}
	}
	return spans
}

// extractFromCodeText pulls every `mote <cmd...> --<flag>` reference
// from a span of code text (a single inline-code span, or a single
// line inside a fenced block). Each `mote` keyword in the text starts
// its own clause; clauses are bounded by the next `mote` keyword (or
// the end of the text) so a later clause's flags are never attributed
// to an earlier command.
func extractFromCodeText(text string) []Reference {
	positions := findAllWordStarts(text, "mote")
	if len(positions) == 0 {
		return nil
	}
	var refs []Reference
	for i, pos := range positions {
		start := pos + len("mote")
		end := len(text)
		if i+1 < len(positions) {
			end = positions[i+1]
		}
		refs = append(refs, extractFromClause(text[start:end])...)
	}
	return refs
}

// extractFromClause parses one "mote ..." clause (the text between
// one `mote` keyword and the next on the same line, or end-of-line
// for the last clause). Returns a Reference per `--flag` found.
func extractFromClause(clause string) []Reference {
	tokens := strings.Fields(clause)
	if len(tokens) == 0 {
		return nil
	}

	var cmdParts []string
	flagStart := -1
	for j, t := range tokens {
		if strings.HasPrefix(t, "-") {
			flagStart = j
			break
		}
		if !commandTokenRe.MatchString(t) {
			// Hit something that is neither a bare word nor a
			// flag (e.g. quoted string, punctuation tail). End
			// of this `mote` clause's command-word run.
			break
		}
		cmdParts = append(cmdParts, t)
	}

	// A doc-style reference needs both a command word and a flag.
	// Bare `mote --help` (no subcommand) is not what the story
	// gates; skip it.
	if len(cmdParts) == 0 || flagStart < 0 {
		return nil
	}

	cmd := strings.Join(cmdParts, " ")
	var refs []Reference
	for _, ft := range tokens[flagStart:] {
		// Non-flag tokens are flag values or trailing prose; skip
		// them but keep looking for additional flags after them.
		if !strings.HasPrefix(ft, "-") {
			continue
		}
		m := flagTokenRe.FindStringSubmatch(ft)
		if m == nil {
			continue
		}
		refs = append(refs, Reference{
			Command: cmd,
			Flag:    "--" + m[1],
		})
	}
	return refs
}

// findAllWordStarts returns every offset where word appears in s as
// a standalone token (i.e. surrounded by non-identifier characters).
// Returns nil when there are no matches.
func findAllWordStarts(s, word string) []int {
	var out []int
	cursor := 0
	for cursor < len(s) {
		idx := strings.Index(s[cursor:], word)
		if idx < 0 {
			return out
		}
		abs := cursor + idx
		if isWordBoundaryBefore(s, abs) && isWordBoundaryAfter(s, abs+len(word)) {
			out = append(out, abs)
		}
		cursor = abs + 1
	}
	return out
}

func isWordBoundaryBefore(s string, pos int) bool {
	if pos == 0 {
		return true
	}
	c := s[pos-1]
	return !isIdentChar(c)
}

func isWordBoundaryAfter(s string, pos int) bool {
	if pos >= len(s) {
		return true
	}
	c := s[pos]
	return !isIdentChar(c)
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}
