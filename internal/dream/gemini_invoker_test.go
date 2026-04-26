// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"motes/internal/core"
)

// gcloudOnPath reports whether the gcloud binary is available — used by
// constructor-validation tests that exercise the gcloud-gating itself.
func gcloudOnPath(t *testing.T) bool {
	t.Helper()
	_, err := exec.LookPath("gcloud")
	return err == nil
}

// newTestGeminiInvoker builds a GeminiInvoker pointed at srv with a fake
// tokenSource. It skips the production NewGeminiInvoker constructor (and
// therefore the gcloud-on-PATH check), since tests inject their own
// tokenSource and never need to call gcloud at runtime. This keeps the
// invoker logic fully exercised on dev machines without gcloud installed.
func newTestGeminiInvoker(t *testing.T, srv *httptest.Server) *GeminiInvoker {
	t.Helper()
	timeout := 2 * time.Second
	return &GeminiInvoker{
		project:         "test-project",
		region:          "us-central1",
		model:           "gemini-2.5-flash",
		safetyThreshold: defaultGeminiSafetyThreshold,
		baseURL:         srv.URL,
		timeout:         timeout,
		httpClient:      &http.Client{Timeout: timeout},
		retryPolicy: &RetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Millisecond,
			MaxDelay:    2 * time.Millisecond,
			Retryable:   IsTransientError,
		},
		limiter:     NewRateLimiter(0),
		tokenSource: func(ctx context.Context) (string, error) { return "ya29.test-token", nil },
	}
}

func geminiOKResponse(text string) []byte {
	body, _ := json.Marshal(geminiResponse{
		Candidates: []geminiCandidate{{
			Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: text}}},
			FinishReason: "STOP",
		}},
		UsageMetadata: geminiUsageMetadata{PromptTokenCount: 7, CandidatesTokenCount: 11},
	})
	return body
}

func TestGeminiInvoker_SuccessfulRequest(t *testing.T) {
	var capturedURL string
	var capturedReq geminiRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer ya29.test-token" {
			t.Errorf("Authorization: got %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedReq)
		w.WriteHeader(http.StatusOK)
		w.Write(geminiOKResponse(`{"visions":[{"id":"v1"}]}`))
	}))
	defer srv.Close()

	inv := newTestGeminiInvoker(t, srv)
	res, err := inv.Invoke("hello", "sonnet")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Response != `{"visions":[{"id":"v1"}]}` {
		t.Errorf("Response: got %q", res.Response)
	}
	if res.InputTokens != 7 || res.OutputTokens != 11 {
		t.Errorf("tokens: got input=%d output=%d, want 7/11", res.InputTokens, res.OutputTokens)
	}
	if res.Model != "gemini-2.5-flash" {
		t.Errorf("Model: got %q", res.Model)
	}
	wantPath := "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.5-flash:generateContent"
	if capturedURL != wantPath {
		t.Errorf("URL path: got %q, want %q", capturedURL, wantPath)
	}
	if capturedReq.SystemInstruction == nil || len(capturedReq.SystemInstruction.Parts) == 0 ||
		!strings.Contains(capturedReq.SystemInstruction.Parts[0].Text, "JSON-only") {
		t.Errorf("system instruction missing JSON-only directive: %+v", capturedReq.SystemInstruction)
	}
	if len(capturedReq.SafetySettings) != 4 {
		t.Errorf("expected 4 safety settings, got %d", len(capturedReq.SafetySettings))
	}
	for _, s := range capturedReq.SafetySettings {
		if s.Threshold != defaultGeminiSafetyThreshold {
			t.Errorf("safety threshold for %s: got %s", s.Category, s.Threshold)
		}
	}
}

func TestGeminiInvoker_SafetyBlockIsNonRetryable(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := json.Marshal(geminiResponse{
			Candidates: []geminiCandidate{{FinishReason: "SAFETY"}},
		})
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	inv := newTestGeminiInvoker(t, srv)
	_, err := inv.Invoke("p", "sonnet")
	if err == nil {
		t.Fatal("expected error for SAFETY finishReason")
	}
	if !strings.Contains(err.Error(), safetyBlockErrorPrefix) {
		t.Errorf("error should contain safety block prefix: %v", err)
	}
	if calls != 1 {
		t.Errorf("safety block must not be retried; got %d attempts", calls)
	}
}

