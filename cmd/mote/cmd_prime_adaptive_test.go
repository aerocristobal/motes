// SPDX-License-Identifier: MIT
// STORY-ADAPRIME-001 — Adaptive `mote prime` mode (MCP vs CLI), --mcp/--full
// overrides, --memories-only composition, mutex error, and detection
// robustness against malformed settings files.
package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/prime"
)

// withFakeHome isolates the test from the developer's real ~/.claude,
// ~/.codex, ~/.gemini contents so MCP-server detection sees only what
// the test seeds. Returns the tempdir path.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	orig, hadHome := os.LookupEnv("HOME")
	_ = os.Setenv("HOME", home)
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", orig)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})
	return home
}

// seedClaudeSettingsWithMote writes ~/.claude/settings.json declaring an
// mcpServers.mote entry under the given fake home.
func seedClaudeSettingsWithMote(t *testing.T, home string) {
	t.Helper()
	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `{"mcpServers": {"mote": {"command": "mote-mcp"}}}`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// resetAdaptivePrimeFlags clears all flags this test file may toggle so
// failed tests don't leak state into subsequent tests sharing the package.
func resetAdaptivePrimeFlags() {
	primeJSON = false
	primeHook = false
	primeMode = "startup"
	primeDebug = false
	primeMemoriesOnly = false
	primeMCP = false
	primeFull = false
}

// --- Scenario 1: auto-detection → MCP-mode brief payload. ---

func TestPrime_AutoDetect_MCPMode_TextOutput(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	seedClaudeSettingsWithMote(t, home)

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Should be hidden in MCP mode", Tags: []string{"x"}, Weight: 0.5},
	})
	seedMemory(t, root, "race", "use -race")

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})

	if !strings.Contains(output, wantDirectivePrefix) {
		t.Errorf("MCP-mode output missing truncation directive:\n%s", output)
	}
	if !strings.Contains(output, prime.MCPNoticeLine) {
		t.Errorf("MCP-mode output missing notice line:\n%s", output)
	}
	if !strings.Contains(output, "Persistent memories") {
		t.Errorf("MCP-mode output missing memories section:\n%s", output)
	}
	// MUST NOT contain CLI-mode markers (Scenario 1).
	for _, banned := range []string{"## Active work", "## Ready to start", "Should be hidden in MCP mode"} {
		if strings.Contains(output, banned) {
			t.Errorf("MCP-mode output should not contain %q:\n%s", banned, output)
		}
	}
}

func TestPrime_AutoDetect_MCPMode_JSONOutput(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	seedClaudeSettingsWithMote(t, home)

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Hidden in JSON MCP mode", Tags: []string{"x"}, Weight: 0.5},
	})
	seedMemory(t, root, "k", "v")

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
		t.Fatalf("invalid JSON: %v\noutput:\n%s", err, output)
	}
	if got.Mode != "mcp" {
		t.Errorf("Mode = %q, want \"mcp\"", got.Mode)
	}
	if got.ModeSource != "auto" {
		t.Errorf("ModeSource = %q, want \"auto\"", got.ModeSource)
	}
	if len(got.Memories) != 1 {
		t.Errorf("memories should be populated in MCP-mode JSON; got %d", len(got.Memories))
	}
	if len(got.ActiveTasks) != 0 {
		t.Errorf("active_tasks should be empty in MCP-mode JSON; got %d", len(got.ActiveTasks))
	}
}

// --- Scenario 1 (boundary): output token count stays within budget. ---

func TestPrime_AutoDetect_MCPMode_StaysWithinTokenBudget(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	seedClaudeSettingsWithMote(t, home)

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMemory(t, root, "race", "use -race")

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})

	tokens := prime.EstimateTokens(output)
	if tokens > prime.MCPModeTokenBudget {
		t.Errorf("MCP-mode output exceeds budget: %d > %d\noutput:\n%s",
			tokens, prime.MCPModeTokenBudget, output)
	}
}

// --- Scenario 2: no MCP detected → CLI mode (JSON has mode=cli, source=auto). ---

func TestPrime_NoDetect_CLIMode_JSONShape(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t) // empty HOME → no settings declare mote

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Visible task", Tags: []string{"x"}, Weight: 0.5},
	})

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
	var got PrimeOutput
	if err := json.Unmarshal([]byte(output[idx:]), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Mode != "cli" {
		t.Errorf("Mode = %q, want \"cli\"", got.Mode)
	}
	if got.ModeSource != "auto" {
		t.Errorf("ModeSource = %q, want \"auto\"", got.ModeSource)
	}
	if len(got.ActiveTasks) == 0 {
		t.Error("CLI mode should populate active_tasks")
	}
}

