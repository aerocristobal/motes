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
	tests := []struct {
		model  string
		input  int
		output int
		want   float64
	}{
		{"claude-sonnet-4-20250514", 1000, 500, (1000*3.0 + 500*15.0) / 1_000_000},
		{"claude-opus-4-20250514", 1000, 500, (1000*15.0 + 500*75.0) / 1_000_000},
		{"unknown-model", 1000, 500, 0},
		{"Claude-Sonnet-Latest", 100, 100, (100*3.0 + 100*15.0) / 1_000_000},
	}
	for _, tt := range tests {
		got := EstimateCost(tt.model, tt.input, tt.output)
		if math.Abs(got-tt.want) > 1e-12 {
			t.Errorf("EstimateCost(%q, %d, %d) = %f, want %f", tt.model, tt.input, tt.output, got, tt.want)
		}
	}
}
