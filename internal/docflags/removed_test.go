// SPDX-License-Identifier: MIT
package docflags

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRemoved_BasicEntry(t *testing.T) {
	p := writeManifest(t, "ls --legacy-index\n")
	got, err := LoadRemoved(p)
	if err != nil {
		t.Fatalf("LoadRemoved: %v", err)
	}
	if !got["ls"]["--legacy-index"] {
		t.Errorf("expected ls/--legacy-index in map: %v", got)
	}
}

func TestLoadRemoved_TwoWordCommand(t *testing.T) {
	p := writeManifest(t, "compliance export --old-format\n")
	got, err := LoadRemoved(p)
	if err != nil {
		t.Fatalf("LoadRemoved: %v", err)
	}
	if !got["compliance export"]["--old-format"] {
		t.Errorf("expected 'compliance export'/--old-format: %v", got)
	}
}

func TestLoadRemoved_IgnoresCommentsAndBlanks(t *testing.T) {
	p := writeManifest(t, `# header comment
# another
ls --foo

# trailing comment
show --bar
`)
	got, err := LoadRemoved(p)
	if err != nil {
		t.Fatalf("LoadRemoved: %v", err)
	}
	if !got["ls"]["--foo"] || !got["show"]["--bar"] {
		t.Errorf("expected both entries, got %v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 cmd entries, got %d: %v", len(got), got)
	}
}

func TestLoadRemoved_MissingFileReturnsEmpty(t *testing.T) {
	got, err := LoadRemoved("/no/such/path/that/exists")
	if err != nil {
		t.Errorf("missing file should be OK, got error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("missing file should return empty map, got %v", got)
	}
}

func TestLoadRemoved_RejectsMalformedLine(t *testing.T) {
	p := writeManifest(t, "onlyoneword\n")
	_, err := LoadRemoved(p)
	if err == nil {
		t.Fatal("expected error for malformed line")
	}
}

func TestLoadRemoved_RejectsBareFlag(t *testing.T) {
	// Flag must start with `--`. A bare word as the last field is
	// not a flag.
	p := writeManifest(t, "ls notaflag\n")
	_, err := LoadRemoved(p)
	if err == nil {
		t.Fatal("expected error when flag does not start with --")
	}
}

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "removed-flags.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return p
}
