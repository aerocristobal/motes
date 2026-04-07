// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ResponseParser extracts structured data from Claude's text responses.
type ResponseParser struct{}

// NewResponseParser creates a response parser.
func NewResponseParser() *ResponseParser {
	return &ResponseParser{}
}

// ParseBatchResponse extracts visions and lucid log updates from a batch response.
func (rp *ResponseParser) ParseBatchResponse(raw string) ([]Vision, LucidLogUpdates, error) {
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, LucidLogUpdates{}, fmt.Errorf("no JSON found in response")
	}
	var resp struct {
		Visions         []Vision        `json:"visions"`
		LucidLogUpdates LucidLogUpdates `json:"lucid_log_updates"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, LucidLogUpdates{}, fmt.Errorf("invalid JSON: %w", err)
	}
	valid := filterValidVisions(resp.Visions)
	return valid, resp.LucidLogUpdates, nil
}

// ParseReconciliationResponse extracts the final vision list from reconciliation.
func (rp *ResponseParser) ParseReconciliationResponse(raw string) ([]Vision, error) {
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}
	var resp struct {
		Visions []Vision `json:"visions"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return filterValidVisions(resp.Visions), nil
}

var fenceRe = regexp.MustCompile("(?s)```(?:json[c]?|JSON)?\\s*\\n?(.*?)```")

// extractJSON finds the first balanced top-level JSON object or array in the text.
// Handles markdown code fences (```json ... ```, ```JSON ... ```, ```jsonc ... ```)
// that Claude may wrap responses in.
func extractJSON(raw string) string {
	// Strip markdown code fences if present
	cleaned := raw
	if m := fenceRe.FindStringSubmatch(cleaned); m != nil {
		cleaned = m[1]
	}

	start := strings.IndexAny(cleaned, "{[")
	if start == -1 {
		return ""
	}
	openChar := cleaned[start]
	closeChar := byte('}')
	if openChar == '[' {
		closeChar = ']'
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(cleaned); i++ {
		ch := cleaned[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == openChar {
			depth++
		} else if ch == closeChar {
			depth--
			if depth == 0 {
				return cleaned[start : i+1]
			}
		}
	}
	return ""
}

// filterValidVisions keeps only visions with required fields populated.
func filterValidVisions(visions []Vision) []Vision {
	var valid []Vision
	for _, v := range visions {
		if v.Type == "" || v.Rationale == "" || len(v.SourceMotes) == 0 {
			continue
		}
		if v.Severity == "" {
			v.Severity = "medium"
		}
		valid = append(valid, v)
	}
	return valid
}
