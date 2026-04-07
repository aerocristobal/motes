// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// SessionState holds persistent state for the current session.
type SessionState struct {
	CurrentTask string   `json:"current_task,omitempty"`
	Topics      []string `json:"topics,omitempty"`
	StartTime   string   `json:"start_time"`
}

// ReadSessionState reads .memory/.session if it exists.
func ReadSessionState(root string) *SessionState {
	data, err := os.ReadFile(filepath.Join(root, ".session"))
	if err != nil {
		return nil
	}
	var s SessionState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return &s
}

// WriteSessionState writes the session state file.
func WriteSessionState(root string, s *SessionState) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(root, ".session"), data, 0644)
}

// ClearSessionState removes the session file.
func ClearSessionState(root string) {
	os.Remove(filepath.Join(root, ".session"))
}

// TextSearchResult holds a single text search hit.
type TextSearchResult struct {
	ID    string
	Score float64
}

// TextSearcher is a minimal interface for text-based search (e.g., BM25).
type TextSearcher interface {
	Search(query string, topK int) []TextSearchResult
}

// SeedSelector finds initial seed motes from topic keywords and ambient signals.
type SeedSelector struct {
	motes        []*Mote
	tagStats     map[string]int
	signals      []SignalConfig
	searcher     TextSearcher
	conceptIndex map[string][]string // concept term → mote IDs
}

// AmbientContext holds signals collected from the environment.
type AmbientContext struct {
	GitBranch   string
	RecentFiles []string
	PromptText  string
	ContextHint string
}

// NewSeedSelector creates a SeedSelector. searcher may be nil to disable BM25.
func NewSeedSelector(motes []*Mote, tagStats map[string]int, signals []SignalConfig, searcher TextSearcher) *SeedSelector {
	return &SeedSelector{motes: motes, tagStats: tagStats, signals: signals, searcher: searcher}
}

// SetConceptIndex sets the concept term → mote IDs mapping for concept-aware seed selection.
func (ss *SeedSelector) SetConceptIndex(ci map[string][]string) {
	ss.conceptIndex = ci
}

// BuildConceptIndex extracts a mapping of concept term → source mote IDs from concept_ref edges.
func BuildConceptIndex(idx *EdgeIndex) map[string][]string {
	ci := map[string][]string{}
	for _, e := range idx.Edges {
		if e.EdgeType == "concept_ref" {
			ci[e.Target] = append(ci[e.Target], e.Source)
		}
	}
	return ci
}

// SelectSeeds returns seed motes matching the topic and ambient signals.
func (ss *SeedSelector) SelectSeeds(topic string, ambient *AmbientContext) []*Mote {
	candidates := make(map[string]float64) // moteID -> signal strength

	keywords := extractKeywords(topic)

	// Concept matching: boost motes that reference matching concept terms
	if ss.conceptIndex != nil {
		for _, kw := range keywords {
			if moteIDs, ok := ss.conceptIndex[kw]; ok {
				for _, id := range moteIDs {
					candidates[id] += 1.0
				}
			}
		}
	}

	// Tag matching
	for _, m := range ss.motes {
		overlap := tagOverlap(keywords, m.Tags)
		if overlap > 0 {
			candidates[m.ID] += float64(overlap)
		}
	}

	// Title fallback: if no tag matches, try title substring
	if len(candidates) == 0 && len(keywords) > 0 {
		for _, m := range ss.motes {
			titleLower := strings.ToLower(m.Title)
			for _, kw := range keywords {
				if strings.Contains(titleLower, kw) {
					candidates[m.ID] += 0.5
				}
			}
		}
	}

	// Body-text keyword matching (lower boost than tags/title)
	if len(keywords) > 0 {
		for _, m := range ss.motes {
			bodyKeywords := extractKeywords(m.Body)
			overlap := keywordOverlap(keywords, bodyKeywords)
			if overlap > 0 {
				candidates[m.ID] += float64(overlap) * 0.3
			}
		}
	}

	// BM25 peer signal
	if ss.searcher != nil && len(keywords) > 0 {
		results := ss.searcher.Search(topic, 20)
		if len(results) > 0 {
			// Normalize by max score to get 0-1 range
			maxScore := results[0].Score
			for _, r := range results {
				if r.Score > maxScore {
					maxScore = r.Score
				}
			}
			if maxScore > 0 {
				for _, r := range results {
					candidates[r.ID] += (r.Score / maxScore) * 0.5
				}
			}
		}
	}

	// Ambient signals
	if ambient != nil {
		for _, signal := range ss.signals {
			switch signal.Type {
			case "built_in":
				ss.applyBuiltinSignal(signal.Name, ambient, candidates)
			case "co_access":
				ss.applyCoAccessSignal(signal, candidates)
			}
		}
	}

	return ss.topN(candidates, 10)
}

// MatchedTags returns the keywords from topic that match a mote's tags.
func MatchedTags(topic string, mote *Mote) []string {
	keywords := extractKeywords(topic)
	var matched []string
	for _, kw := range keywords {
		for _, tag := range mote.Tags {
			if strings.EqualFold(kw, tag) {
				matched = append(matched, tag)
				break
			}
		}
	}
	return matched
}

