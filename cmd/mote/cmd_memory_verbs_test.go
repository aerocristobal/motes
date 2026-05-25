// SPDX-License-Identifier: MIT
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

// readAuditEntries reads .memory/audit.jsonl from the given root.
// Returns nil if the file does not exist.
func readAuditEntries(t *testing.T, root string) []core.AuditEntry {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "audit.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("open audit.jsonl: %v", err)
	}
	defer func() { _ = f.Close() }()
	var entries []core.AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e core.AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err == nil {
			entries = append(entries, e)
		}
	}
	return entries
}

// captureOutputs runs fn with os.Stdout and os.Stderr swapped for pipes;
// returns the captured stdout, stderr, and the error fn returned.
func captureOutputs(fn func() error) (stdout, stderr string, err error) {
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	err = fn()
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr
	sb, _ := io.ReadAll(rOut)
	eb, _ := io.ReadAll(rErr)
	return string(sb), string(eb), err
}

// resetRememberFlags returns a defer-able cleanup that restores the
// package-level cobra flag globals after a test mutates them.
func resetRememberFlags(t *testing.T) func() {
	t.Helper()
	return func() {
		rememberKey = ""
		rememberForce = false
		rememberNoClobber = false
		rememberQuiet = false
		rememberJSON = false
	}
}

// callRemember invokes the cobra command end-to-end (Args validation and
// flag parsing included), the way the binary would.
func callRemember(args ...string) (stdout, stderr string, err error) {
	full := append([]string{"remember"}, args...)
	rootCmd.SetArgs(full)
	return captureOutputs(func() error {
		return rootCmd.Execute()
	})
}

func callRecall(args ...string) (stdout, stderr string, err error) {
	full := append([]string{"recall"}, args...)
	rootCmd.SetArgs(full)
	return captureOutputs(func() error {
		return rootCmd.Execute()
	})
}

func callForget(args ...string) (stdout, stderr string, err error) {
	full := append([]string{"forget"}, args...)
	rootCmd.SetArgs(full)
	return captureOutputs(func() error {
		return rootCmd.Execute()
	})
}

func callMemories(args ...string) (stdout, stderr string, err error) {
	full := append([]string{"memories"}, args...)
	rootCmd.SetArgs(full)
	return captureOutputs(func() error {
		return rootCmd.Execute()
	})
}

// Scenario 1: Save with auto-derived key.
func TestRemember_AutoKey_Persists(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	stdout, _, err := callRemember("always run tests with -race flag")
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if !strings.Contains(stdout, "always-run-tests-with-race-flag") {
		t.Errorf("stdout missing slug: %q", stdout)
	}
	rec, err := core.NewMemoryStore(root).Get("always-run-tests-with-race-flag")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Body != "always run tests with -race flag" {
		t.Errorf("body: %q", rec.Body)
	}
}

// Scenario 2: Save with explicit --key.
func TestRemember_ExplicitKey(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	_, _, err := callRemember("auth uses JWT not sessions", "--key", "auth-jwt")
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	rec, err := core.NewMemoryStore(root).Get("auth-jwt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Body != "auth uses JWT not sessions" {
		t.Errorf("body: %q", rec.Body)
	}
}