// --- Scenario 3: --mcp forces brief mode even without detection. ---

func TestPrime_McpFlag_ForcesMCPMode(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t) // no settings declare mote

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Should be hidden", Tags: []string{"x"}, Weight: 0.5},
	})

	primeMCP = true
	primeJSON = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --mcp --json: %v", err)
		}
	})
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON:\n%s", output)
	}
	var got PrimeOutput
	if err := json.Unmarshal([]byte(output[idx:]), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Mode != "mcp" {
		t.Errorf("Mode = %q, want \"mcp\"", got.Mode)
	}
	if got.ModeSource != "flag" {
		t.Errorf("ModeSource = %q, want \"flag\"", got.ModeSource)
	}
}

// --- Scenario 4: --full forces CLI mode even when detection would succeed. ---

func TestPrime_FullFlag_ForcesCLIMode(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	seedClaudeSettingsWithMote(t, home)

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Visible despite MCP wired", Tags: []string{"x"}, Weight: 0.5},
	})

	primeFull = true
	primeJSON = true
	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --full --json: %v", err)
		}
	})
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON:\n%s", output)
	}
	var got PrimeOutput
	if err := json.Unmarshal([]byte(output[idx:]), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Mode != "cli" {
		t.Errorf("Mode = %q, want \"cli\"", got.Mode)
	}
	if got.ModeSource != "flag" {
		t.Errorf("ModeSource = %q, want \"flag\"", got.ModeSource)
	}
	if len(got.ActiveTasks) == 0 {
		t.Error("--full should produce CLI payload with active_tasks populated")
	}
}

// --- Scenario 5: --memories-only collapses both modes; composition matrix. ---

func TestPrime_MemoriesOnly_ComposesWithModeMatrix(t *testing.T) {
	type row struct {
		name       string
		detected   bool
		mcpFlag    bool
		fullFlag   bool
		wantMode   string
		wantSource string
	}
	rows := []row{
		{"detected=yes, no flag", true, false, false, "mcp", "auto"},
		{"detected=no, no flag", false, false, false, "cli", "auto"},
		{"detected=yes, --mcp", true, true, false, "mcp", "flag"},
		{"detected=no, --mcp", false, true, false, "mcp", "flag"},
		{"detected=yes, --full", true, false, true, "cli", "flag"},
		{"detected=no, --full", false, false, true, "cli", "flag"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			defer resetAdaptivePrimeFlags()
			home := withFakeHome(t)
			if r.detected {
				seedClaudeSettingsWithMote(t, home)
			}
			root, cleanup := setupIntegrationTest(t)
			defer cleanup()
			seedMotes(t, root, []moteSpec{
				{Type: "task", Title: "Body should be hidden", Tags: []string{"x"}, Weight: 0.5},
				{Type: "decision", Title: "Recent decision", Body: "X"},
			})
			seedMemory(t, root, "k", "v")

			primeMemoriesOnly = true
			primeMCP = r.mcpFlag
			primeFull = r.fullFlag
			primeJSON = true

			output := captureStdout(func() {
				if err := primeCmd.RunE(primeCmd, nil); err != nil {
					t.Fatalf("prime: %v", err)
				}
			})
			idx := strings.Index(output, "{")
			if idx < 0 {
				t.Fatalf("no JSON:\n%s", output)
			}
			var got PrimeOutput
			if err := json.Unmarshal([]byte(output[idx:]), &got); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, output)
			}
			if got.Mode != r.wantMode {
				t.Errorf("Mode = %q, want %q", got.Mode, r.wantMode)
			}
			if got.ModeSource != r.wantSource {
				t.Errorf("ModeSource = %q, want %q", got.ModeSource, r.wantSource)
			}
			if len(got.Memories) != 1 {
				t.Errorf("memories should be populated; got %d", len(got.Memories))
			}
			if len(got.ActiveTasks) != 0 || len(got.Decisions) != 0 {
				t.Errorf("memories-only must suppress other arrays; got tasks=%d decisions=%d",
					len(got.ActiveTasks), len(got.Decisions))
			}
		})
	}
}

// --- Scenario 5 (text variant): --memories-only text output identical across modes. ---

