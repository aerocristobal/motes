// SPDX-License-Identifier: MIT
package docflags

import (
	"strings"
	"testing"
)

func TestAllowlist_NearestH2ContainsRemoved(t *testing.T) {
	content := `# Doc title

## Removed flags

The ` + "`--legacy-index`" + ` flag was retired in v0.4.0.
`
	lines := strings.Split(content, "\n")
	// Line numbering: 1="# Doc title", 2="", 3="## Removed flags", 4="", 5="The ..."
	ref := Reference{Path: "x.md", Line: 5, Command: "ls", Flag: "--legacy-index"}
	if !IsAllowlisted(ref, lines, DefaultTokens) {
		t.Errorf("expected allowlisted under '## Removed flags' heading")
	}
}

func TestAllowlist_NearestH2DoesNotContainToken(t *testing.T) {
	content := `# Doc title

## Examples

Run ` + "`mote ls --legacy-index`" + ` for diagnostics.
`
	lines := strings.Split(content, "\n")
	ref := Reference{Path: "x.md", Line: 5, Command: "ls", Flag: "--legacy-index"}
	if IsAllowlisted(ref, lines, DefaultTokens) {
		t.Errorf("expected NOT allowlisted under '## Examples' heading")
	}
}

func TestAllowlist_ParagraphContainsToken(t *testing.T) {
	// No H2 here; the paragraph itself carries the allowlist token.
	content := `# Title

This paragraph describes a deprecated workflow that used
` + "`mote ls --legacy-index`" + ` to inspect old formats.

Another paragraph.
`
	lines := strings.Split(content, "\n")
	ref := Reference{Path: "x.md", Line: 4, Command: "ls", Flag: "--legacy-index"}
	if !IsAllowlisted(ref, lines, DefaultTokens) {
		t.Errorf("expected allowlisted via paragraph token 'deprecated'")
	}
}

func TestAllowlist_TokenInFarParagraphDoesNotAllowlist(t *testing.T) {
	// Token in a different paragraph (separated by blank line) must
	// not leak across paragraph boundaries.
	content := `# Title

This paragraph mentions removed and deprecated flags.

Now an unrelated paragraph using ` + "`mote ls --legacy-index`" + `.
`
	lines := strings.Split(content, "\n")
	ref := Reference{Path: "x.md", Line: 5, Command: "ls", Flag: "--legacy-index"}
	if IsAllowlisted(ref, lines, DefaultTokens) {
		t.Errorf("token in far-away paragraph must not allowlist")
	}
}

func TestAllowlist_Scenario7_PerReferenceNotWholeFile(t *testing.T) {
	// From the story: a doc has a Removed heading section with one
	// allowlisted reference, AND a separate section with a real
	// violation. The allowlist must apply per-reference.
	content := `# Title

## Removed flags

The ` + "`--legacy-index`" + ` flag was removed.

## Examples

Run ` + "`mote ls --no-such-flag`" + ` for diagnostics.
`
	lines := strings.Split(content, "\n")
	allowed := Reference{Path: "x.md", Line: 5, Command: "ls", Flag: "--legacy-index"}
	violation := Reference{Path: "x.md", Line: 9, Command: "ls", Flag: "--no-such-flag"}

	if !IsAllowlisted(allowed, lines, DefaultTokens) {
		t.Errorf("first reference under Removed heading should be allowlisted")
	}
	if IsAllowlisted(violation, lines, DefaultTokens) {
		t.Errorf("second reference under Examples heading must NOT be allowlisted")
	}
}

func TestAllowlist_CaseInsensitive(t *testing.T) {
	content := `# T

## CHANGELOG

` + "`mote ls --legacy-index`" + ` removed in 0.4.
`
	lines := strings.Split(content, "\n")
	ref := Reference{Path: "x.md", Line: 5, Command: "ls", Flag: "--legacy-index"}
	if !IsAllowlisted(ref, lines, DefaultTokens) {
		t.Errorf("CHANGELOG (uppercase) should match case-insensitively")
	}
}

func TestAllowlist_H1ResetsContext(t *testing.T) {
	// Walking backward should stop at an H1 — an H1 indicates a new
	// top-level section and the H2 above it doesn't apply anymore.
	content := `## Removed flags

intro

# New section

Run ` + "`mote ls --legacy-index`" + ` here.
`
	lines := strings.Split(content, "\n")
	ref := Reference{Path: "x.md", Line: 7, Command: "ls", Flag: "--legacy-index"}
	if IsAllowlisted(ref, lines, DefaultTokens) {
		t.Errorf("H1 must reset H2 context; reference should not be allowlisted")
	}
}

func TestAllowlist_HistoricalToken(t *testing.T) {
	content := `## Historical note

Once, ` + "`mote ls --legacy-index`" + ` existed.
`
	lines := strings.Split(content, "\n")
	ref := Reference{Path: "x.md", Line: 3, Command: "ls", Flag: "--legacy-index"}
	if !IsAllowlisted(ref, lines, DefaultTokens) {
		t.Errorf("'Historical' must allowlist (it is in DefaultTokens)")
	}
}
