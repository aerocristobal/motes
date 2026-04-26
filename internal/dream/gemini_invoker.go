// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"motes/internal/core"
)

// vertexAIAuthValue is the sentinel value users put in provider.auth to opt
// into Vertex AI Application Default Credentials (gcloud OAuth tokens). This
// is the only auth mode supported in the first multi-provider release; an
// API-key flow against generativelanguage.googleapis.com may follow later.
const vertexAIAuthValue = "vertex-ai"

// defaultGeminiRegion is used when provider.options.gcp_region is unset.
const defaultGeminiRegion = "us-central1"

// defaultGeminiSafetyThreshold is permissive enough not to false-positive on
// most technical content while still filtering egregious material.
const defaultGeminiSafetyThreshold = "BLOCK_ONLY_HIGH"

// validGeminiSafetyThresholds matches Vertex AI's accepted enum values.
var validGeminiSafetyThresholds = map[string]bool{
	"BLOCK_NONE":             true,
	"BLOCK_ONLY_HIGH":        true,
	"BLOCK_MEDIUM_AND_ABOVE": true,
	"BLOCK_LOW_AND_ABOVE":    true,
}

// safetyBlockErrorPrefix is matched by IsTransientError to mark safety blocks
// as non-retryable. Keep in sync with the error returned in invoke().
const safetyBlockErrorPrefix = "gemini response blocked"

// geminiTokenSource produces a fresh OAuth bearer token for each invocation.
// In production this shells out to `gcloud auth print-access-token`; tests
// substitute a no-op closure.
type geminiTokenSource func(ctx context.Context) (string, error)

// GeminiInvoker calls Vertex AI's Gemini generateContent API.
type GeminiInvoker struct {
	project         string
	region          string
	model           string
	safetyThreshold string
	baseURL         string // overridable for tests
	timeout         time.Duration
	httpClient      *http.Client
	retryPolicy     *RetryPolicy
	limiter         *RateLimiter
	tokenSource     geminiTokenSource
}

var _ Invoker = (*GeminiInvoker)(nil)

// NewGeminiInvoker constructs a Vertex AI Gemini invoker. Returns an error if
// auth isn't "vertex-ai", required options are missing, gcloud isn't on PATH,
// or the requested safety threshold is unknown.
func NewGeminiInvoker(entry core.ProviderEntry, rateLimitRPM int) (*GeminiInvoker, error) {
	if entry.Auth != vertexAIAuthValue {
		return nil, fmt.Errorf(
			"gemini backend currently supports only auth=%q (Vertex AI ADC); got %q",
			vertexAIAuthValue, entry.Auth)
	}
	if entry.Model == "" {
		return nil, fmt.Errorf("gemini backend requires provider.model (e.g. gemini-2.5-flash)")
	}
	project := entry.Options["gcp_project"]
	if project == "" {
		return nil, fmt.Errorf("gemini backend requires options.gcp_project")
	}
	region := entry.Options["gcp_region"]
	if region == "" {
		region = defaultGeminiRegion
	}
	threshold := entry.Options["safety_threshold"]
	if threshold == "" {
		threshold = defaultGeminiSafetyThreshold
	} else if !validGeminiSafetyThresholds[threshold] {
		return nil, fmt.Errorf(
			"gemini safety_threshold %q invalid; valid values: BLOCK_NONE, BLOCK_ONLY_HIGH, BLOCK_MEDIUM_AND_ABOVE, BLOCK_LOW_AND_ABOVE",
			threshold)
	}
	// gcloud is checked LAST so the more specific config-shape errors above
	// surface first on dev machines without gcloud installed.
	if _, err := exec.LookPath("gcloud"); err != nil {
		return nil, fmt.Errorf("gemini auth=vertex-ai requires gcloud CLI on PATH: %w", err)
	}
	timeout := 5 * time.Minute
	return &GeminiInvoker{
		project:         project,
		region:          region,
		model:           entry.Model,
		safetyThreshold: threshold,
		baseURL:         vertexAIBaseURL(region),
		timeout:         timeout,
		httpClient:      &http.Client{Timeout: timeout},
		retryPolicy:     DefaultRetryPolicy(),
		limiter:         NewRateLimiter(rateLimitRPM),
		tokenSource:     gcloudPrintAccessToken,
	}, nil
}

