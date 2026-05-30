// SPDX-License-Identifier: MIT
// STORY-PRIMEOVR-001 — integration tests for the PRIME.md three-tier
// override wired into `mote prime`. Exercises both CLI mode and MCP mode
// and pins the silent-failure fall-through behaviour from Scenario 6.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/prime"
)

// writePrimeMd is a focused helper for these tests so individual cases
// stay readable. It creates parent dirs as needed.
func writePrimeMd(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Scenario 1 — clone-specific PRIME.md is rendered between the directive
// and the data sections; tier-2 and tier-3 are not consulted.
func TestPrime_Override_TierCloneWinsInRenderedOutput(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMemory(t, root, "k", "v")

	writePrimeMd(t, filepath.Join(root, prime.PrimeMdFilename), "alpha")
	writePrimeMd(t, filepath.Join(filepath.Dir(root), prime.PrimeMdFilename), "beta")
	writePrimeMd(t, filepath.Join(home, ".motes", prime.PrimeMdFilename), "gamma")

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})

	if !strings.Contains(output, "alpha") {
		t.Errorf("tier-clone content missing from output:\n%s", output)
	}
	if strings.Contains(output, "beta") || strings.Contains(output, "gamma") {
		t.Errorf("lower-tier content leaked into output:\n%s", output)
	}

	// Ordering invariant: directive → prose → memories section.
	dirIdx := strings.Index(output, "[mote prime]")
	proseIdx := strings.Index(output, "alpha")
	memIdx := strings.Index(output, "## Persistent memories")
	if dirIdx < 0 || dirIdx >= proseIdx || proseIdx >= memIdx {
		t.Errorf("render order broken — directive=%d prose=%d memories=%d\n%s",
			dirIdx, proseIdx, memIdx, output)
	}
}

// Scenario 2 — workspace-shared PRIME.md is used when tier 1 is absent.
func TestPrime_Override_TierWorkspaceWhenCloneAbsent(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	writePrimeMd(t, filepath.Join(filepath.Dir(root), prime.PrimeMdFilename), "shared rules for this repo")
	writePrimeMd(t, filepath.Join(home, ".motes", prime.PrimeMdFilename), "global rules")

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})

	if !strings.Contains(output, "shared rules for this repo") {
		t.Errorf("expected workspace PRIME.md in output:\n%s", output)
	}
	if strings.Contains(output, "global rules") {
		t.Errorf("tier-3 leaked into output:\n%s", output)
	}
}

// Scenario 4 — no PRIME.md anywhere → no prose preamble appears (today's
// output unchanged). Asserts the prose-vs-data boundary: the truncation
// directive flows directly into the memories section with no
// intermediate non-blank content.
func TestPrime_Override_NoFileNoProseEmitted(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMemory(t, root, "k", "v")

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})

	// Find lines between the directive and the memories section header
	// and confirm they're all blank — no leftover prose marker.
	dirLine := "[mote prime]"
	memHeader := "## Persistent memories"
	dirIdx := strings.Index(output, dirLine)
	memIdx := strings.Index(output, memHeader)
	if dirIdx < 0 || memIdx < 0 || memIdx <= dirIdx {
		t.Fatalf("unexpected output layout:\n%s", output)
	}
	// Skip past the directive line itself.
	betweenStart := dirIdx + strings.Index(output[dirIdx:], "\n") + 1
	between := output[betweenStart:memIdx]
	for _, line := range strings.Split(between, "\n") {
		if strings.TrimSpace(line) != "" {
			t.Errorf("unexpected non-blank line between directive and memories: %q\nfull output:\n%s", line, output)
		}
	}
	_ = root
}

// Scenario 6 — unreadable tier-1 PRIME.md falls through to tier-2 with
// no stderr noise by default; --debug surfaces a one-line warning naming
// the skipped path.
func TestPrime_Override_FallThroughIsSilentUnlessDebug(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("mode 0000 is bypassed by root")
	}
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	clonePath := filepath.Join(root, prime.PrimeMdFilename)
	writePrimeMd(t, clonePath, "unreadable")
	if err := os.Chmod(clonePath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(clonePath, 0o644) })

	writePrimeMd(t, filepath.Join(filepath.Dir(root), prime.PrimeMdFilename), "fallback content")

	// Default behavior: stderr stays empty.
	var stdout string
	stderr := captureStderr(func() {
		stdout = captureStdout(func() {
			if err := primeCmd.RunE(primeCmd, nil); err != nil {
				t.Fatalf("prime: %v", err)
			}
		})
	})
	if !strings.Contains(stdout, "fallback content") {
		t.Errorf("expected tier-2 fallback in stdout:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("default mode should be silent on tier fall-through; stderr:\n%s", stderr)
	}

	// --debug surfaces a one-line warning naming the skipped path.
	resetAdaptivePrimeFlags()
	primeDebug = true
	stderr = captureStderr(func() {
		_ = captureStdout(func() {
			if err := primeCmd.RunE(primeCmd, nil); err != nil {
				t.Fatalf("prime --debug: %v", err)
			}
		})
	})
	if !strings.Contains(stderr, clonePath) {
		t.Errorf("expected --debug stderr to name skipped path %q; got:\n%s", clonePath, stderr)
	}
}

