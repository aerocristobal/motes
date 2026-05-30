// SPDX-License-Identifier: MIT
// STORY-PRIMEOVR-001 — unit tests for three-tier resolution + LoadPrimeMd.
package prime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/prime"
)

// Helpers ---------------------------------------------------------------

// fakeProject creates a fake workspace + .memory/ pair and a fake HOME
// (with .motes/ already created so the test only needs to drop files
// in). Returns (memoryRoot, homeDir).
func fakeProject(t *testing.T) (string, string) {
	t.Helper()
	workspace := t.TempDir()
	memoryRoot := filepath.Join(workspace, ".memory")
	if err := os.MkdirAll(memoryRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".motes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return memoryRoot, home
}

func writeFile(t *testing.T, p string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeStr(t *testing.T, p, body string) {
	t.Helper()
	writeFile(t, p, []byte(body))
}

func tierPath(memoryRoot, home string, tier prime.Tier) string {
	switch tier {
	case prime.TierClone:
		return filepath.Join(memoryRoot, prime.PrimeMdFilename)
	case prime.TierWorkspace:
		return filepath.Join(filepath.Dir(memoryRoot), prime.PrimeMdFilename)
	case prime.TierUserGlobal:
		return filepath.Join(home, ".motes", prime.PrimeMdFilename)
	}
	return ""
}

// Scenario 1 — Clone-specific PRIME.md wins when all three tiers exist.
// ---------------------------------------------------------------------

func TestResolveOverride_TierCloneWinsWhenAllPresent(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	writeStr(t, tierPath(memoryRoot, home, prime.TierClone), "alpha")
	writeStr(t, tierPath(memoryRoot, home, prime.TierWorkspace), "beta")
	writeStr(t, tierPath(memoryRoot, home, prime.TierUserGlobal), "gamma")

	got, dbg := prime.ResolveOverride(memoryRoot, home)
	if got == nil {
		t.Fatal("expected a resolved override, got nil")
	}
	if got.Tier != prime.TierClone {
		t.Errorf("Tier = %v, want TierClone", got.Tier)
	}
	if got.Content != "alpha" {
		t.Errorf("Content = %q, want %q", got.Content, "alpha")
	}
	if strings.Contains(got.Content, "beta") || strings.Contains(got.Content, "gamma") {
		t.Errorf("higher-tier content leaked into lower tier: %q", got.Content)
	}
	if got.TruncatedAt != 0 {
		t.Errorf("TruncatedAt = %d, want 0 for sub-cap file", got.TruncatedAt)
	}
	if got.OriginalSize != len("alpha") {
		t.Errorf("OriginalSize = %d, want %d", got.OriginalSize, len("alpha"))
	}
	if len(dbg) != 0 {
		t.Errorf("no tier should be skipped when tier-1 resolves cleanly; got dbg=%v", dbg)
	}
}

// Scenario 2 — Workspace-shared wins when tier 1 absent. ----------------

func TestResolveOverride_TierWorkspaceWhenCloneAbsent(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	writeStr(t, tierPath(memoryRoot, home, prime.TierWorkspace), "shared rules for this repo")
	writeStr(t, tierPath(memoryRoot, home, prime.TierUserGlobal), "global rules")

	got, _ := prime.ResolveOverride(memoryRoot, home)
	if got == nil {
		t.Fatal("expected resolved override, got nil")
	}
	if got.Tier != prime.TierWorkspace {
		t.Errorf("Tier = %v, want TierWorkspace", got.Tier)
	}
	if got.Content != "shared rules for this repo" {
		t.Errorf("Content = %q", got.Content)
	}
	if strings.Contains(got.Content, "global rules") {
		t.Errorf("tier-3 content leaked: %q", got.Content)
	}
}

// Scenario 3 — User-global wins when neither project-level tier exists. -

func TestResolveOverride_TierUserGlobalWhenOnlyGlobalPresent(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	writeStr(t, tierPath(memoryRoot, home, prime.TierUserGlobal), "my own rules across all projects")

	got, _ := prime.ResolveOverride(memoryRoot, home)
	if got == nil {
		t.Fatal("expected resolved override, got nil")
	}
	if got.Tier != prime.TierUserGlobal {
		t.Errorf("Tier = %v, want TierUserGlobal", got.Tier)
	}
	if got.Content != "my own rules across all projects" {
		t.Errorf("Content = %q", got.Content)
	}
}

// Scenario 4 — No tier exists → nil result (baked-in default rendered). -

func TestResolveOverride_NilWhenNoTier(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	got, dbg := prime.ResolveOverride(memoryRoot, home)
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
	if len(dbg) != 0 {
		t.Errorf("no tier should be 'tried-and-skipped' when files don't exist; got dbg=%v", dbg)
	}
}

func TestResolveOverride_EmptyHomeDirSkipsTier3(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	// Place a tier-3 file (in the *real* home), but pass empty homeDir
	// to ResolveOverride — tier 3 should be skipped.
	writeStr(t, tierPath(memoryRoot, home, prime.TierUserGlobal), "global")
	got, _ := prime.ResolveOverride(memoryRoot, "")
	if got != nil {
		t.Errorf("expected nil when homeDir is empty, got %+v", got)
	}
}

// Bug-fix coverage — when every tier is present but each fails to
// load, ResolveOverride returns nil but the accumulated debug messages
// must still be returned so --debug can surface them. Regression for
// the "--debug shows nothing when all tiers fail" issue caught during
// post-implementation validation.
func TestResolveOverride_DebugMessagesReturnedWhenAllTiersFail(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("mode 0000 is bypassed by root")
	}
	memoryRoot, home := fakeProject(t)

	clonePath := tierPath(memoryRoot, home, prime.TierClone)
	workPath := tierPath(memoryRoot, home, prime.TierWorkspace)
	globPath := tierPath(memoryRoot, home, prime.TierUserGlobal)
	for _, p := range []string{clonePath, workPath, globPath} {
		writeStr(t, p, "unreadable")
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func(p string) func() { return func() { _ = os.Chmod(p, 0o644) } }(p))
	}

	got, dbg := prime.ResolveOverride(memoryRoot, home)
	if got != nil {
		t.Errorf("expected nil result when every tier fails; got %+v", got)
	}
	if len(dbg) != 3 {
		t.Errorf("expected one debug message per failing tier (3); got %d: %v", len(dbg), dbg)
	}
	joined := strings.Join(dbg, "\n")
	for _, p := range []string{clonePath, workPath, globPath} {
		if !strings.Contains(joined, p) {
			t.Errorf("debug messages should name path %q; got:\n%s", p, joined)
		}
	}
}