// vertexAIBaseURL returns the regional Vertex AI endpoint base.
func vertexAIBaseURL(region string) string {
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com", region)
}

// Model returns the configured model name.
func (gi *GeminiInvoker) Model() string { return gi.model }

// Invoke sends the prompt to Vertex AI's generateContent endpoint.
func (gi *GeminiInvoker) Invoke(prompt string, tier string) (InvokeResult, error) {
	if err := gi.limiter.Wait(context.Background()); err != nil {
		return InvokeResult{}, fmt.Errorf("rate limiter: %w", err)
	}
	return Do(gi.retryPolicy, func() (InvokeResult, error) {
		return gi.invoke(prompt)
	})
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiGenerationConfig struct {
	ResponseMIMEType string `json:"responseMimeType,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
	SafetySettings    []geminiSafetySetting  `json:"safetySettings,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

type geminiErrorEnvelope struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func (gi *GeminiInvoker) invoke(prompt string) (InvokeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gi.timeout)
	defer cancel()

	token, err := gi.tokenSource(ctx)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("gemini auth: %w", err)
	}
	if token == "" {
		return InvokeResult{}, fmt.Errorf("gemini auth: gcloud returned empty access token")
	}

	body, err := json.Marshal(geminiRequest{
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: prompt}}},
		},
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: jsonOnlySystemPrompt}},
		},
		GenerationConfig: geminiGenerationConfig{ResponseMIMEType: "application/json"},
		SafetySettings: []geminiSafetySetting{
			{"HARM_CATEGORY_HARASSMENT", gi.safetyThreshold},
			{"HARM_CATEGORY_HATE_SPEECH", gi.safetyThreshold},
			{"HARM_CATEGORY_SEXUALLY_EXPLICIT", gi.safetyThreshold},
			{"HARM_CATEGORY_DANGEROUS_CONTENT", gi.safetyThreshold},
		},
	})
	if err != nil {
		return InvokeResult{}, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		gi.baseURL, gi.project, gi.region, gi.model)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return InvokeResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := gi.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return InvokeResult{}, fmt.Errorf("gemini request timed out after %v", gi.timeout)
		}
		return InvokeResult{}, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return InvokeResult{}, fmt.Errorf("read response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr geminiErrorEnvelope
		_ = json.Unmarshal(respBody, &apiErr)
		detail := apiErr.Error.Message
		if detail == "" {
			detail = string(respBody)
		}
		return InvokeResult{}, fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, detail)
	}

	var parsed geminiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return InvokeResult{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Candidates) == 0 {
		return InvokeResult{}, fmt.Errorf("gemini returned no candidates")
	}
	cand := parsed.Candidates[0]
	if cand.FinishReason == "SAFETY" || cand.FinishReason == "RECITATION" || cand.FinishReason == "PROHIBITED_CONTENT" {
		return InvokeResult{}, fmt.Errorf("%s: %s", safetyBlockErrorPrefix, cand.FinishReason)
	}
	if len(cand.Content.Parts) == 0 {
		return InvokeResult{}, fmt.Errorf("gemini candidate has no content parts (finishReason=%s)", cand.FinishReason)
	}

	return InvokeResult{
		Response:     cand.Content.Parts[0].Text,
		InputTokens:  parsed.UsageMetadata.PromptTokenCount,
		OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		Model:        gi.model,
	}, nil
}

// gcloudPrintAccessToken shells out to `gcloud auth print-access-token`. The
// access token is short-lived (~1 hour); we fetch fresh on each invocation
// because dream cycles run infrequently and gcloud caches the token itself.
func gcloudPrintAccessToken(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token").Output()
	if err != nil {
		return "", fmt.Errorf("gcloud auth print-access-token failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
