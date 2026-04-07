// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import "testing"

func TestExtractCorpusFromChunkID(t *testing.T) {
	tests := []struct {
		chunkID string
		want    string
	}{
		{"docs-auth-000", "docs"},
		{"api-docs-routes-001", "api-docs"},
		{"my-corpus-file-002", "my-corpus"},
		{"a-b", ""},   // too few segments
		{"single", ""}, // no dashes
	}

	for _, tt := range tests {
		got := extractCorpusFromChunkID(tt.chunkID, nil)
		if got != tt.want {
			t.Errorf("extractCorpusFromChunkID(%q, nil) = %q, want %q", tt.chunkID, got, tt.want)
		}
	}
}