func (ss *SeedSelector) applyBuiltinSignal(name string, ambient *AmbientContext, candidates map[string]float64) {
	switch name {
	case "git_branch":
		if ambient.GitBranch != "" {
			keywords := extractKeywords(ambient.GitBranch)
			ss.matchKeywordsToTags(keywords, candidates, 0.5)
		}
	case "recent_files":
		if len(ambient.RecentFiles) > 0 {
			keywords := extractKeywordsFromPaths(ambient.RecentFiles)
			ss.matchKeywordsToTags(keywords, candidates, 0.3)
		}
	case "prompt_keywords":
		if ambient.PromptText != "" {
			keywords := extractKeywords(ambient.PromptText)
			ss.matchKeywordsToTags(keywords, candidates, 0.8)
		}
	case "context_hint":
		if ambient.ContextHint != "" {
			keywords := extractKeywords(ambient.ContextHint)
			ss.matchKeywordsToTags(keywords, candidates, 0.7)
		}
	}
}

func (ss *SeedSelector) applyCoAccessSignal(signal SignalConfig, candidates map[string]float64) {
	if len(signal.TriggerTags) == 0 || len(signal.BoostTags) == 0 {
		return
	}
	boost := signal.BoostAmount
	if boost == 0 {
		boost = 0.3
	}

	// Check if any candidate mote has a trigger tag
	triggered := false
	for _, m := range ss.motes {
		if candidates[m.ID] <= 0 {
			continue
		}
		for _, tag := range m.Tags {
			for _, trigger := range signal.TriggerTags {
				if strings.EqualFold(tag, trigger) {
					triggered = true
					break
				}
			}
			if triggered {
				break
			}
		}
		if triggered {
			break
		}
	}

	if !triggered {
		return
	}

	// Boost motes that have any boost tag
	for _, m := range ss.motes {
		for _, tag := range m.Tags {
			for _, bt := range signal.BoostTags {
				if strings.EqualFold(tag, bt) {
					candidates[m.ID] += boost
					break
				}
			}
		}
	}
}

func (ss *SeedSelector) matchKeywordsToTags(keywords []string, candidates map[string]float64, boost float64) {
	for _, m := range ss.motes {
		overlap := tagOverlap(keywords, m.Tags)
		if overlap > 0 {
			candidates[m.ID] += float64(overlap) * boost
		}
	}
}

func (ss *SeedSelector) topN(candidates map[string]float64, n int) []*Mote {
	type scored struct {
		mote  *Mote
		score float64
	}

	moteMap := make(map[string]*Mote, len(ss.motes))
	for _, m := range ss.motes {
		moteMap[m.ID] = m
	}

	var items []scored
	for id, s := range candidates {
		if m, ok := moteMap[id]; ok {
			items = append(items, scored{mote: m, score: s})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	if len(items) > n {
		items = items[:n]
	}

	result := make([]*Mote, len(items))
	for i, item := range items {
		result[i] = item.mote
	}
	return result
}

func tagOverlap(keywords, tags []string) int {
	count := 0
	for _, kw := range keywords {
		for _, tag := range tags {
			if strings.EqualFold(kw, tag) {
				count++
				break
			}
		}
	}
	return count
}

func keywordOverlap(query, target []string) int {
	targetSet := make(map[string]bool, len(target))
	for _, t := range target {
		targetSet[t] = true
	}
	count := 0
	for _, q := range query {
		if targetSet[q] {
			count++
		}
	}
	return count
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "of": true,
	"to": true, "in": true, "for": true, "and": true, "or": true,
	"with": true, "on": true, "at": true, "by": true, "from": true,
}

// ExtractKeywords splits a string into lowercase keyword tokens,
// removing stop words and short words.
func ExtractKeywords(s string) []string {
	return extractKeywords(s)
}

func extractKeywords(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := make(map[string]bool)
	var result []string
	for _, w := range words {
		if len(w) < 2 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		result = append(result, w)
	}
	return result
}

func extractKeywordsFromPaths(paths []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range paths {
		for _, kw := range extractKeywords(p) {
			if !seen[kw] {
				seen[kw] = true
				result = append(result, kw)
			}
		}
	}
	return result
}

// CollectAmbientContext gathers signals from the environment.
func CollectAmbientContext() *AmbientContext {
	ctx := &AmbientContext{}

	// Git branch
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err == nil {
		ctx.GitBranch = strings.TrimSpace(string(out))
	}

	// Recent files from git diff
	diffOut, diffErr := exec.Command("git", "diff", "--name-only", "HEAD").Output()
	if diffErr == nil {
		for _, f := range strings.Split(strings.TrimSpace(string(diffOut)), "\n") {
			if f != "" {
				ctx.RecentFiles = append(ctx.RecentFiles, f)
			}
		}
	}

	// Context hint file
	cwd, _ := os.Getwd()
	hintPath := filepath.Join(cwd, ".memory", ".context-hint")
	if data, err := os.ReadFile(hintPath); err == nil {
		ctx.ContextHint = strings.TrimSpace(string(data))
	}

	return ctx
}