// Bug-fix coverage — when EVERY tier exists but each is unreadable,
// `mote prime` falls through to "no preamble" silently by default, and
// `--debug` surfaces a stderr warning for *every* failing tier even
// though no override resolved. Regression for the "--debug shows nothing
// when all tiers fail" issue caught in post-implementation validation.
func TestPrime_Override_DebugSurfacesAllTierFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("mode 0000 is bypassed by root")
	}
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	clonePath := filepath.Join(root, prime.PrimeMdFilename)
	workPath := filepath.Join(filepath.Dir(root), prime.PrimeMdFilename)
	globPath := filepath.Join(home, ".motes", prime.PrimeMdFilename)
	for _, p := range []string{clonePath, workPath, globPath} {
		writePrimeMd(t, p, "unreadable")
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func(p string) func() { return func() { _ = os.Chmod(p, 0o644) } }(p))
	}

	// Default behaviour: silent, no preamble in rendered output.
	stderr := captureStderr(func() {
		out := captureStdout(func() {
			if err := primeCmd.RunE(primeCmd, nil); err != nil {
				t.Fatalf("prime: %v", err)
			}
		})
		if strings.Contains(out, "unreadable") {
			t.Errorf("unreadable content must not leak into rendered output:\n%s", out)
		}
	})
	if stderr != "" {
		t.Errorf("default mode should be silent when every tier fails; stderr:\n%s", stderr)
	}

	// --debug surfaces a warning for every failing tier.
	resetAdaptivePrimeFlags()
	primeDebug = true
	stderr = captureStderr(func() {
		_ = captureStdout(func() {
			if err := primeCmd.RunE(primeCmd, nil); err != nil {
				t.Fatalf("prime --debug: %v", err)
			}
		})
	})
	for _, p := range []string{clonePath, workPath, globPath} {
		if !strings.Contains(stderr, p) {
			t.Errorf("--debug stderr should name failing tier path %q; got:\n%s", p, stderr)
		}
	}
}

// Scenario 7 — oversize PRIME.md is truncated; the marker is visible in
// rendered output.
func TestPrime_Override_OversizeTruncatedWithMarker(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	big := strings.Repeat("A", prime.PrimeMdMaxBytes*2)
	writePrimeMd(t, filepath.Join(root, prime.PrimeMdFilename), big)

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})
	if !strings.Contains(output, "[PRIME.md truncated") {
		t.Errorf("expected truncation marker in output; got %d-byte body", len(output))
	}
}

// Scenario 8 — --memories-only bypasses override resolution entirely.
func TestPrime_Override_MemoriesOnlyBypassesOverride(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMemory(t, root, "k", "v")
	writePrimeMd(t, filepath.Join(root, prime.PrimeMdFilename), "ALPHA-CONTENT-SHOULD-NOT-APPEAR")

	primeMemoriesOnly = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --memories-only: %v", err)
		}
	})
	if strings.Contains(output, "ALPHA-CONTENT-SHOULD-NOT-APPEAR") {
		t.Errorf("--memories-only must not include PRIME.md content; got:\n%s", output)
	}
	if !strings.Contains(output, "[mote prime]") {
		t.Errorf("expected truncation directive still prepended in --memories-only:\n%s", output)
	}
}

// MCP mode — PRIME.md content appears between the directive and the
// memories section (story §10 Notes: "the override is the project's own
// declaration of what agents need to know, and that doesn't go away
// just because the agent has MCP access").
func TestPrime_Override_AppearsInMCPMode(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMemory(t, root, "k", "v")
	writePrimeMd(t, filepath.Join(root, prime.PrimeMdFilename), "mcp-prose-marker")

	primeMCP = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --mcp: %v", err)
		}
	})
	if !strings.Contains(output, "mcp-prose-marker") {
		t.Errorf("expected PRIME.md content in MCP-mode output:\n%s", output)
	}
	dirIdx := strings.Index(output, "[mote prime]")
	proseIdx := strings.Index(output, "mcp-prose-marker")
	memIdx := strings.Index(output, "## Persistent memories")
	if dirIdx >= proseIdx || proseIdx >= memIdx {
		t.Errorf("expected ordering directive < prose < memories; got dirIdx=%d proseIdx=%d memIdx=%d\n%s",
			dirIdx, proseIdx, memIdx, output)
	}
}

// Q7 — JSON envelope gains `prose_section` field, populated when an
// override resolved and omitted otherwise (omitempty).
func TestPrime_Override_JSONShape(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMemory(t, root, "k", "v")
	writePrimeMd(t, filepath.Join(root, prime.PrimeMdFilename), "json-prose")

	primeJSON = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --json: %v", err)
		}
	})
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON in output:\n%s", output)
	}
	var got PrimeOutput
	if err := json.Unmarshal([]byte(output[idx:]), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.ProseSection != "json-prose" {
		t.Errorf("ProseSection = %q, want %q", got.ProseSection, "json-prose")
	}
}

// omitempty contract — when no override, the JSON envelope drops
// prose_section entirely so pre-story consumers see the unchanged shape.
func TestPrime_Override_JSONOmittedWhenNoOverride(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	primeJSON = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --json: %v", err)
		}
	})
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON:\n%s", output)
	}
	if strings.Contains(output[idx:], `"prose_section"`) {
		t.Errorf("prose_section should be omitted when no override resolved:\n%s", output[idx:])
	}
}

// BOM stripping integration smoke — a BOM at the start of PRIME.md does
// not leak into rendered output (would otherwise show as ï»¿ in many
// terminals).
func TestPrime_Override_BOMStrippedInRender(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	body := append([]byte{0xEF, 0xBB, 0xBF}, []byte("first-rune-clean")...)
	clonePath := filepath.Join(root, prime.PrimeMdFilename)
	if err := os.MkdirAll(filepath.Dir(clonePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clonePath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})
	if strings.ContainsRune(output, '\uFEFF') {
		t.Errorf("BOM leaked into rendered output:\n%q", output)
	}
	if !strings.Contains(output, "first-rune-clean") {
		t.Errorf("expected BOM-stripped content in output:\n%s", output)
	}
}
