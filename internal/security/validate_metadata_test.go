// SPDX-License-Identifier: MIT
package security

import (
	"strings"
	"testing"
)

func TestValidateMetadataKey_Accepts(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"alnum_underscore", "execution_mode"},
		{"longer_compound", "execution_parallel_group"},
		{"due_at_like", "due_at"},
		{"single_char", "a"},
		{"underscores_and_digits", "with_underscores_123"},
		{"uppercase_ok", "UPPER_CASE_OK"},
		{"mixed_case", "Execution_Mode"},
		{"leading_underscore", "_internal"},
		{"only_digits", "12345"},
		{"at_max_len", strings.Repeat("a", MetadataKeyMaxLen)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateMetadataKey(tt.key); err != nil {
				t.Errorf("ValidateMetadataKey(%q) returned unexpected error: %v", tt.key, err)
			}
		})
	}
}

func TestValidateMetadataKey_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		key        string
		wantSubstr string
	}{
		{"empty", "", "empty"},
		{"dot", "execution.mode", "invalid metadata key"},
		{"slash", "execution/mode", "invalid metadata key"},
		{"backslash", "execution\\mode", "invalid metadata key"},
		{"traversal_dotdot", "..", "invalid metadata key"},
		{"path_traversal", "../etc/passwd", "invalid metadata key"},
		{"dollar_paren", "$(rm -rf ~)", "invalid metadata key"},
		{"space", "foo bar", "invalid metadata key"},
		{"semicolon", "foo;bar", "invalid metadata key"},
		{"backtick", "foo`bar`", "invalid metadata key"},
		{"newline", "foo\nbar", "invalid metadata key"},
		{"carriage_return", "foo\rbar", "invalid metadata key"},
		{"tab", "foo\tbar", "invalid metadata key"},
		{"nul_byte", "foo\x00bar", "invalid metadata key"},
		{"hyphen", "foo-bar", "invalid metadata key"},
		{"bidi_lro", "execution_mode‭", "bidi"},
		{"bidi_rle", "execution_mode‫", "bidi"},
		{"bidi_pdf", "execution_mode‬", "bidi"},
		{"bidi_lri", "execution_mode⁦", "bidi"},
		{"bidi_pdi", "execution_mode⁩", "bidi"},
		{"too_long", strings.Repeat("a", MetadataKeyMaxLen+1), "too long"},
		{"invalid_utf8", "\xff\xfe\xfd", "invalid"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMetadataKey(tt.key)
			if err == nil {
				t.Fatalf("ValidateMetadataKey(%q) expected error, got nil", tt.key)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("ValidateMetadataKey(%q) error = %v; want substring %q", tt.key, err, tt.wantSubstr)
			}
		})
	}
}

func TestValidateMetadataValue_Accepts(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"normal_string", "parallel"},
		{"with_spaces", "hello world"},
		{"with_punctuation", "group-A.1_v2"},
		{"with_equals", "foo=bar"},
		{"with_semicolon", "a;b"},
		{"with_backtick", "a`b"},
		{"unicode", "résumé"},
		{"emoji", "fire 🔥"},
		{"at_max_len", strings.Repeat("v", MetadataValueMaxLen)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateMetadataValue(tt.value); err != nil {
				t.Errorf("ValidateMetadataValue(%q) returned unexpected error: %v", tt.value, err)
			}
		})
	}
}

func TestValidateMetadataValue_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		value      string
		wantSubstr string
	}{
		{"too_long", strings.Repeat("v", MetadataValueMaxLen+1), "too long"},
		{"invalid_utf8", "\xff\xfe\xfd", "invalid metadata value"},
		{"nul_byte", "foo\x00bar", "NUL byte"},
		{"bidi_rlo", "foo‮bar", "bidi"},
		{"bidi_lro", "foo‭bar", "bidi"},
		{"bidi_lri", "foo⁦bar", "bidi"},
		{"bidi_pdi", "foo⁩bar", "bidi"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMetadataValue(tt.value)
			if err == nil {
				t.Fatalf("ValidateMetadataValue(%q) expected error, got nil", tt.value)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("ValidateMetadataValue(%q) error = %v; want substring %q", tt.value, err, tt.wantSubstr)
			}
		})
	}
}
