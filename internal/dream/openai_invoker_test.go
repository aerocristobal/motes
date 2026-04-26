// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"motes/internal/core"
)

// newTestOpenAIInvoker returns an invoker whose baseURL points at the supplied
// httptest server. Test API key is hardcoded so we never need an env var.
func newTestOpenAIInvoker(t *testing.T, server *httptest.Server) *OpenAIInvoker {
	t.Helper()
	inv, err := NewOpenAIInvoker(core.ProviderEntry{
		Auth:  "sk-test-literal",
		Model: "gpt-4o",
	}, 0)
	if err != nil {
		t.Fatalf("NewOpenAIInvoker: %v", err)
	}
	inv.baseURL = server.URL
	// Tighten the per-test retry/timeout so failure modes don't slow the suite.
	inv.timeout = 2 * time.Second
	inv.httpClient.Timeout = 2 * time.Second
	inv.retryPolicy = &RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    2 * time.Millisecond,
		Retryable:   IsTransientError,
	}
	return inv
}

func openAIOKResponse(content string) []byte {
	body, _ := json.Marshal(openAIChatResponse{
		Model: "gpt-4o-2025-09-15",
		Choices: []openAIChatChoice{{
			Message: openAIChatMessage{Role: "assistant", Content: content},
		}},
		Usage: openAIChatUsage{PromptTokens: 17, CompletionTokens: 42},
	})
	return body
}

func TestOpenAIInvoker_SuccessfulRequest(t *testing.T) {
	var capturedReq openAIChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test-literal" {
			t.Errorf("Authorization: got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type: got %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedReq)
		w.WriteHeader(http.StatusOK)
		w.Write(openAIOKResponse(`{"visions":[]}`))
	}))
	defer srv.Close()

	inv := newTestOpenAIInvoker(t, srv)
	res, err := inv.Invoke("hello world", "sonnet")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Response != `{"visions":[]}` {
		t.Errorf("Response: got %q", res.Response)
	}
	if res.InputTokens != 17 {
		t.Errorf("InputTokens: got %d, want 17", res.InputTokens)
	}
	if res.OutputTokens != 42 {
		t.Errorf("OutputTokens: got %d, want 42", res.OutputTokens)
	}
	if res.Model != "gpt-4o-2025-09-15" {
		t.Errorf("Model: got %q", res.Model)
	}
	if capturedReq.Model != "gpt-4o" {
		t.Errorf("captured request model: got %q", capturedReq.Model)
	}
	if len(capturedReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Errorf("system message role: got %q", capturedReq.Messages[0].Role)
	}
	if !strings.Contains(capturedReq.Messages[0].Content, "JSON-only") {
		t.Errorf("system message should enforce JSON-only output: %q", capturedReq.Messages[0].Content)
	}
	if capturedReq.Messages[1].Content != "hello world" {
		t.Errorf("user message: got %q", capturedReq.Messages[1].Content)
	}
}

func TestOpenAIInvoker_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limited","code":"429"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(openAIOKResponse(`{"ok":true}`))
	}))
	defer srv.Close()

	inv := newTestOpenAIInvoker(t, srv)
	res, err := inv.Invoke("p", "sonnet")
	if err != nil {
		t.Fatalf("Invoke after retries: %v", err)
	}
	if res.Response != `{"ok":true}` {
		t.Errorf("response after retry: got %q", res.Response)
	}
	if calls != 3 {
		t.Errorf("expected 3 attempts, got %d", calls)
	}
}

func TestOpenAIInvoker_RetriesOn503ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"message":"upstream busy"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(openAIOKResponse(`{"ok":true}`))
	}))
	defer srv.Close()

	inv := newTestOpenAIInvoker(t, srv)
	if _, err := inv.Invoke("p", "sonnet"); err != nil {
		t.Fatalf("Invoke after 503 retry: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 attempts, got %d", calls)
	}
}

func TestOpenAIInvoker_DoesNotRetry400(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad input"}}`))
	}))
	defer srv.Close()

	inv := newTestOpenAIInvoker(t, srv)
	_, err := inv.Invoke("p", "sonnet")
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should reference HTTP status: %v", err)
	}
	if calls != 1 {
		t.Errorf("400 should not be retried; got %d attempts", calls)
	}
}

func TestOpenAIInvoker_NoChoicesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	inv := newTestOpenAIInvoker(t, srv)
	_, err := inv.Invoke("p", "sonnet")
	if err == nil {
		t.Fatal("expected error when API returns zero choices")
	}
}

func TestNewOpenAIInvoker_RejectsEmptyAuth(t *testing.T) {
	_, err := NewOpenAIInvoker(core.ProviderEntry{Backend: "openai", Model: "gpt-4o"}, 0)
	if err == nil {
		t.Fatal("expected error for empty auth")
	}
}

func TestNewOpenAIInvoker_RejectsEmptyModel(t *testing.T) {
	_, err := NewOpenAIInvoker(core.ProviderEntry{Backend: "openai", Auth: "sk-x"}, 0)
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestNewInvoker_DispatchesToOpenAI(t *testing.T) {
	inv, err := NewInvoker(core.ProviderEntry{
		Backend: "openai",
		Auth:    "sk-test",
		Model:   "gpt-4o",
	}, 0)
	if err != nil {
		t.Fatalf("NewInvoker: %v", err)
	}
	if _, ok := inv.(*OpenAIInvoker); !ok {
		t.Errorf("expected *OpenAIInvoker, got %T", inv)
	}
}
