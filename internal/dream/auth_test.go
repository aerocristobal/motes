// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"strings"
	"testing"
)

func TestResolveAuth_EnvVarResolution(t *testing.T) {
	const varName = "MOTES_TEST_OPENAI_KEY"
	t.Setenv(varName, "sk-from-env")
	got, err := resolveAuth(varName)
	if err != nil {
		t.Fatalf("resolveAuth: %v", err)
	}
	if got != "sk-from-env" {
		t.Errorf("got %q, want sk-from-env", got)
	}
}

func TestResolveAuth_LiteralValue(t *testing.T) {
	// A literal credential that doesn't match the env-var-name heuristic
	// (contains lowercase + dashes) is returned verbatim.
	got, err := resolveAuth("sk-abc123")
	if err != nil {
		t.Fatalf("resolveAuth: %v", err)
	}
	if got != "sk-abc123" {
		t.Errorf("got %q, want sk-abc123", got)
	}
}

func TestResolveAuth_EnvVarNameButUnset(t *testing.T) {
	// Make sure the var is unset
	const varName = "MOTES_NEVER_SET_ENV_VAR_XYZ"
	os.Unsetenv(varName)
	_, err := resolveAuth(varName)
	if err == nil {
		t.Fatal("expected error for unset env var name")
	}
	if !strings.Contains(err.Error(), varName) {
		t.Errorf("error should mention the missing var name: %v", err)
	}
}

func TestResolveAuth_Empty(t *testing.T) {
	_, err := resolveAuth("")
	if err == nil {
		t.Fatal("expected error for empty auth")
	}
}

func TestLooksLikeEnvVarName(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"OPENAI_API_KEY", true},
		{"GEMINI_API_KEY", true},
		{"X_Y_Z", true},
		{"NOTHING", false}, // no underscore — most secrets aren't single-word
		{"sk-abc123", false},
		{"sk_abc_123", false}, // lowercase
		{"", false},
		{"oauth", false},
	}
	for _, tt := range tests {
		if got := looksLikeEnvVarName(tt.in); got != tt.want {
			t.Errorf("looksLikeEnvVarName(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
