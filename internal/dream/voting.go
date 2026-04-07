// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"fmt"
	"sort"
	"strings"
)

// VoteVisions takes N independent vision lists from the same batch
// and returns visions that appear in a majority (>N/2) of runs.
// threshold is the fraction required (e.g. 0.5 for majority).
func VoteVisions(candidates [][]Vision, threshold float64) []Vision {
	if len(candidates) <= 1 {
		if len(candidates) == 1 {
			for i := range candidates[0] {
				candidates[0][i].Agreement = 1.0
			}
			return candidates[0]
		}
		return nil
	}

	n := len(candidates)
	minVotes := int(float64(n)*threshold) + 1
	if minVotes < 1 {
		minVotes = 1
	}

	// Group visions by canonical key across all runs
	groups := make(map[string][]Vision)
	for _, visions := range candidates {
		for _, v := range visions {
			key := visionKey(v)
			groups[key] = append(groups[key], v)
		}
	}

	// Keep visions that meet the vote threshold
	var result []Vision
	for _, group := range groups {
		if len(group) >= minVotes {
			merged := mergeAgreedVisions(group, n)
			result = append(result, merged)
		}
	}

	// Sort for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return visionKey(result[i]) < visionKey(result[j])
	})

	return result
}

// visionKey generates a canonical key for matching visions across runs.
// Key = (type, action, sorted(source_motes), sorted(target_motes) for link types, link_type for links)
func visionKey(v Vision) string {
	sources := make([]string, len(v.SourceMotes))
	copy(sources, v.SourceMotes)
	sort.Strings(sources)

	key := fmt.Sprintf("%s|%s|%s", v.Type, v.Action, strings.Join(sources, ","))

	if v.Type == "link_suggestion" {
		targets := make([]string, len(v.TargetMotes))
		copy(targets, v.TargetMotes)
		sort.Strings(targets)
		key += fmt.Sprintf("|%s|%s", strings.Join(targets, ","), v.LinkType)
	}

	return key
}

// mergeAgreedVisions combines matching visions from multiple runs.
func mergeAgreedVisions(group []Vision, totalRuns int) Vision {
	merged := group[0]
	merged.Agreement = float64(len(group)) / float64(totalRuns)

	// Use highest severity
	severityRank := map[string]int{"low": 1, "medium": 2, "high": 3}
	bestSev := severityRank[merged.Severity]

	// Track longest rationale
	longestRationale := merged.Rationale

	// Union source and target motes across all runs
	sourceSet := make(map[string]bool)
	targetSet := make(map[string]bool)
	for _, id := range merged.SourceMotes {
		sourceSet[id] = true
	}
	for _, id := range merged.TargetMotes {
		targetSet[id] = true
	}

	for _, v := range group[1:] {
		if severityRank[v.Severity] > bestSev {
			bestSev = severityRank[v.Severity]
			merged.Severity = v.Severity
		}
		if len(v.Rationale) > len(longestRationale) {
			longestRationale = v.Rationale
		}
		for _, id := range v.SourceMotes {
			sourceSet[id] = true
		}
		for _, id := range v.TargetMotes {
			targetSet[id] = true
		}
	}

	// Annotate rationale with agreement count
	merged.Rationale = fmt.Sprintf("[%d/%d runs agreed] %s", len(group), totalRuns, longestRationale)

	// Rebuild sorted mote lists
	merged.SourceMotes = sortedKeys(sourceSet)
	merged.TargetMotes = sortedKeys(targetSet)

	return merged
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