// Scenario 6 — Fall-through outline. ------------------------------------

func TestResolveOverride_FallsThroughOnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("mode 0000 is bypassed by root; skipping when running as root")
	}
	memoryRoot, home := fakeProject(t)

	clonePath := tierPath(memoryRoot, home, prime.TierClone)
	writeFile(t, clonePath, []byte("unreadable content"))
	if err := os.Chmod(clonePath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(clonePath, 0o644) })

	writeStr(t, tierPath(memoryRoot, home, prime.TierWorkspace), "fallback content")

	got, dbg := prime.ResolveOverride(memoryRoot, home)
	if got == nil {
		t.Fatal("expected fallback to tier-workspace, got nil")
	}
	if got.Tier != prime.TierWorkspace {
		t.Errorf("Tier = %v, want TierWorkspace", got.Tier)
	}
	if got.Content != "fallback content" {
		t.Errorf("Content = %q", got.Content)
	}
	if len(dbg) == 0 {
		t.Errorf("expected debug messages naming the skipped tier-1 path; got none")
	} else {
		joined := strings.Join(dbg, "\n")
		if !strings.Contains(joined, clonePath) {
			t.Errorf("debug messages should mention the skipped path %q; got %q", clonePath, joined)
		}
	}
}

func TestResolveOverride_FallsThroughOnInvalidUTF8(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	// 0xFF is invalid as the first byte of any UTF-8 sequence.
	writeFile(t, tierPath(memoryRoot, home, prime.TierClone), []byte{0xFF, 0xFE, 0xFD})
	writeStr(t, tierPath(memoryRoot, home, prime.TierWorkspace), "fallback")

	got, dbg := prime.ResolveOverride(memoryRoot, home)
	if got == nil || got.Tier != prime.TierWorkspace || got.Content != "fallback" {
		t.Fatalf("expected tier-2 fallback, got %+v", got)
	}
	if len(dbg) == 0 {
		t.Error("expected debug messages for invalid-UTF-8 fall-through")
	}
}

