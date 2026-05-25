// SPDX-License-Identifier: MIT
package format

import "testing"

func TestStatusIcon_AllStatuses(t *testing.T) {
	tests := []struct {
		status    string
		want      string
		wantASCII string
	}{
		{"active", "○", "o"},
		{"in_progress", "◐", "p"},
		{"completed", "✓", "x"},
		{"archived", "●", "."},
		{"deprecated", "❄", "-"},
		{"unknown", "?", "?"},
		{"", "?", "?"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := StatusIcon(tt.status, false); got != tt.want {
				t.Errorf("Unicode: want %q, got %q", tt.want, got)
			}
			if got := StatusIcon(tt.status, true); got != tt.wantASCII {
				t.Errorf("ASCII: want %q, got %q", tt.wantASCII, got)
			}
		})
	}
}

func TestIconASCIIFromEnv(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"1", true},
		{"true", true},
		{"yes", true},
	}
	for _, tt := range tests {
		t.Run("NO_UNICODE="+tt.val, func(t *testing.T) {
			t.Setenv("NO_UNICODE", tt.val)
			if got := IconASCIIFromEnv(); got != tt.want {
				t.Errorf("NO_UNICODE=%q: want %v, got %v", tt.val, tt.want, got)
			}
		})
	}
}