func TestPrime_MemoriesOnly_TextOutput_IdenticalAcrossModes(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	collect := func(detected bool) string {
		t.Helper()
		// Reset between runs.
		resetAdaptivePrimeFlags()
		home := withFakeHome(t)
		if detected {
			seedClaudeSettingsWithMote(t, home)
		}
		root, cleanup := setupIntegrationTest(t)
		defer cleanup()
		seedMemory(t, root, "k", "v")
		primeMemoriesOnly = true
		return captureStdout(func() {
			if err := primeCmd.RunE(primeCmd, nil); err != nil {
				t.Fatalf("prime: %v", err)
			}
		})
	}
	got1 := collect(true)
	got2 := collect(false)
	if got1 != got2 {
		t.Errorf("--memories-only text output diverged across modes:\n--- detected=true ---\n%s\n--- detected=false ---\n%s", got1, got2)
	}
}

// --- Scenario 6: --mcp + --full is rejected before any prime output. ---

func TestPrime_McpAndFull_MutuallyExclusive(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	primeMCP = true
	primeFull = true

	// Capture both stdout and stderr.
	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	err := primeCmd.RunE(primeCmd, nil)
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	stdout, _ := io.ReadAll(rOut)
	stderrBytes, _ := io.ReadAll(rErr)

	if err == nil {
		t.Fatal("expected non-nil error from --mcp --full conflict")
	}
	if len(stdout) != 0 {
		t.Errorf("stdout should be empty on conflict; got %d bytes:\n%s", len(stdout), string(stdout))
	}
	// Error message must name both flags.
	msg := err.Error() + " " + string(stderrBytes)
	if !strings.Contains(msg, "--mcp") || !strings.Contains(msg, "--full") {
		t.Errorf("error/stderr should name both --mcp and --full; got error=%q stderr=%q", err.Error(), string(stderrBytes))
	}
}

// Same mutex check still triggers when --debug is set (flag-misuse is a
// developer error and bypasses the §23.4 silent-failure policy).
func TestPrime_McpAndFull_MutexNotSilencedByDebug(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	withFakeHome(t)
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	primeMCP = true
	primeFull = true
	primeDebug = true

	err := primeCmd.RunE(primeCmd, nil)
	if err == nil {
		t.Fatal("expected non-nil error from --mcp --full conflict even with --debug")
	}
}

// --- Scenario 7: malformed settings → CLI fallback (silent by default). ---

func TestPrime_MalformedSettings_FallsBackToCLI(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Visible CLI task", Tags: []string{"x"}, Weight: 0.5},
	})

	// Capture stderr to confirm silence under default (no --debug) behaviour.
	oldStderr := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime: %v", err)
		}
	})
	_ = wErr.Close()
	os.Stderr = oldStderr
	stderrBytes, _ := io.ReadAll(rErr)

	if !strings.Contains(output, "## Active work") {
		t.Errorf("expected CLI-mode output on malformed settings; got:\n%s", output)
	}
	if len(stderrBytes) != 0 {
		t.Errorf("default mode should be silent on parse failure; stderr:\n%s", string(stderrBytes))
	}
}

// --debug surfaces a one-line warning naming the broken path.
func TestPrime_MalformedSettings_DebugSurfacesWarning(t *testing.T) {
	defer resetAdaptivePrimeFlags()
	home := withFakeHome(t)
	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "T", Tags: []string{"x"}, Weight: 0.5},
	})

	primeDebug = true
	oldStderr := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr

	_ = captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --debug: %v", err)
		}
	})
	_ = wErr.Close()
	os.Stderr = oldStderr
	stderrBytes, _ := io.ReadAll(rErr)

	if !strings.Contains(string(stderrBytes), p) {
		t.Errorf("expected --debug stderr to name broken path %q; got:\n%s", p, string(stderrBytes))
	}
}

// --- Scenario 8 (smoke): docs section + README link exist. Compile-time
//     guarded via this test so a future doc rename surfaces immediately. ---

func TestDocs_AgentsGuideHasMotePrimeSizeBudgetSection(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(origCwd, "..", "..", "docs", "agents-guide.md"))
	if err != nil {
		t.Skipf("docs/agents-guide.md not found from cwd %q (test runs from cmd/mote/): %v", origCwd, err)
	}
	if !strings.Contains(string(data), "## `mote prime` size budget") {
		t.Errorf("docs/agents-guide.md missing '## `mote prime` size budget' section")
	}
}

func TestDocs_ReadmeLinksToSizeBudgetSection(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(origCwd, "..", "..", "README.md"))
	if err != nil {
		t.Skipf("README.md not found from cwd %q: %v", origCwd, err)
	}
	if !strings.Contains(string(data), "mote-prime-size-budget") {
		t.Errorf("README.md missing link to '#mote-prime-size-budget' in agents-guide.md")
	}
}
