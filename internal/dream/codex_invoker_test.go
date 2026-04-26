// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

// stubBinary writes an executable shell script at <dir>/<name> that runs the
// provided body. It is the test-side counterpart to exec.LookPath: by
// prepending dir to PATH we make `exec.CommandContext("name", ...)` resolve to
// our fake instead of any real binary on the developer machine.
func stubBinary(t *testing.T, dir, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH-stub tests assume POSIX shell; not supported on windows")
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
}

// withStubPATH prepends dir to PATH for the duration of the test.
func withStubPATH(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", dir+string(os.PathListSeparator)+orig)
}

func TestNewCodexInvoker_RequiresBinaryOnPath(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := NewCodexInvoker(core.ProviderEntry{Backend: "codex-cli", Model: "gpt-5-codex"}, 0)
	if err == nil {
		t.Fatal("expected error when codex binary is missing from PATH")
	}
	if !strings.Contains(err.Error(), "codex CLI") {
		t.Errorf("error should mention the codex CLI requirement: %v", err)
	}
}

func TestCodexInvoker_SuccessfulInvoke(t *testing.T) {
	dir := t.TempDir()
	// The stub writes a fixed JSON payload to whatever path is given via
	// --output-last-message FILE, then exits 0. It also captures argv and
	// stdin to a sibling file so the test can assert on them.
	argvLog := filepath.Join(dir, "argv.log")
	stdinLog := filepath.Join(dir, "stdin.log")
	stubBinary(t, dir, "codex", `
echo "$@" > `+argvLog+`
cat > `+stdinLog+`
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--output-last-message" ]; then out="$a"; fi
  prev="$a"
done
printf '%s' '{"visions":[{"id":"v1"}]}' > "$out"
`)
	withStubPATH(t, dir)

	inv, err := NewCodexInvoker(core.ProviderEntry{Backend: "codex-cli", Model: "gpt-5-codex"}, 0)
	if err != nil {
		t.Fatalf("NewCodexInvoker: %v", err)
	}
	// Tighten timeout for fast test failure.
	inv.timeout = 5 * time.Second
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	res, err := inv.Invoke("hello world", "sonnet")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Response != `{"visions":[{"id":"v1"}]}` {
		t.Errorf("Response: got %q", res.Response)
	}
	if res.Model != "gpt-5-codex" {
		t.Errorf("Model: got %q", res.Model)
	}
	if res.InputTokens == 0 || res.OutputTokens == 0 {
		t.Errorf("token estimates should be non-zero: in=%d out=%d", res.InputTokens, res.OutputTokens)
	}

	argv, _ := os.ReadFile(argvLog)
	argvStr := string(argv)
	for _, want := range []string{"exec", "--skip-git-repo-check", "--ephemeral", "--output-last-message", "-m", "gpt-5-codex", "-"} {
		if !strings.Contains(argvStr, want) {
			t.Errorf("argv missing %q: %s", want, argvStr)
		}
	}
	stdin, _ := os.ReadFile(stdinLog)
	if !strings.Contains(string(stdin), "hello world") {
		t.Errorf("stdin should contain user prompt: %q", string(stdin))
	}
	if !strings.Contains(string(stdin), "JSON-only") {
		t.Errorf("stdin should contain JSON-only system directive: %q", string(stdin))
	}
}

func TestCodexInvoker_NoModelOmitsFlag(t *testing.T) {
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	stubBinary(t, dir, "codex", `
echo "$@" > `+argvLog+`
cat > /dev/null
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--output-last-message" ]; then out="$a"; fi
  prev="$a"
done
printf '%s' 'ok' > "$out"
`)
	withStubPATH(t, dir)

	inv, err := NewCodexInvoker(core.ProviderEntry{Backend: "codex-cli"}, 0)
	if err != nil {
		t.Fatalf("NewCodexInvoker: %v", err)
	}
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	if _, err := inv.Invoke("p", ""); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	argv, _ := os.ReadFile(argvLog)
	if strings.Contains(string(argv), "-m ") {
		t.Errorf("expected no -m flag when model is empty; got argv=%q", string(argv))
	}
}

func TestCodexInvoker_NonZeroExitSurfacesStderr(t *testing.T) {
	dir := t.TempDir()
	stubBinary(t, dir, "codex", `
echo "boom" >&2
exit 7
`)
	withStubPATH(t, dir)

	inv, err := NewCodexInvoker(core.ProviderEntry{Backend: "codex-cli", Model: "gpt-5-codex"}, 0)
	if err != nil {
		t.Fatalf("NewCodexInvoker: %v", err)
	}
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	_, invErr := inv.Invoke("p", "")
	if invErr == nil {
		t.Fatal("expected error when codex exits non-zero")
	}
	if !strings.Contains(invErr.Error(), "boom") {
		t.Errorf("error should surface stderr: %v", invErr)
	}
}

func TestCodexInvoker_EmptyOutputIsError(t *testing.T) {
	dir := t.TempDir()
	// Write an empty file to the output path, exit 0.
	stubBinary(t, dir, "codex", `
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--output-last-message" ]; then out="$a"; fi
  prev="$a"
done
: > "$out"
`)
	withStubPATH(t, dir)

	inv, err := NewCodexInvoker(core.ProviderEntry{Backend: "codex-cli", Model: "gpt-5-codex"}, 0)
	if err != nil {
		t.Fatalf("NewCodexInvoker: %v", err)
	}
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	_, invErr := inv.Invoke("p", "")
	if invErr == nil {
		t.Fatal("expected error when codex produces no output")
	}
	if !strings.Contains(invErr.Error(), "no last-message") {
		t.Errorf("error should explain empty output: %v", invErr)
	}
}

func TestNewInvoker_DispatchesToCodex(t *testing.T) {
	dir := t.TempDir()
	stubBinary(t, dir, "codex", `exit 0`)
	withStubPATH(t, dir)

	inv, err := NewInvoker(core.ProviderEntry{Backend: "codex-cli", Model: "gpt-5-codex"}, 0)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	if _, ok := inv.(*CodexInvoker); !ok {
		t.Errorf("expected *CodexInvoker, got %T", inv)
	}
}
