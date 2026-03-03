package dream

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"motes/internal/core"
)

// ClaudeInvoker shells out to the claude CLI for LLM operations.
type ClaudeInvoker struct {
	batchModel string
	reconModel string
	timeout    time.Duration
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
		batchModel: batchModel,
		reconModel: reconModel,
		timeout:    5 * time.Minute,
	}
}

// Invoke runs the claude CLI with the given prompt and model tier.
func (ci *ClaudeInvoker) Invoke(prompt string, model string) (string, error) {
	modelName := ci.batchModel
	if model == "opus" {
		modelName = ci.reconModel
	}

	ctx, cancel := context.WithTimeout(context.Background(), ci.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude",
		"--model", modelName,
		"--output-format", "text",
		"--print",
		"--max-turns", "1",
	)
	cmd.Stdin = strings.NewReader(prompt)

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude timed out after %v", ci.timeout)
		}
		return "", fmt.Errorf("claude invocation failed: %w", err)
	}
	return string(output), nil
}
