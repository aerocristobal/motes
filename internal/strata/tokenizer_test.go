// SPDX-License-Identifier: AGPL-3.0-or-later
package strata

import (
	"testing"
)

func TestTokenize_Basic(t *testing.T) {
	tokens := Tokenize("The OAuth API is great")
	expected := []string{"oauth", "api", "great"}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
	for i, tok := range tokens {
		if tok != expected[i] {
			t.Errorf("token %d: got %q, want %q", i, tok, expected[i])
		}
	}
}

func TestTokenize_StopWords(t *testing.T) {
	tokens := Tokenize("the a an is are was were be been being")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens (all stop words), got %d: %v", len(tokens), tokens)
	}
}

func TestTokenize_Empty(t *testing.T) {
	tokens := Tokenize("")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestTokenize_Underscores(t *testing.T) {
	tokens := Tokenize("access_token refresh_token")
	// tokenRegex matches [a-zA-Z0-9_]+, so access_token is one token
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d: %v", len(tokens), tokens)
	}
}
