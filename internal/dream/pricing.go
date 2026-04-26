// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import "strings"

// EstimateTokens estimates the token count of text (~4 chars/token).
func EstimateTokens(text string) int {
	return len(text) / 4
}

// modelRates is the per-million-token pricing for a model family.
type modelRates struct {
	inputPPM  float64
	outputPPM float64
}

// pricingTable maps a model-name substring to its rates. EstimateCost
// lower-cases the model name and returns the first matching family.
//
// The first matching substring wins, so list more-specific identifiers
// (e.g. "gpt-4o-mini") before broader ones (e.g. "gpt-4o") when both are
// present. The current entries are non-overlapping aside from the
// gemini-2.5-{flash,pro} pair, which discriminate on the trailing word.
//
// Pricing constants are sourced from each vendor's public pricing page on
// 2026-04-25. Update both the value and the date when refreshing.
var pricingTable = []struct {
	substring string
	rates     modelRates
}{
	// Anthropic — anthropic.com/pricing (2026-04-25)
	{"claude-opus", modelRates{15.0, 75.0}},
	{"claude-sonnet", modelRates{3.0, 15.0}},
	// OpenAI — openai.com/api/pricing (2026-04-25)
	{"gpt-4o-mini", modelRates{0.15, 0.60}},
	{"gpt-4o", modelRates{2.50, 10.0}},
	{"o1-mini", modelRates{3.0, 12.0}},
	{"o1", modelRates{15.0, 60.0}},
	// Google — cloud.google.com/vertex-ai/pricing (2026-04-25)
	{"gemini-2.5-flash", modelRates{0.30, 2.50}},
	{"gemini-2.5-pro", modelRates{1.25, 10.0}},
}

// EstimateCost returns an estimated cost in USD for a model invocation.
// Returns 0 for unknown models — callers should treat zero as "unknown",
// not "free".
func EstimateCost(model string, inputTokens, outputTokens int) float64 {
	lower := strings.ToLower(model)
	for _, entry := range pricingTable {
		if strings.Contains(lower, entry.substring) {
			return (float64(inputTokens)*entry.rates.inputPPM +
				float64(outputTokens)*entry.rates.outputPPM) / 1_000_000
		}
	}
	return 0
}
