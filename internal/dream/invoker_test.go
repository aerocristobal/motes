// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestNewInvoker_ClaudeCLIBackend(t *testing.T) {
	tests := []struct {
		name    string
		entry   core.ProviderEntry
		wantErr bool
	}{
		{
			name:  "explicit claude-cli backend",
			entry: core.ProviderEntry{Backend: "claude-cli", Model: "claude-sonnet-4-6"},
		},
		{
			name:  "empty backend defaults to claude-cli",
			entry: core.ProviderEntry{Backend: "", Model: "claude-sonnet-4-6"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := NewInvoker(tt.entry, 0)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewInvoker err = %v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if _, ok := inv.(*ClaudeInvoker); !ok {
					t.Errorf("expected *ClaudeInvoker, got %T", inv)
				}
				if inv.Model() != tt.entry.Model {
					t.Errorf("Model() = %q, want %q", inv.Model(), tt.entry.Model)
				}
			}
		})
	}
}

func TestNewInvoker_UnknownBackend(t *testing.T) {
	_, err := NewInvoker(core.ProviderEntry{Backend: "anthropic-direct"}, 0)
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "anthropic-direct") {
		t.Errorf("error should mention the offending backend value: %v", err)
	}
	if !strings.Contains(err.Error(), "claude-cli") {
		t.Errorf("error should list valid options: %v", err)
	}
}

func TestNewInvoker_OpenAIRequiresAuthAndModel(t *testing.T) {
	// Real OpenAI dispatch lives in TestNewInvoker_DispatchesToOpenAI;
	// this test pins the failure mode when the user forgets to configure auth.
	_, err := NewInvoker(core.ProviderEntry{Backend: "openai"}, 0)
	if err == nil {
		t.Fatal("openai backend should error when auth/model are missing")
	}
}

func TestNewInvoker_GeminiRequiresVertexAuth(t *testing.T) {
	// Real Gemini dispatch is exercised in TestNewInvoker_DispatchesToGemini
	// (gemini_invoker_test.go); this test pins the failure mode for missing
	// auth.
	_, err := NewInvoker(core.ProviderEntry{Backend: "gemini"}, 0)
	if err == nil {
		t.Fatal("gemini backend should error when auth is missing")
	}
}

func TestClaudeInvoker_SatisfiesInterface(t *testing.T) {
	// Compile-time assertion exists in invoker.go; this test makes the contract
	// visible to readers and ensures Model() returns the configured value.
	ci := NewClaudeInvoker(core.ProviderEntry{Model: "claude-opus-4-6"}, 0)
	var inv Invoker = ci
	if inv.Model() != "claude-opus-4-6" {
		t.Errorf("Model() = %q, want claude-opus-4-6", inv.Model())
	}
}

func TestClaudeInvoker_DefaultsModel(t *testing.T) {
	ci := NewClaudeInvoker(core.ProviderEntry{}, 0)
	if ci.Model() == "" {
		t.Error("empty entry.Model should produce a non-empty default")
	}
}

func TestFilterEnv_RemovesNamedVar(t *testing.T) {
	os.Setenv("MOTES_TEST_VAR", "hello")
	defer os.Unsetenv("MOTES_TEST_VAR")

	result := filterEnv("MOTES_TEST_VAR")
	for _, e := range result {
		if strings.HasPrefix(e, "MOTES_TEST_VAR=") {
			t.Error("MOTES_TEST_VAR should be removed")
		}
	}
}

func TestFilterEnv_RemovesMultiple(t *testing.T) {
	os.Setenv("MOTES_A", "1")
	os.Setenv("MOTES_B", "2")
	defer os.Unsetenv("MOTES_A")
	defer os.Unsetenv("MOTES_B")

	result := filterEnv("MOTES_A", "MOTES_B")
	for _, e := range result {
		k, _, _ := strings.Cut(e, "=")
		if k == "MOTES_A" || k == "MOTES_B" {
			t.Errorf("%s should be removed", k)
		}
	}
}

func TestFilterEnv_NoMatchReturnsAll(t *testing.T) {
	full := os.Environ()
	result := filterEnv("NONEXISTENT_VAR_XYZ_12345")
	if len(result) != len(full) {
		t.Errorf("expected %d env vars, got %d", len(full), len(result))
	}
}

func TestFilterEnv_EmptyNames(t *testing.T) {
	full := os.Environ()
	result := filterEnv()
	if len(result) != len(full) {
		t.Errorf("expected %d env vars, got %d", len(full), len(result))
	}
}
