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

// InvokeResult holds the response and estimated token usage from an LLM invocation.
type InvokeResult struct {
	Response     string
	InputTokens  int
	OutputTokens int
	Model        string
}

// Invoker is the contract every LLM backend must satisfy. The dream pipeline
// holds two of these — one for batch reasoning and one for reconciliation —
// and never type-asserts back to a concrete implementation.
//
// The tier parameter is a hint ("sonnet" for batch, "opus" for recon) carried
// over from the original Claude-only design. Single-model invokers (OpenAI,
// Gemini) ignore it; the orchestrator already picks the right invoker per stage.
type Invoker interface {
	Invoke(prompt string, tier string) (InvokeResult, error)
	Model() string
}

// NewInvoker constructs the right backend for entry.Backend. An empty backend
// resolves to claude-cli for back-compat with pre-multi-provider config files.
func NewInvoker(entry core.ProviderEntry, rateLimitRPM int) (Invoker, error) {
	switch entry.Backend {
	case "claude-cli", "":
		return NewClaudeInvoker(entry, rateLimitRPM), nil
	case "openai":
		return nil, fmt.Errorf("openai backend not yet implemented")
	case "gemini":
		return nil, fmt.Errorf("gemini backend not yet implemented")
	default:
		return nil, fmt.Errorf(
			"unknown dream provider backend %q (valid: claude-cli, openai, gemini)",
			entry.Backend)
	}
}

// filterEnv returns os.Environ() with the named variables removed.
func filterEnv(names ...string) []string {
	skip := make(map[string]bool, len(names))
	for _, n := range names {
		skip[n] = true
	}
	var env []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if !skip[k] {
			env = append(env, e)
		}
	}
	return env
}

// ClaudeInvoker shells out to the claude CLI for LLM operations.
type ClaudeInvoker struct {
	model       string
	timeout     time.Duration
	retryPolicy *RetryPolicy
	limiter     *RateLimiter
}

var _ Invoker = (*ClaudeInvoker)(nil)

// NewClaudeInvoker creates an invoker for one stage. The model defaults match
// the historical batch/recon defaults so a stage with an empty model is still
// usable: when constructed without an explicit model, callers should pass the
// stage-appropriate ProviderEntry (Batch or Reconciliation) — DefaultConfig
// already populates both.
func NewClaudeInvoker(entry core.ProviderEntry, rateLimitRPM int) *ClaudeInvoker {
	model := entry.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &ClaudeInvoker{
		model:       model,
		timeout:     5 * time.Minute,
		retryPolicy: DefaultRetryPolicy(),
		limiter:     NewRateLimiter(rateLimitRPM),
	}
}

// Model returns the configured model name.
func (ci *ClaudeInvoker) Model() string { return ci.model }

// Invoke runs the claude CLI with the given prompt. The tier parameter is
// accepted for interface compatibility and ignored — each invoker holds a
// single model now.
func (ci *ClaudeInvoker) Invoke(prompt string, tier string) (InvokeResult, error) {
	if err := ci.limiter.Wait(context.Background()); err != nil {
		return InvokeResult{}, fmt.Errorf("rate limiter: %w", err)
	}

	return Do(ci.retryPolicy, func() (InvokeResult, error) {
		return ci.invoke(prompt)
	})
}

func (ci *ClaudeInvoker) invoke(prompt string) (InvokeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ci.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude",
		"--model", ci.model,
		"--output-format", "text",
		"--print",
		"--system-prompt", "You are a JSON-only API. Respond with a single valid JSON object. No prose, no markdown.",
	)
	// Clear CLAUDECODE env var to allow nested invocation from within a Claude session.
	cmd.Env = filterEnv("CLAUDECODE")
	cmd.Stdin = strings.NewReader(prompt)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return InvokeResult{}, fmt.Errorf("claude timed out after %v", ci.timeout)
		}
		errMsg := stderr.String()
		if errMsg != "" {
			return InvokeResult{}, fmt.Errorf("claude invocation failed: %w: %s", err, errMsg)
		}
		return InvokeResult{}, fmt.Errorf("claude invocation failed: %w", err)
	}

	response := string(output)
	return InvokeResult{
		Response:     response,
		InputTokens:  EstimateTokens(prompt),
		OutputTokens: EstimateTokens(response),
		Model:        ci.model,
	}, nil
}