func TestGeminiInvoker_RetriesOn429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(geminiOKResponse(`{"ok":true}`))
	}))
	defer srv.Close()

	inv := newTestGeminiInvoker(t, srv)
	if _, err := inv.Invoke("p", "sonnet"); err != nil {
		t.Fatalf("Invoke after retry: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 attempts, got %d", calls)
	}
}

func TestGeminiInvoker_TokenSourceErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called when token retrieval fails")
	}))
	defer srv.Close()

	inv := newTestGeminiInvoker(t, srv)
	inv.tokenSource = func(ctx context.Context) (string, error) {
		return "", io.ErrUnexpectedEOF
	}
	_, err := inv.Invoke("p", "sonnet")
	if err == nil {
		t.Fatal("expected error when tokenSource fails")
	}
	if !strings.Contains(err.Error(), "gemini auth") {
		t.Errorf("error should mention gemini auth: %v", err)
	}
}

// These constructor-validation tests intentionally trip checks that run BEFORE
// the gcloud-on-PATH check, so they pass without gcloud installed. Tests that
// exercise the gcloud branch itself live below and skip gracefully.

func TestNewGeminiInvoker_RejectsNonVertexAuth(t *testing.T) {
	_, err := NewGeminiInvoker(core.ProviderEntry{
		Backend: "gemini",
		Auth:    "GEMINI_API_KEY", // API-key path not yet supported
		Model:   "gemini-2.5-flash",
		Options: map[string]string{"gcp_project": "p"},
	}, 0)
	if err == nil {
		t.Fatal("expected error for non-vertex auth value")
	}
	if !strings.Contains(err.Error(), "vertex-ai") {
		t.Errorf("error should explain vertex-ai requirement: %v", err)
	}
}

func TestNewGeminiInvoker_RequiresGCPProject(t *testing.T) {
	_, err := NewGeminiInvoker(core.ProviderEntry{
		Backend: "gemini",
		Auth:    vertexAIAuthValue,
		Model:   "gemini-2.5-flash",
	}, 0)
	if err == nil {
		t.Fatal("expected error for missing gcp_project")
	}
	if !strings.Contains(err.Error(), "gcp_project") {
		t.Errorf("error should mention gcp_project: %v", err)
	}
}

func TestNewGeminiInvoker_RejectsUnknownSafetyThreshold(t *testing.T) {
	_, err := NewGeminiInvoker(core.ProviderEntry{
		Backend: "gemini",
		Auth:    vertexAIAuthValue,
		Model:   "gemini-2.5-flash",
		Options: map[string]string{
			"gcp_project":      "p",
			"safety_threshold": "BLOCK_EVERYTHING",
		},
	}, 0)
	if err == nil {
		t.Fatal("expected error for unknown safety threshold")
	}
}

func TestNewGeminiInvoker_DefaultsRegion(t *testing.T) {
	if !gcloudOnPath(t) {
		t.Skip("gcloud not on PATH; cannot exercise this code path")
	}
	inv, err := NewGeminiInvoker(core.ProviderEntry{
		Backend: "gemini",
		Auth:    vertexAIAuthValue,
		Model:   "gemini-2.5-flash",
		Options: map[string]string{"gcp_project": "p"},
	}, 0)
	if err != nil {
		t.Fatalf("NewGeminiInvoker: %v", err)
	}
	if inv.region != defaultGeminiRegion {
		t.Errorf("region: got %q, want %q", inv.region, defaultGeminiRegion)
	}
	if !strings.Contains(inv.baseURL, defaultGeminiRegion) {
		t.Errorf("baseURL should embed default region: %q", inv.baseURL)
	}
}

func TestNewInvoker_DispatchesToGemini(t *testing.T) {
	if !gcloudOnPath(t) {
		t.Skip("gcloud not on PATH; factory dispatch test requires it")
	}
	inv, err := NewInvoker(core.ProviderEntry{
		Backend: "gemini",
		Auth:    vertexAIAuthValue,
		Model:   "gemini-2.5-flash",
		Options: map[string]string{"gcp_project": "p"},
	}, 0)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	if _, ok := inv.(*GeminiInvoker); !ok {
		t.Errorf("expected *GeminiInvoker, got %T", inv)
	}
}

func TestIsTransientError_SafetyBlock(t *testing.T) {
	err := &errStr{s: "gemini response blocked: SAFETY"}
	if IsTransientError(err) {
		t.Error("safety block should be classified non-retryable")
	}
}

// errStr is a tiny error type used to construct test error values without
// importing errors and avoiding fmt.Errorf allocations in benchmarks.
type errStr struct{ s string }

func (e *errStr) Error() string { return e.s }
