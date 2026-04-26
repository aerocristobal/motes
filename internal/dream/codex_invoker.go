// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"motes/internal/core"
)

// CodexInvoker shells out to the OpenAI Codex CLI's non-interactive `codex
// exec` subcommand. The CLI owns its own OAuth state (Sign in with ChatGPT or
// API key in ~/.codex/auth.json), so this backend inherits whatever the user
// has configured locally — no env-var auth handling lives here.
//
// The dream-cycle prompt is delivered on stdin (via `-` as the prompt
// positional, per `codex exec --help`) and Codex's last assistant message is
// captured via the `--output-last-message FILE` flag. Streaming the full
// JSONL event log (`--json`) and parsing it is more brittle: --output-last-
// message gives us exactly the assistant's final text in one read.
//
// Sandbox / approvals: --skip-git-repo-check lets us run outside git roots,
// and --full-auto + --dangerously-bypass-approvals-and-sandbox would relax
// approval prompts. Dream prompts are pure text-in/text-out (the model is
// instructed to emit JSON only), so we deliberately do NOT pass the
// dangerous-bypass flag here. If a user's config triggers a sandbox prompt,
// we want the run to fail loudly rather than silently auto-approving.
type CodexInvoker struct {
	model       string
	timeout     time.Duration
	retryPolicy *RetryPolicy
	limiter     *RateLimiter
}

var _ Invoker = (*CodexInvoker)(nil)

// NewCodexInvoker constructs a Codex-backed invoker. The auth field is a
// placeholder ("oauth" by convention, mirroring claude-cli) — the binary
// itself authenticates. An empty model is allowed and falls through to
// whatever the user has configured as their codex default.
func NewCodexInvoker(entry core.ProviderEntry, rateLimitRPM int) (*CodexInvoker, error) {
	if _, err := exec.LookPath("codex"); err != nil {
		return nil, fmt.Errorf("codex-cli backend requires the codex CLI on PATH: %w", err)
	}
	return &CodexInvoker{
		model:       entry.Model,
		timeout:     5 * time.Minute,
		retryPolicy: DefaultRetryPolicy(),
		limiter:     NewRateLimiter(rateLimitRPM),
	}, nil
}

// Model returns the configured model name; empty means "let codex pick its
// default".
func (ci *CodexInvoker) Model() string { return ci.model }

// Invoke runs the codex CLI with the given prompt. The tier parameter is
// accepted for interface compatibility and ignored.
func (ci *CodexInvoker) Invoke(prompt string, tier string) (InvokeResult, error) {
	if err := ci.limiter.Wait(context.Background()); err != nil {
		return InvokeResult{}, fmt.Errorf("rate limiter: %w", err)
	}
	return Do(ci.retryPolicy, func() (InvokeResult, error) {
		return ci.invoke(prompt)
	})
}

func (ci *CodexInvoker) invoke(prompt string) (InvokeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ci.timeout)
	defer cancel()

	// Codex doesn't have a `--system-prompt` flag, so prepend the JSON-only
	// directive to the prompt body. The model treats it as part of the user
	// message; the orchestrator's JSON parser is robust to a small amount of
	// preamble in the response anyway.
	fullPrompt := jsonOnlySystemPrompt + "\n\n" + prompt

	// Capture only the final assistant message via --output-last-message.
	// Using a tempfile avoids parsing the JSONL event stream.
	tmp, err := os.CreateTemp("", "codex-last-message-*.txt")
	if err != nil {
		return InvokeResult{}, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	args := []string{
		"exec",
		"--skip-git-repo-check",
		"--ephemeral",
		"--output-last-message", tmpPath,
	}
	if ci.model != "" {
		args = append(args, "-m", ci.model)
	}
	args = append(args, "-") // read prompt from stdin

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Stdin = strings.NewReader(fullPrompt)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if _, err := cmd.Output(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return InvokeResult{}, fmt.Errorf("codex timed out after %v", ci.timeout)
		}
		errMsg := stderr.String()
		if errMsg != "" {
			return InvokeResult{}, fmt.Errorf("codex invocation failed: %w: %s", err, errMsg)
		}
		return InvokeResult{}, fmt.Errorf("codex invocation failed: %w", err)
	}

	data, readErr := os.ReadFile(tmpPath)
	if readErr != nil {
		return InvokeResult{}, fmt.Errorf("read codex output: %w", readErr)
	}
	response := strings.TrimSpace(string(data))
	if response == "" {
		return InvokeResult{}, fmt.Errorf("codex produced no last-message output (stderr: %s)", stderr.String())
	}

	return InvokeResult{
		Response:     response,
		InputTokens:  EstimateTokens(fullPrompt),
		OutputTokens: EstimateTokens(response),
		Model:        ci.model,
	}, nil
}