func TestResolveOverride_FallsThroughOnBrokenSymlink(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	clonePath := tierPath(memoryRoot, home, prime.TierClone)
	if err := os.Symlink("/nonexistent/target", clonePath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	writeStr(t, tierPath(memoryRoot, home, prime.TierWorkspace), "fallback")

	got, _ := prime.ResolveOverride(memoryRoot, home)
	if got == nil || got.Tier != prime.TierWorkspace {
		t.Fatalf("expected tier-2 fallback for broken symlink, got %+v", got)
	}
}

func TestResolveOverride_EmptyFileIsValidNotSkipped(t *testing.T) {
	// Quirk to pin: a zero-byte PRIME.md is *valid* UTF-8 and within the
	// size cap, so it should be returned (not skipped). The renderer
	// guards on Content != "" before emitting.
	memoryRoot, home := fakeProject(t)
	writeStr(t, tierPath(memoryRoot, home, prime.TierClone), "")

	got, _ := prime.ResolveOverride(memoryRoot, home)
	if got == nil {
		t.Fatal("empty PRIME.md should resolve, not fall through")
	}
	if got.Tier != prime.TierClone || got.Content != "" {
		t.Errorf("got %+v, want tier-clone with empty content", got)
	}
}

// Scenario 7 — Truncation at PRIME_MD_MAX_BYTES with marker. -----------

func TestResolveOverride_TruncatesOversizeFileAndAppendsMarker(t *testing.T) {
	memoryRoot, home := fakeProject(t)
	clonePath := tierPath(memoryRoot, home, prime.TierClone)

	big := make([]byte, prime.PrimeMdMaxBytes*2)
	for i := range big {
		big[i] = 'A'
	}
	writeFile(t, clonePath, big)

	got, _ := prime.ResolveOverride(memoryRoot, home)
	if got == nil {
		t.Fatal("expected override, got nil")
	}
	if got.TruncatedAt != prime.PrimeMdMaxBytes {
		t.Errorf("TruncatedAt = %d, want %d", got.TruncatedAt, prime.PrimeMdMaxBytes)
	}
	if got.OriginalSize != len(big) {
		t.Errorf("OriginalSize = %d, want %d", got.OriginalSize, len(big))
	}
	if !strings.Contains(got.Content, "[PRIME.md truncated") {
		t.Errorf("expected truncation marker in Content, got first 64 bytes: %q", got.Content[:64])
	}
	if !strings.Contains(got.Content, clonePath) {
		t.Errorf("truncation marker should reference path %q", clonePath)
	}
}

// BOM stripping ---------------------------------------------------------

func TestLoadPrimeMd_StripsLeadingUTF8BOM(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, prime.PrimeMdFilename)
	writeFile(t, p, append([]byte{0xEF, 0xBB, 0xBF}, []byte("rules")...))

	content, originalSize, truncatedAt, err := prime.LoadPrimeMd(p)
	if err != nil {
		t.Fatal(err)
	}
	if content != "rules" {
		t.Errorf("Content = %q, want %q (BOM should be stripped)", content, "rules")
	}
	if originalSize != 3+len("rules") {
		t.Errorf("OriginalSize = %d, want %d (raw file size including BOM)", originalSize, 3+len("rules"))
	}
	if truncatedAt != 0 {
		t.Errorf("TruncatedAt = %d, want 0", truncatedAt)
	}
}

func TestLoadPrimeMd_AtCapDoesNotTruncate(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, prime.PrimeMdFilename)
	body := make([]byte, prime.PrimeMdMaxBytes)
	for i := range body {
		body[i] = 'B'
	}
	writeFile(t, p, body)

	content, _, truncatedAt, err := prime.LoadPrimeMd(p)
	if err != nil {
		t.Fatal(err)
	}
	if truncatedAt != 0 {
		t.Errorf("file exactly at cap should not trigger truncation marker; truncatedAt=%d", truncatedAt)
	}
	if strings.Contains(content, "[PRIME.md truncated") {
		t.Error("file at cap should not contain truncation marker")
	}
}

// Scenario 9-adjacent — DefaultExportTemplate sanity ------------------

func TestDefaultExportTemplate_ContainsNeitherDirectiveNorDataHeaders(t *testing.T) {
	tpl := prime.DefaultExportTemplate()

	if tpl == "" {
		t.Fatal("DefaultExportTemplate returned empty string")
	}
	// Must NOT include mote-generated artifacts (per Scenario 5).
	if strings.Contains(tpl, prime.TruncationDirective) {
		t.Error("export template must not embed the truncation directive (mote generates it at render time)")
	}
	for _, banned := range []string{
		"## Persistent memories",
		"## Ready to start",
		"## Active work",
		"## Relevant decisions",
		"## Key lessons",
		"## Prior explorations",
	} {
		if strings.Contains(tpl, banned) {
			t.Errorf("export template must not embed live data section header %q", banned)
		}
	}
	// Must mention the three tier paths so a user can navigate from
	// `--export` to the right destination.
	for _, expected := range []string{
		".memory/PRIME.md",
		"~/.motes/PRIME.md",
	} {
		if !strings.Contains(tpl, expected) {
			t.Errorf("export template should mention tier path %q", expected)
		}
	}
}

func TestTierString_KnownTiers(t *testing.T) {
	cases := []struct {
		tier prime.Tier
		want string
	}{
		{prime.TierClone, "tier-clone"},
		{prime.TierWorkspace, "tier-workspace"},
		{prime.TierUserGlobal, "tier-user-global"},
		{prime.TierNone, "tier-none"},
	}
	for _, c := range cases {
		if got := c.tier.String(); got != c.want {
			t.Errorf("Tier(%d).String() = %q, want %q", c.tier, got, c.want)
		}
	}
}
