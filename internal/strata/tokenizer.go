package strata

import (
	"regexp"
	"strings"
)

var bmStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "with": true, "by": true, "from": true, "as": true,
	"and": true, "but": true, "or": true, "not": true, "if": true,
	"this": true, "that": true, "these": true, "those": true,
	"it": true, "its": true, "he": true, "she": true, "they": true,
}

var tokenRegex = regexp.MustCompile(`[a-zA-Z0-9_]+`)

// Tokenize splits text into lowercase tokens, removing stop words and short words.
func Tokenize(text string) []string {
	return tokenize(text)
}

func tokenize(text string) []string {
	words := tokenRegex.FindAllString(strings.ToLower(text), -1)
	result := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) < 2 || bmStopWords[w] {
			continue
		}
		result = append(result, w)
	}
	return result
}

// estimateTokens approximates token count as wordCount * 1.3.
func estimateTokens(text string) int {
	words := len(strings.Fields(text))
	return int(float64(words) * 1.3)
}
