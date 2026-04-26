// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"motes/internal/core"
)

// GeminiCLIInvoker shells out to the Gemini CLI's non-interactive prompt mode
// (`gemini -p <prompt> -o text -m <model>`). The CLI owns its own OAuth state
// (Login with Google → ~/.gemini/oauth_creds.json) or API key, so this
// backend inherits whatever the user has configured locally.
//
// This is the parallel option to the existing `gemini` backend, which talks
// to Vertex AI directly via gcloud ADC. The two coexist:
//   - `gemini` backend: HTTPS to Vertex AI; needs GCP project + gcloud.
//   - `gemini-cli` backend: subprocess to `gemini`; needs the CLI logged in.
//
// Gemini CLI does not expose a `--system-prompt` flag, so the JSON-only
// directive is prepended to the prompt body.
type GeminiCLIInvoker struct {
	model       string
	timeout     time.Duration
	retryPolicy *RetryPolicy
	limiter     *RateLimiter
}

var _ Invoker = (*GeminiCLIInvoker)(nil)

// NewGeminiCLIInvoker constructs a Gemini-CLI-backed invoker. The auth field
// is a placeholder ("oauth" by convention, mirroring claude-cli) — the binary
// itself authenticates.
func NewGeminiCLIInvoker(entry core.ProviderEntry, rateLimitRPM int) (*GeminiCLIInvoker, error) {
	if _, err := exec.LookPath("gemini"); err != nil {
		return nil, fmt.Errorf("gemini-cli backend requires the gemini CLI on PATH: %w", err)
	}
	return &GeminiCLIInvoker{
		model:       entry.Model,
		timeout:     5 * time.Minute,
		retryPolicy: DefaultRetryPolicy(),
		limiter:     NewRateLimiter(rateLimitRPM),
	}, nil
}

// Model returns the configured model name; empty means "let gemini pick its
// default".
func (gi *GeminiCLIInvoker) Model() string { return gi.model }

// Invoke runs the gemini CLI in non-interactive mode. The tier parameter is
// accepted for interface compatibility and ignored.
func (gi *GeminiCLIInvoker) Invoke(prompt string, tier string) (InvokeResult, error) {
	if err := gi.limiter.Wait(context.Background()); err != nil {
		return InvokeResult{}, fmt.Errorf("rate limiter: %w", err)
	}
	return Do(gi.retryPolicy, func() (InvokeResult, error) {
		return gi.invoke(prompt)
	})
}

func (gi *GeminiCLIInvoker) invoke(prompt string) (InvokeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gi.timeout)
	defer cancel()

	// Gemini CLI has no --system-prompt; inline the JSON-only directive at
	// the head of the prompt body.
	fullPrompt := jsonOnlySystemPrompt + "\n\n" + prompt

	args := []string{
		"-p", fullPrompt,
		"-o", "text",
	}
	if gi.model != "" {
		args = append(args, "-m", gi.model)
	}

	cmd := exec.CommandContext(ctx, "gemini", args...)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return InvokeResult{}, fmt.Errorf("gemini timed out after %v", gi.timeout)
		}
		errMsg := stderr.String()
		if errMsg != "" {
			return InvokeResult{}, fmt.Errorf("gemini invocation failed: %w: %s", err, errMsg)
		}
		return InvokeResult{}, fmt.Errorf("gemini invocation failed: %w", err)
	}

	response := strings.TrimSpace(string(output))
	if response == "" {
		return InvokeResult{}, fmt.Errorf("gemini produced no output (stderr: %s)", stderr.String())
	}

	return InvokeResult{
		Response:     response,
		InputTokens:  EstimateTokens(fullPrompt),
		OutputTokens: EstimateTokens(response),
		Model:        gi.model,
	}, nil
}
