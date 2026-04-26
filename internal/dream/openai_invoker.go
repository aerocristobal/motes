// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"motes/internal/core"
)

// openAIDefaultBaseURL is the production endpoint. Tests inject an
// httptest.Server URL through the unexported baseURL field on OpenAIInvoker.
const openAIDefaultBaseURL = "https://api.openai.com/v1"

// jsonOnlySystemPrompt is the system instruction shared with ClaudeInvoker.
// Defined once in this package so all backends emit identical instructions —
// the dream pipeline's parser depends on JSON-only output.
const jsonOnlySystemPrompt = "You are a JSON-only API. Respond with a single valid JSON object. No prose, no markdown."

// OpenAIInvoker calls the OpenAI Chat Completions API.
type OpenAIInvoker struct {
	apiKey      string
	model       string
	baseURL     string
	timeout     time.Duration
	httpClient  *http.Client
	retryPolicy *RetryPolicy
	limiter     *RateLimiter
}

var _ Invoker = (*OpenAIInvoker)(nil)

// NewOpenAIInvoker constructs an OpenAI-backed invoker for one stage. Returns
// an error if entry.Auth is missing or names an unset env var, or if
// entry.Model is empty.
func NewOpenAIInvoker(entry core.ProviderEntry, rateLimitRPM int) (*OpenAIInvoker, error) {
	apiKey, err := resolveAuth(entry.Auth)
	if err != nil {
		return nil, fmt.Errorf("openai auth: %w", err)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("openai auth: resolved value is empty (env var %q is set but empty?)", entry.Auth)
	}
	if entry.Model == "" {
		return nil, fmt.Errorf("openai backend requires provider.model to be set (e.g. gpt-4o)")
	}
	timeout := 5 * time.Minute
	return &OpenAIInvoker{
		apiKey:  apiKey,
		model:   entry.Model,
		baseURL: openAIDefaultBaseURL,
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryPolicy: DefaultRetryPolicy(),
		limiter:     NewRateLimiter(rateLimitRPM),
	}, nil
}

// Model returns the configured model name.
func (oi *OpenAIInvoker) Model() string { return oi.model }

// Invoke sends the prompt to the Chat Completions endpoint. The tier
// parameter is accepted for interface compatibility and ignored — each
// invoker holds a single model.
func (oi *OpenAIInvoker) Invoke(prompt string, tier string) (InvokeResult, error) {
	if err := oi.limiter.Wait(context.Background()); err != nil {
		return InvokeResult{}, fmt.Errorf("rate limiter: %w", err)
	}
	return Do(oi.retryPolicy, func() (InvokeResult, error) {
		return oi.invoke(prompt)
	})
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
}

type openAIChatChoice struct {
	Message openAIChatMessage `json:"message"`
}

type openAIChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openAIChatResponse struct {
	Model   string             `json:"model"`
	Choices []openAIChatChoice `json:"choices"`
	Usage   openAIChatUsage    `json:"usage"`
}

type openAIErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (oi *OpenAIInvoker) invoke(prompt string) (InvokeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), oi.timeout)
	defer cancel()

	body, err := json.Marshal(openAIChatRequest{
		Model: oi.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: jsonOnlySystemPrompt},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return InvokeResult{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oi.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return InvokeResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+oi.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := oi.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return InvokeResult{}, fmt.Errorf("openai request timed out after %v", oi.timeout)
		}
		return InvokeResult{}, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return InvokeResult{}, fmt.Errorf("read response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr openAIErrorBody
		_ = json.Unmarshal(respBody, &apiErr)
		detail := apiErr.Error.Message
		if detail == "" {
			detail = string(respBody)
		}
		return InvokeResult{}, fmt.Errorf("openai HTTP %d: %s", resp.StatusCode, detail)
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return InvokeResult{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return InvokeResult{}, fmt.Errorf("openai returned no choices")
	}

	model := parsed.Model
	if model == "" {
		model = oi.model
	}
	return InvokeResult{
		Response:     parsed.Choices[0].Message.Content,
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
		Model:        model,
	}, nil
}
