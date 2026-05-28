// SPDX-License-Identifier: MIT
package docflags

import (
	"strings"
	"testing"
)

// fixtureSurface builds a small CLISurface that mirrors a believable
// fragment of the live mote tree: `ls --ready`, `ls --json`, `show
// --json`, `compliance export --json`, root persistents.
func fixtureSurface() CLISurface {
	return CLISurface{
		Commands: []CommandSpec{
			{Name: "ls", Flags: []FlagSpec{{Name: "--ready"}, {Name: "--json"}}},
			{Name: "show", Flags: []FlagSpec{{Name: "--json"}}},
			{Name: "add", Flags: []FlagSpec{{Name: "--type"}, {Name: "--title"}}},
			{Name: "compliance export", Flags: []FlagSpec{{Name: "--json"}}},
		},
		Persistent: []FlagSpec{
			{Name: "--no-color"},
			{Name: "--plain"},
			{Name: "--pretty"},
			{Name: "--help"},
		},
	}
}

func TestCheck_KnownFlagIsClean(t *testing.T) {
	refs := []Reference{{Path: "x.md", Line: 1, Command: "ls", Flag: "--ready"}}
	got := Check(refs, fixtureSurface(), nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("expected no violations, got %+v", got)
	}
}

// Scenario 2 from the story.
func TestCheck_Scenario2_UnknownFlagFails(t *testing.T) {
	refs := []Reference{{Path: "docs/onboarding.md", Line: 7, Command: "ls", Flag: "--no-such-flag"}}
	got := Check(refs, fixtureSurface(), nil, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Reason, "unknown flag") {
		t.Errorf("reason should name flag as unknown, got %q", got[0].Reason)
	}
	if !strings.Contains(got[0].Reason, "ls") {
		t.Errorf("reason should name command ls, got %q", got[0].Reason)
	}
}

// Scenario 5 from the story.
func TestCheck_Scenario5_CrossCommandFlagReuseIsViolation(t *testing.T) {
	// --ready exists on ls, not on show. Documenting `mote show
	// --ready` is a real bug to surface.
	refs := []Reference{{Path: "docs/agents-guide.md", Line: 12, Command: "show", Flag: "--ready"}}
	got := Check(refs, fixtureSurface(), nil, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %+v", got)
	}
	if !strings.Contains(got[0].Reason, "show") || !strings.Contains(got[0].Reason, "--ready") {
		t.Errorf("reason should name show and --ready, got %q", got[0].Reason)
	}
}

// Scenario 6 from the story.
func TestCheck_Scenario6_PersistentFlagAcceptedAnywhere(t *testing.T) {
	refs := []Reference{
		{Path: "docs/internals.md", Line: 30, Command: "ls", Flag: "--no-color"},
		{Path: "docs/internals.md", Line: 31, Command: "show", Flag: "--no-color"},
	}
	got := Check(refs, fixtureSurface(), nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("persistent flag should validate on any command, got %+v", got)
	}
}

// Scenario 3 from the story.
func TestCheck_Scenario3_RemovedFlagAllowlistedIsClean(t *testing.T) {
	fileContent := []string{
		"# Doc",
		"",
		"## Removed flags",
		"",
		"The `--legacy-index` flag was retired.",
	}
	refs := []Reference{{Path: "docs/maintenance.md", Line: 5, Command: "ls", Flag: "--legacy-index"}}
	removed := map[string]map[string]bool{"ls": {"--legacy-index": true}}
	fileLines := map[string][]string{"docs/maintenance.md": fileContent}

	got := Check(refs, fixtureSurface(), removed, fileLines, DefaultTokens)
	if len(got) != 0 {
		t.Errorf("removed flag in Removed section should be allowlisted, got %+v", got)
	}
}

// Scenario 4 from the story.
func TestCheck_Scenario4_RemovedFlagOutsideAllowlistedContextFails(t *testing.T) {
	fileContent := []string{
		"# Doc",
		"",
		"Run `mote ls --legacy-index` to inspect old data.",
	}
	refs := []Reference{{Path: "docs/onboarding.md", Line: 3, Command: "ls", Flag: "--legacy-index"}}
	removed := map[string]map[string]bool{"ls": {"--legacy-index": true}}
	fileLines := map[string][]string{"docs/onboarding.md": fileContent}

	got := Check(refs, fixtureSurface(), removed, fileLines, DefaultTokens)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %+v", got)
	}
	if !strings.Contains(strings.ToLower(got[0].Reason), "removed") {
		t.Errorf("reason should note the flag as removed, got %q", got[0].Reason)
	}
}

// Scenario 7 from the story (the boundary case).
func TestCheck_Scenario7_AllowlistAppliesPerReferenceNotWholeFile(t *testing.T) {
	fileContent := []string{
		"# Title",
		"",
		"## Removed flags",
		"",
		"The `--legacy-index` flag was removed.",
		"",
		"## Examples",
		"",
		"Run `mote ls --no-such-flag` for diagnostics.",
	}
	refs := []Reference{
		{Path: "docs/maintenance.md", Line: 5, Command: "ls", Flag: "--legacy-index"},
		{Path: "docs/maintenance.md", Line: 9, Command: "ls", Flag: "--no-such-flag"},
	}
	removed := map[string]map[string]bool{"ls": {"--legacy-index": true}}
	fileLines := map[string][]string{"docs/maintenance.md": fileContent}

	got := Check(refs, fixtureSurface(), removed, fileLines, DefaultTokens)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 violation (the Examples-section one), got %+v", got)
	}
	if got[0].Reference.Line != 9 {
		t.Errorf("violation should be at line 9 (Examples), got line %d", got[0].Reference.Line)
	}
	if got[0].Reference.Flag != "--no-such-flag" {
		t.Errorf("violation should be --no-such-flag, got %s", got[0].Reference.Flag)
	}
}

func TestCheck_UnknownCommandIsViolation(t *testing.T) {
	refs := []Reference{{Path: "x.md", Line: 1, Command: "nonexistent", Flag: "--anything"}}
	got := Check(refs, fixtureSurface(), nil, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %+v", got)
	}
	if !strings.Contains(got[0].Reason, "unknown command") {
		t.Errorf("reason should name unknown command, got %q", got[0].Reason)
	}
}

func TestCheck_LongestPrefixResolution(t *testing.T) {
	// "compliance export" is a 2-level command; "compliance" alone
	// is also a known command. A reference to "compliance export
	// somearg --json" should resolve to "compliance export" (the
	// longest known prefix).
	refs := []Reference{{Path: "x.md", Line: 1, Command: "compliance export somearg", Flag: "--json"}}
	got := Check(refs, fixtureSurface(), nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("expected longest-prefix resolution to find compliance export, got %+v", got)
	}
}

func TestCheck_PositionalArgFallback(t *testing.T) {
	// "add foo bar" — neither "add foo bar" nor "add foo" is a
	// real command path, but "add" is. Longest-prefix resolution
	// should fall back to "add" and then validate --type against it.
	refs := []Reference{{Path: "x.md", Line: 1, Command: "add foo bar", Flag: "--type"}}
	got := Check(refs, fixtureSurface(), nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("positional-arg fallback to 'add' should validate --type, got %+v", got)
	}
}
