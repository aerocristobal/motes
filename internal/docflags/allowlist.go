// SPDX-License-Identifier: MIT
package docflags

import "strings"

// IsAllowlisted reports whether the flag reference at line ref.Line
// in the file represented by lines is exempt from the removed-flag
// check.  A reference is exempt when EITHER the nearest preceding
// H2 heading text OR the enclosing paragraph (the contiguous run of
// non-blank lines that contains ref.Line) case-insensitively contains
// any of tokens.
//
// The two-axis scope (heading OR paragraph) is what gives Scenario 7
// its precision: a "## Removed flags" heading allowlists everything
// in its section until the next H2, while a stray reference in a
// "## Examples" section is checked normally.
func IsAllowlisted(ref Reference, lines []string, tokens []string) bool {
	if len(lines) == 0 || ref.Line < 1 {
		return false
	}

	if paragraphContainsToken(lines, ref.Line, tokens) {
		return true
	}
	if nearestH2ContainsToken(lines, ref.Line, tokens) {
		return true
	}
	return false
}

// paragraphContainsToken checks the contiguous block of non-blank
// lines that contains line ref.Line (1-indexed). Returns true if any
// of tokens appears (case-insensitively) within that block.
func paragraphContainsToken(lines []string, refLine int, tokens []string) bool {
	idx := refLine - 1
	if idx < 0 || idx >= len(lines) {
		return false
	}

	// Walk up to the start of the paragraph (or fenced block — we
	// treat fences as opaque paragraph members so allowlist tokens
	// inside a fenced block are recognized).
	start := idx
	for start > 0 && strings.TrimSpace(lines[start-1]) != "" {
		start--
	}
	end := idx
	for end < len(lines)-1 && strings.TrimSpace(lines[end+1]) != "" {
		end++
	}

	block := strings.ToLower(strings.Join(lines[start:end+1], "\n"))
	for _, tok := range tokens {
		if strings.Contains(block, strings.ToLower(tok)) {
			return true
		}
	}
	return false
}

// nearestH2ContainsToken walks backward from ref.Line looking for the
// nearest line whose trimmed prefix is "## " (an H2 heading). Returns
// true if the heading text contains any token (case-insensitively).
// Stops at the start of the file or at any "# " H1, since an H1
// resets the section context for a single-file document.
func nearestH2ContainsToken(lines []string, refLine int, tokens []string) bool {
	for i := refLine - 1; i >= 0; i-- {
		if i >= len(lines) {
			continue
		}
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "## ") {
			head := strings.ToLower(strings.TrimPrefix(t, "## "))
			for _, tok := range tokens {
				if strings.Contains(head, strings.ToLower(tok)) {
					return true
				}
			}
			return false
		}
		if strings.HasPrefix(t, "# ") && !strings.HasPrefix(t, "## ") {
			// H1 reached without finding an H2; nothing to match on.
			return false
		}
	}
	return false
}
