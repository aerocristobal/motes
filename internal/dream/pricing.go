package dream

import "strings"

// EstimateTokens estimates the token count of text (~4 chars/token).
func EstimateTokens(text string) int {
	return len(text) / 4
}

// Per-million-token pricing (input, output) as of 2025.
const (
	sonnetInputPPM  = 3.0
	sonnetOutputPPM = 15.0
	opusInputPPM    = 15.0
	opusOutputPPM   = 75.0
)

// EstimateCost returns an estimated cost in USD for a model invocation.
func EstimateCost(model string, inputTokens, outputTokens int) float64 {
	lower := strings.ToLower(model)
	var inRate, outRate float64
	switch {
	case strings.Contains(lower, "opus"):
		inRate, outRate = opusInputPPM, opusOutputPPM
	case strings.Contains(lower, "sonnet"):
		inRate, outRate = sonnetInputPPM, sonnetOutputPPM
	default:
		return 0
	}
	return (float64(inputTokens)*inRate + float64(outputTokens)*outRate) / 1_000_000
}
