// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"math"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abcd", 1},
		{"abcdefgh", 2},
		{"abc", 0}, // truncates
	}
	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got != tt.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestEstimateCost(t *testing.T) {
	// Helper: per-million-token rate × token count, scaled.
	calc := func(in, out int, inPPM, outPPM float64) float64 {
		return (float64(in)*inPPM + float64(out)*outPPM) / 1_000_000
	}
	tests := []struct {
		name   string
		model  string
		input  int
		output int
		want   float64
	}{
		{"sonnet by exact id", "claude-sonnet-4-20250514", 1000, 500, calc(1000, 500, 3.0, 15.0)},
		{"opus by exact id", "claude-opus-4-20250514", 1000, 500, calc(1000, 500, 15.0, 75.0)},
		{"sonnet case-insensitive", "Claude-Sonnet-Latest", 100, 100, calc(100, 100, 3.0, 15.0)},
		{"unknown model returns zero", "unknown-model", 1000, 500, 0},
		{"gpt-4o", "gpt-4o", 1_000_000, 1_000_000, calc(1_000_000, 1_000_000, 2.5, 10.0)},
		{"gpt-4o-mini matches mini before parent", "gpt-4o-mini", 1_000_000, 1_000_000, calc(1_000_000, 1_000_000, 0.15, 0.60)},
		{"o1", "o1-2024-12-17", 1_000_000, 1_000_000, calc(1_000_000, 1_000_000, 15.0, 60.0)},
		{"o1-mini matches mini before parent", "o1-mini-2024-09-12", 1_000_000, 1_000_000, calc(1_000_000, 1_000_000, 3.0, 12.0)},
		{"gemini flash", "gemini-2.5-flash", 1_000_000, 1_000_000, calc(1_000_000, 1_000_000, 0.30, 2.50)},
		{"gemini pro", "gemini-2.5-pro", 1_000_000, 1_000_000, calc(1_000_000, 1_000_000, 1.25, 10.0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateCost(tt.model, tt.input, tt.output)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("EstimateCost(%q, %d, %d) = %f, want %f", tt.model, tt.input, tt.output, got, tt.want)
			}
		})
	}
}

// TestEstimateCost_TierOrdering ensures higher-capability models are at least
// as expensive as lower-capability ones in the same family. A regression here
// usually means the pricing table has a typo (e.g. opus and sonnet swapped).
func TestEstimateCost_TierOrdering(t *testing.T) {
	const tokens = 1_000_000
	pairs := []struct {
		bigger, smaller string
	}{
		{"claude-opus", "claude-sonnet"},
		{"o1", "gpt-4o"},
		{"gemini-2.5-pro", "gemini-2.5-flash"},
	}
	for _, p := range pairs {
		big := EstimateCost(p.bigger, tokens, tokens)
		small := EstimateCost(p.smaller, tokens, tokens)
		if !(big > small) {
			t.Errorf("expected %s ($%.4f) to cost more than %s ($%.4f)",
				p.bigger, big, p.smaller, small)
		}
	}
}
