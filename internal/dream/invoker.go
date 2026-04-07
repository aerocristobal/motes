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

// InvokeResult holds the response and estimated token usage from a Claude invocation.
type InvokeResult struct {
	Response     string
	InputTokens  int
	OutputTokens int
	Model        string
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
	batchModel  string
	reconModel  string
	timeout     time.Duration
	retryPolicy *RetryPolicy
	limiter     *RateLimiter
}

// NewClaudeInvoker creates an invoker with models from config.
func NewClaudeInvoker(cfg core.DreamProvider) *ClaudeInvoker {
	batchModel := cfg.Batch.Model
	if batchModel == "" {
		batchModel = "claude-sonnet-4-20250514"
	}
	reconModel := cfg.Reconciliation.Model
	if reconModel == "" {
		reconModel = "claude-opus-4-20250514"
	}
	return &ClaudeInvoker{
		batchModel:  batchModel,
		reconModel:  reconModel,
		timeout:     5 * time.Minute,
		retryPolicy: DefaultRetryPolicy(),
		limiter:     NewRateLimiter(cfg.RateLimitRPM),
	}
}

// Invoke runs the claude CLI with the given prompt and model tier.
func (ci *ClaudeInvoker) Invoke(prompt string, model string) (InvokeResult, error) {
	modelName := ci.batchModel
	if model == "opus" {
		modelName = ci.reconModel
	}

	// Rate limit before invoking
	if err := ci.limiter.Wait(context.Background()); err != nil {
		return InvokeResult{}, fmt.Errorf("rate limiter: %w", err)
	}

	return Do(ci.retryPolicy, func() (InvokeResult, error) {
		return ci.invoke(prompt, modelName)
	})
}

func (ci *ClaudeInvoker) invoke(prompt string, modelName string) (InvokeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ci.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude",
		"--model", modelName,
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
		Model:        modelName,
	}, nil
}
