// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

func TestNewGeminiCLIInvoker_RequiresBinaryOnPath(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := NewGeminiCLIInvoker(core.ProviderEntry{Backend: "gemini-cli", Model: "gemini-2.5-flash"}, 0)
	if err == nil {
		t.Fatal("expected error when gemini binary is missing from PATH")
	}
	if !strings.Contains(err.Error(), "gemini CLI") {
		t.Errorf("error should mention the gemini CLI requirement: %v", err)
	}
}

func TestGeminiCLIInvoker_SuccessfulInvoke(t *testing.T) {
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	stubBinary(t, dir, "gemini", `
printf '%s\n' "$@" > `+argvLog+`
printf '%s' '{"visions":[{"id":"v1"}]}'
`)
	withStubPATH(t, dir)

	inv, err := NewGeminiCLIInvoker(core.ProviderEntry{Backend: "gemini-cli", Model: "gemini-2.5-flash"}, 0)
	if err != nil {
		t.Fatalf("NewGeminiCLIInvoker: %v", err)
	}
	inv.timeout = 5 * time.Second
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	res, err := inv.Invoke("hello world", "sonnet")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Response != `{"visions":[{"id":"v1"}]}` {
		t.Errorf("Response: got %q", res.Response)
	}
	if res.Model != "gemini-2.5-flash" {
		t.Errorf("Model: got %q", res.Model)
	}
	if res.InputTokens == 0 || res.OutputTokens == 0 {
		t.Errorf("token estimates should be non-zero: in=%d out=%d", res.InputTokens, res.OutputTokens)
	}

	argvBytes, _ := os.ReadFile(argvLog)
	argv := string(argvBytes)
	// Each arg is on its own line; assert the prompt arg contains both the
	// JSON-only directive and the user prompt.
	if !strings.Contains(argv, "-p") {
		t.Errorf("missing -p flag in argv: %s", argv)
	}
	if !strings.Contains(argv, "JSON-only") {
		t.Errorf("argv should include JSON-only directive in prompt: %s", argv)
	}
	if !strings.Contains(argv, "hello world") {
		t.Errorf("argv should include user prompt: %s", argv)
	}
	if !strings.Contains(argv, "-o\ntext\n") {
		t.Errorf("argv should include -o text: %s", argv)
	}
	if !strings.Contains(argv, "-m\ngemini-2.5-flash\n") {
		t.Errorf("argv should include -m flag with model: %s", argv)
	}
}

func TestGeminiCLIInvoker_NoModelOmitsFlag(t *testing.T) {
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	stubBinary(t, dir, "gemini", `
printf '%s\n' "$@" > `+argvLog+`
printf '%s' 'ok'
`)
	withStubPATH(t, dir)

	inv, err := NewGeminiCLIInvoker(core.ProviderEntry{Backend: "gemini-cli"}, 0)
	if err != nil {
		t.Fatalf("NewGeminiCLIInvoker: %v", err)
	}
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	if _, err := inv.Invoke("p", ""); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	argv, _ := os.ReadFile(argvLog)
	if strings.Contains(string(argv), "-m\n") {
		t.Errorf("expected no -m flag when model is empty; got argv=%q", string(argv))
	}
}

func TestGeminiCLIInvoker_NonZeroExitSurfacesStderr(t *testing.T) {
	dir := t.TempDir()
	stubBinary(t, dir, "gemini", `
echo "auth required" >&2
exit 1
`)
	withStubPATH(t, dir)

	inv, err := NewGeminiCLIInvoker(core.ProviderEntry{Backend: "gemini-cli", Model: "gemini-2.5-flash"}, 0)
	if err != nil {
		t.Fatalf("NewGeminiCLIInvoker: %v", err)
	}
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	_, invErr := inv.Invoke("p", "")
	if invErr == nil {
		t.Fatal("expected error when gemini exits non-zero")
	}
	if !strings.Contains(invErr.Error(), "auth required") {
		t.Errorf("error should surface stderr: %v", invErr)
	}
}

func TestGeminiCLIInvoker_EmptyOutputIsError(t *testing.T) {
	dir := t.TempDir()
	stubBinary(t, dir, "gemini", `exit 0`)
	withStubPATH(t, dir)

	inv, err := NewGeminiCLIInvoker(core.ProviderEntry{Backend: "gemini-cli", Model: "gemini-2.5-flash"}, 0)
	if err != nil {
		t.Fatalf("NewGeminiCLIInvoker: %v", err)
	}
	inv.retryPolicy = &RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Retryable: IsTransientError}

	_, invErr := inv.Invoke("p", "")
	if invErr == nil {
		t.Fatal("expected error when gemini produces no output")
	}
	if !strings.Contains(invErr.Error(), "no output") {
		t.Errorf("error should explain empty output: %v", invErr)
	}
}

func TestNewInvoker_DispatchesToGeminiCLI(t *testing.T) {
	dir := t.TempDir()
	stubBinary(t, dir, "gemini", `exit 0`)
	withStubPATH(t, dir)

	inv, err := NewInvoker(core.ProviderEntry{Backend: "gemini-cli", Model: "gemini-2.5-flash"}, 0)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	if _, ok := inv.(*GeminiCLIInvoker); !ok {
		t.Errorf("expected *GeminiCLIInvoker, got %T", inv)
	}
}
