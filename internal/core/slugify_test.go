// SPDX-License-Identifier: MIT
package core

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"basic", "auth uses JWT", "auth-uses-jwt"},
		{"whitespace_collapse", "  multiple   spaces  ", "multiple-spaces"},
		{"punctuation_stripped", "don't use!sessions.", "don-t-use-sessions"},
		{"leading_punct", "...auth", "auth"},
		{"trailing_punct", "auth...", "auth"},
		{"only_punctuation", "!!!", ""},
		{"empty", "", ""},
		{"non_ascii_dropped", "café au lait", "caf-au-lait"},
		{"emoji_dropped", "ship it 🚀", "ship-it"},
		{"numbers_kept", "go 1.22 release", "go-1-22-release"},
		{"already_slug", "auth-jwt", "auth-jwt"},
		{"underscores_become_hyphens", "auth_jwt_token", "auth-jwt-token"},
		{"runs_of_hyphens_collapse", "auth---jwt", "auth-jwt"},
		{"basic_full_sentence", "always run tests with -race flag",
			"always-run-tests-with-race-flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.in)
			if got != tt.want {
				t.Errorf("Slugify(%q): want %q, got %q", tt.in, tt.want, got)
			}
		})
	}
}

func TestSlugify_TruncatesAtMaxLen(t *testing.T) {
	in := "a very long body that should truncate after the SlugMaxLen boundary in total length"
	got := Slugify(in)
	if len(got) > SlugMaxLen {
		t.Errorf("slug length %d exceeds SlugMaxLen=%d: %q", len(got), SlugMaxLen, got)
	}
	if strings.HasSuffix(got, "-") {
		t.Errorf("truncated slug should not end with hyphen: %q", got)
	}
}

func TestSlugify_TruncationDoesNotLeaveTrailingHyphen(t *testing.T) {
	// Engineer a string where the SlugMaxLen-th byte falls on a hyphen.
	// Build "a-b-c-d-..." until length > SlugMaxLen.
	var parts []string
	for i := 0; len(strings.Join(parts, "-")) < SlugMaxLen+5; i++ {
		parts = append(parts, "x")
	}
	in := strings.Join(parts, " ")
	got := Slugify(in)
	if strings.HasSuffix(got, "-") {
		t.Errorf("slug should never end with a hyphen, got %q", got)
	}
}
