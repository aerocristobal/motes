// SPDX-License-Identifier: MIT
package docflags

import (
	"os"
	"path/filepath"
	"testing"
)

// --- extractFromCodeText: clause-level extraction ------------------------
//
// These tests feed code text directly (the kind of thing that lives
// inside backticks or inside a fenced block). They do not exercise
// the markdown-context filter — that's covered by the scanLines-level
// tests below.

func TestExtractCodeText_BasicReference(t *testing.T) {
	got := extractFromCodeText("mote ls --ready")
	mustHaveRef(t, got, "ls", "--ready")
	if len(got) != 1 {
		t.Errorf("expected 1 reference, got %d: %+v", len(got), got)
	}
}

func TestExtractCodeText_MultipleFlagsOnOneCommand(t *testing.T) {
	got := extractFromCodeText("mote ls --ready --json")
	if len(got) != 2 {
		t.Fatalf("expected 2 references, got %d: %+v", len(got), got)
	}
	mustHaveRef(t, got, "ls", "--ready")
	mustHaveRef(t, got, "ls", "--json")
}

func TestExtractCodeText_FlagWithEqualsValue(t *testing.T) {
	got := extractFromCodeText(`mote add --type=task --title="Foo"`)
	mustHaveRef(t, got, "add", "--type")
	mustHaveRef(t, got, "add", "--title")
}

func TestExtractCodeText_TwoLevelCommand(t *testing.T) {
	got := extractFromCodeText("mote compliance export --json")
	if len(got) != 1 {
		t.Fatalf("expected 1 reference, got %d: %+v", len(got), got)
	}
	mustHaveRef(t, got, "compliance export", "--json")
}

func TestExtractCodeText_StripsTrailingPunctuation(t *testing.T) {
	got := extractFromCodeText("mote ls --json.")
	mustHaveRef(t, got, "ls", "--json")
	for _, r := range got {
		if r.Flag == "--json." {
			t.Errorf("trailing punctuation leaked into flag name: %+v", r)
		}
	}
}

func TestExtractCodeText_IgnoresMoteWithoutCommandWord(t *testing.T) {
	got := extractFromCodeText("mote --help")
	if len(got) != 0 {
		t.Errorf("expected 0 refs for bare mote --help, got %+v", got)
	}
}

func TestExtractCodeText_IgnoresWordContainingMote(t *testing.T) {
	got := extractFromCodeText("remote-only --ready filter")
	if len(got) != 0 {
		t.Errorf("expected 0 refs (no real mote token), got %+v", got)
	}
}

func TestExtractCodeText_SkipsValueTokenAfterFlag(t *testing.T) {
	got := extractFromCodeText("mote ls --type task --json")
	mustHaveRef(t, got, "ls", "--type")
	mustHaveRef(t, got, "ls", "--json")
}

// --- inlineCodeSpans: backtick-span splitting -----------------------------

func TestInlineCodeSpans_TwoSpansOnLine(t *testing.T) {
	got := inlineCodeSpans("Run `mote ls --ready` then `mote show --json`.")
	want := []string{"mote ls --ready", "mote show --json"}
	if !equalSlices(got, want) {
		t.Errorf("inlineCodeSpans = %v, want %v", got, want)
	}
}

func TestInlineCodeSpans_NoBackticks(t *testing.T) {
	got := inlineCodeSpans("plain prose with no code")
	if len(got) != 0 {
		t.Errorf("expected no spans, got %v", got)
	}
}

func TestInlineCodeSpans_UnclosedBacktickProducesNoSpan(t *testing.T) {
	got := inlineCodeSpans("text with `unclosed code")
	if len(got) != 0 {
		t.Errorf("unclosed backtick should produce no span, got %v", got)
	}
}

// --- Scan: markdown-context-aware extraction ------------------------------

// Scenario from the prose-false-positive bug: two backtick spans on
// one line, only one of which contains `mote`. The `mote` span has no
// flag; the other span has flags but no `mote`. Result: NO refs.
func TestScan_ProseBetweenBackticksDoesNotCreateClause(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md",
		"`mote` does not currently expose a `--format json` flag.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("prose between separate backtick spans should produce no refs, got %+v", refs)
	}
}

func TestScan_ReferenceInsideOneInlineCodeSpan(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md", "Use `mote ls --ready` to filter.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 1 || refs[0].Command != "ls" || refs[0].Flag != "--ready" {
		t.Errorf("expected single ls/--ready ref, got %+v", refs)
	}
}

func TestScan_TwoInlineCodeSpansOnOneLine(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md",
		"Use `mote ls --ready` then `mote show --json` for context.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %+v", len(refs), refs)
	}
	mustHaveRef(t, refs, "ls", "--ready")
	mustHaveRef(t, refs, "show", "--json")
}

func TestScan_FencedBlockTreatsWholeLineAsCode(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md",
		"```bash\n"+
			"mote ls --ready\n"+
			"```\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 1 || refs[0].Flag != "--ready" {
		t.Errorf("expected ls/--ready inside fenced block, got %+v", refs)
	}
}

func TestScan_ProseOutsideCodeIsNotScanned(t *testing.T) {
	root := t.TempDir()
	// The word "mote" and a "--flag" pattern appear in prose with
	// no backticks. This should NOT produce a reference.
	writeFile(t, root, "x.md",
		"The mote project uses --json output by convention.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("prose mention should not produce refs, got %+v", refs)
	}
}

func TestScan_LineNumbersAreOneIndexed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md", "header\n\nUse `mote ls --ready` here.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Path != "x.md" || refs[0].Line != 3 {
		t.Errorf("expected x.md:3, got %s:%d", refs[0].Path, refs[0].Line)
	}
}

// --- Suppression markers --------------------------------------------------

func TestScan_SuppressionMarkerSkipsNextLine(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md",
		"# title\n\n"+
			"<!-- doc-flags: ignore-next -->\n"+
			"Run `mote ls --fake-flag` for diagnostics.\n"+
			"And `mote ls --ready` works.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref (the unsuppressed one), got %d: %+v", len(refs), refs)
	}
	if refs[0].Flag != "--ready" {
		t.Errorf("expected --ready, got %s", refs[0].Flag)
	}
}

func TestScan_SuppressionMarkerSkipsFencedBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md",
		"# title\n\n"+
			"<!-- doc-flags: ignore-next -->\n"+
			"```bash\n"+
			"mote ls --bogus\n"+
			"mote show --also-bogus\n"+
			"```\n\n"+
			"After the block: `mote ls --ready`.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref (only the post-block one), got %d: %+v", len(refs), refs)
	}
	if refs[0].Flag != "--ready" {
		t.Errorf("expected --ready, got %s", refs[0].Flag)
	}
}

func TestScan_BlankLinePreservesPendingSuppression(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "x.md",
		"<!-- doc-flags: ignore-next -->\n"+
			"\n"+
			"Use `mote ls --fake-flag` here.\n")
	refs, _, err := Scan(root, []string{"x.md"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("blank line between marker and target should still suppress; got %+v", refs)
	}
}

func TestScan_MissingFileReturnsError(t *testing.T) {
	root := t.TempDir()
	_, _, err := Scan(root, []string{"nope.md"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- helpers --------------------------------------------------------------

func mustHaveRef(t *testing.T, got []Reference, cmd, flag string) {
	t.Helper()
	for _, r := range got {
		if r.Command == cmd && r.Flag == flag {
			return
		}
	}
	t.Errorf("expected ref {cmd=%q, flag=%q} in %+v", cmd, flag, got)
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