// Scenario 2b: --json output.
func TestRemember_JSONOutput(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	stdout, _, err := callRemember("auth uses JWT not sessions", "--key", "auth-jwt", "--json")
	if err != nil {
		t.Fatalf("remember --json: %v", err)
	}
	var resp struct {
		Key       string `json:"key"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &resp); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if resp.Key != "auth-jwt" {
		t.Errorf("key: %q", resp.Key)
	}
	if resp.CreatedAt == "" {
		t.Error("created_at must be set")
	}
	if _, perr := time.Parse(time.RFC3339, resp.CreatedAt); perr != nil {
		t.Errorf("created_at %q is not RFC3339: %v", resp.CreatedAt, perr)
	}
}

// Scenario 3: List all memories, sorted by key.
func TestMemories_ListAllSorted(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	for _, kv := range []struct{ k, v string }{
		{"auth-jwt", "auth uses JWT not sessions"},
		{"race-flag", "always run tests with -race flag"},
		{"dolt-required", "dolt must be installed before tests"},
	} {
		if _, _, err := callRemember(kv.v, "--key", kv.k); err != nil {
			t.Fatalf("seed %s: %v", kv.k, err)
		}
	}

	stdout, _, err := callMemories()
	if err != nil {
		t.Fatalf("memories: %v", err)
	}
	idxAuth := strings.Index(stdout, "auth-jwt")
	idxDolt := strings.Index(stdout, "dolt-required")
	idxRace := strings.Index(stdout, "race-flag")
	if idxAuth < 0 || idxDolt < 0 || idxRace < 0 {
		t.Fatalf("missing keys in output:\n%s", stdout)
	}
	if idxAuth >= idxDolt || idxDolt >= idxRace {
		t.Errorf("not sorted ascending: auth=%d dolt=%d race=%d\nOutput:\n%s",
			idxAuth, idxDolt, idxRace, stdout)
	}
}

// Scenario 4: Substring filter.
func TestMemories_SubstringFilter(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	for _, kv := range []struct{ k, v string }{
		{"auth-jwt", "auth uses JWT"},
		{"race-flag", "always run tests with -race flag"},
		{"dolt-required", "dolt must be installed before tests"},
	} {
		if _, _, err := callRemember(kv.v, "--key", kv.k); err != nil {
			t.Fatalf("seed %s: %v", kv.k, err)
		}
	}

	stdout, _, err := callMemories("dolt")
	if err != nil {
		t.Fatalf("memories dolt: %v", err)
	}
	if !strings.Contains(stdout, "dolt-required") {
		t.Errorf("expected dolt-required, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "auth-jwt") || strings.Contains(stdout, "race-flag") {
		t.Errorf("substring leak: %s", stdout)
	}
}

// Scenario 5: Recall happy path.
func TestRecall_HappyPath(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")
	if _, _, err := callRemember("auth uses JWT not sessions", "--key", "auth-jwt"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stdout, _, err := callRecall("auth-jwt")
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if strings.TrimRight(stdout, "\n") != "auth uses JWT not sessions" {
		t.Errorf("body mismatch: %q", stdout)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("expected trailing newline: %q", stdout)
	}
}

// Scenario 6: Forget happy path + audit entry.
func TestForget_HappyPath_AuditLogged(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")
	if _, _, err := callRemember("stale", "--key", "stale-rule"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, _, err := callForget("stale-rule")
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if core.NewMemoryStore(root).Has("stale-rule") {
		t.Error("memory should be deleted")
	}

	entries := readAuditEntries(t, root)
	var seenDel bool
	for _, e := range entries {
		if e.Operation == "memory.delete" && e.MoteID == "stale-rule" {
			seenDel = true
			break
		}
	}
	if !seenDel {
		t.Errorf("missing memory.delete audit entry; got: %+v", entries)
	}
}

// Scenario 9: Empty body rejected.
func TestRemember_EmptyBodyRejected(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	_, _, err := callRemember("")
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "empty") {
		t.Errorf("error should mention empty: %v", err)
	}
}

// Scenario 10: Unknown-key recall/forget exit code 2.
func TestRecall_UnknownKey_Exit2(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	_, _, err := callRecall("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("exit code: want 2, got %d", ec.code)
	}
	if !strings.Contains(ec.err.Error(), "not found") {
		t.Errorf("error should say 'not found': %v", ec.err)
	}
}

func TestForget_UnknownKey_Exit2(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	_, _, err := callForget("already-gone")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("exit code: want 2, got %d", ec.code)
	}
}

// Scenario 11: Security scan blocks; --force bypasses.
func TestRemember_SecurityScan_BlocksAndForceBypasses(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	// The blockPatterns regex requires the literal "-----BEGIN PRIVATE KEY-----"
	// substring; the leading "leaked: " prefix sidesteps cobra's flag parser
	// (which would otherwise interpret a leading "--" as a malformed flag).
	poisoned := "leaked: -----BEGIN PRIVATE KEY-----\nABCDEF\n-----END PRIVATE KEY-----"

	_, _, err := callRemember(poisoned, "--key", "secret")
	if err == nil {
		t.Fatal("expected security block without --force")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "blocked") &&
		!strings.Contains(strings.ToLower(err.Error()), "private key") {
		t.Errorf("error should mention block reason: %v", err)
	}
	if core.NewMemoryStore(root).Has("secret") {
		t.Error("memory should not be persisted when blocked")
	}

	_, _, err = callRemember(poisoned, "--key", "secret", "--force")
	if err != nil {
		t.Fatalf("--force should bypass scan: %v", err)
	}
	if !core.NewMemoryStore(root).Has("secret") {
		t.Error("memory should be persisted with --force")
	}
}

// --no-clobber + duplicate key fails. Not in BDD scenarios but is in §6 / Q2.
func TestRemember_NoClobber_FailsOnDuplicate(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()
	t.Setenv("MOTE_AGENT_ID", "claude-code")

	if _, _, err := callRemember("first", "--key", "k"); err != nil {
		t.Fatalf("first remember: %v", err)
	}
	_, _, err := callRemember("second", "--key", "k", "--no-clobber")
	if err == nil {
		t.Fatal("expected duplicate-key error with --no-clobber")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention duplicate: %v", err)
	}
}

// Memories --json empty store returns {"memories":[]}.
func TestMemories_JSON_EmptyStore(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetRememberFlags(t)()

	stdout, _, err := callMemories("--json")
	if err != nil {
		t.Fatalf("memories --json: %v", err)
	}
	stdout = strings.TrimSpace(stdout)
	if stdout != `{"memories":[]}` {
		t.Errorf("empty-store JSON: want '{\"memories\":[]}', got %q", stdout)
	}
}
