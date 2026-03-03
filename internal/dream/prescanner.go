package dream

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motes/internal/core"
)

// PreScanner deterministically identifies dream cycle candidates.
type PreScanner struct {
	moteManager  *core.MoteManager
	indexManager  *core.IndexManager
	root         string
	config       core.DreamConfig
}

// NewPreScanner creates a pre-scanner with the given managers.
func NewPreScanner(root string, mm *core.MoteManager, im *core.IndexManager, cfg core.DreamConfig) *PreScanner {
	return &PreScanner{
		moteManager:  mm,
		indexManager:  im,
		root:         root,
		config:       cfg,
	}
}

// Scan reads all motes and the index, returning candidates across 9 categories.
func (ps *PreScanner) Scan() (*ScanResult, error) {
	motes, err := ps.moteManager.ReadAllParallel()
	if err != nil {
		return nil, err
	}
	idx, err := ps.indexManager.Load()
	if err != nil {
		return nil, err
	}
	return &ScanResult{
		LinkCandidates:          ps.findLinkCandidates(motes, idx),
		ContradictionCandidates: ps.findContradictionCandidates(motes),
		OverloadedTags:          ps.findOverloadedTags(idx.TagStats),
		StaleMotes:              ps.findStaleMotes(motes),
		ConstellationEvolution:  ps.findConstellationCandidates(motes),
		CompressionCandidates:   ps.findCompressionCandidates(motes),
		UncrystallizedIssues:    ps.findUncrystallized(motes),
		StrataCrystallization:   ps.findStrataCandidates(),
		SignalCandidates:        ps.findSignalPatterns(motes),
	}, nil
}

// findLinkCandidates finds pairs of motes with shared tags but no direct link.
func (ps *PreScanner) findLinkCandidates(motes []*core.Mote, idx *core.EdgeIndex) []MotePair {
	minShared := ps.config.PreScan.LinkCandidateMinSharedTags
	if minShared <= 0 {
		minShared = 3
	}

	// Build tag->mote mapping
	tagMotes := map[string][]string{}
	for _, m := range motes {
		if m.Status == "deprecated" {
			continue
		}
		for _, tag := range m.Tags {
			tagMotes[tag] = append(tagMotes[tag], m.ID)
		}
	}

	// Find pairs with enough shared tags
	type pairKey struct{ a, b string }
	pairTags := map[pairKey][]string{}

	for tag, ids := range tagMotes {
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				a, b := ids[i], ids[j]
				if a > b {
					a, b = b, a
				}
				pk := pairKey{a, b}
				pairTags[pk] = append(pairTags[pk], tag)
			}
		}
	}

	// Filter by minShared and no existing edge
	var candidates []MotePair
	for pk, tags := range pairTags {
		if len(tags) < minShared {
			continue
		}
		// Check if already linked
		edges := idx.Neighbors(pk.a, nil)
		alreadyLinked := false
		for _, e := range edges {
			if e.Target == pk.b {
				alreadyLinked = true
				break
			}
		}
		if !alreadyLinked {
			candidates = append(candidates, MotePair{A: pk.a, B: pk.b, SharedTags: tags})
		}
	}
	return candidates
}

// findContradictionCandidates finds active mote pairs with conflicting terminology.
func (ps *PreScanner) findContradictionCandidates(motes []*core.Mote) []MotePair {
	// Simple heuristic: decisions/lessons with overlapping tags and opposite keywords
	var active []*core.Mote
	for _, m := range motes {
		if m.Status == "active" && (m.Type == "decision" || m.Type == "lesson") {
			active = append(active, m)
		}
	}

	negators := []string{"not", "never", "avoid", "instead", "don't", "shouldn't", "deprecated", "replaced"}

	var candidates []MotePair
	for i := 0; i < len(active); i++ {
		for j := i + 1; j < len(active); j++ {
			shared := sharedTags(active[i], active[j])
			if len(shared) == 0 {
				continue
			}
			// Check if bodies have opposing sentiment markers
			bodyA := strings.ToLower(active[i].Body)
			bodyB := strings.ToLower(active[j].Body)
			hasNeg := false
			for _, neg := range negators {
				aHas := strings.Contains(bodyA, neg)
				bHas := strings.Contains(bodyB, neg)
				if aHas != bHas {
					hasNeg = true
					break
				}
			}
			if hasNeg {
				candidates = append(candidates, MotePair{A: active[i].ID, B: active[j].ID, SharedTags: shared})
			}
		}
	}
	return candidates
}

// findOverloadedTags returns tags with count exceeding threshold.
func (ps *PreScanner) findOverloadedTags(tagStats map[string]int) []TagOverload {
	threshold := ps.config.PreScan.TagOverloadThreshold
	if threshold <= 0 {
		threshold = 15
	}
	var overloaded []TagOverload
	for tag, count := range tagStats {
		if count > threshold {
			overloaded = append(overloaded, TagOverload{Tag: tag, Count: count})
		}
	}
	return overloaded
}

// findStaleMotes returns IDs of active motes not accessed within the threshold.
func (ps *PreScanner) findStaleMotes(motes []*core.Mote) []string {
	thresholdDays := ps.config.PreScan.StalenessThresholdDays
	if thresholdDays <= 0 {
		thresholdDays = 180
	}
	threshold := time.Duration(thresholdDays) * 24 * time.Hour
	now := time.Now()

	var stale []string
	for _, m := range motes {
		if m.Status != "active" {
			continue
		}
		if m.LastAccessed == nil || now.Sub(*m.LastAccessed) > threshold {
			stale = append(stale, m.ID)
		}
	}
	return stale
}

// constellationRecord mirrors the record stored in constellations.jsonl.
type constellationRecord struct {
	Tag                 string   `json:"tag"`
	ConstellationMoteID string   `json:"constellation_mote_id"`
	MemberMoteIDs       []string `json:"member_mote_ids"`
}

// findConstellationCandidates checks existing constellations for membership growth.
// A constellation is flagged if its tag's current mote count has grown >= ThemeGrowthThresholdPct
// beyond the recorded member count.
func (ps *PreScanner) findConstellationCandidates(motes []*core.Mote) []ConstellationEvolution {
	// Read constellations.jsonl for recorded member counts
	cPath := filepath.Join(ps.root, "constellations.jsonl")
	data, err := os.ReadFile(cPath)
	if err != nil {
		return nil
	}

	// Parse records
	var records []constellationRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var r constellationRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		records = append(records, r)
	}
	if len(records) == 0 {
		return nil
	}

	// Build current tag counts (non-deprecated motes only)
	tagCounts := map[string]int{}
	for _, m := range motes {
		if m.Status == "deprecated" {
			continue
		}
		for _, tag := range m.Tags {
			tagCounts[tag]++
		}
	}

	growthPct := ps.config.PreScan.ThemeGrowthThresholdPct
	if growthPct <= 0 {
		growthPct = 30
	}

	var candidates []ConstellationEvolution
	for _, r := range records {
		oldCount := len(r.MemberMoteIDs)
		if oldCount == 0 {
			continue
		}
		newCount := tagCounts[r.Tag]
		growth := float64(newCount-oldCount) / float64(oldCount) * 100
		if growth >= float64(growthPct) {
			candidates = append(candidates, ConstellationEvolution{
				ConstellationID: r.ConstellationMoteID,
				Tag:             r.Tag,
				OldCount:        oldCount,
				NewCount:        newCount,
			})
		}
	}
	return candidates
}

// findCompressionCandidates finds verbose motes exceeding word threshold.
func (ps *PreScanner) findCompressionCandidates(motes []*core.Mote) []string {
	minWords := ps.config.PreScan.CompressionMinWords
	if minWords <= 0 {
		minWords = 300
	}
	var candidates []string
	for _, m := range motes {
		if m.Status != "active" {
			continue
		}
		words := len(strings.Fields(m.Body))
		if words > minWords {
			candidates = append(candidates, m.ID)
		}
	}
	return candidates
}

// findUncrystallized finds completed/archived motes with no corresponding crystallized mote.
func (ps *PreScanner) findUncrystallized(motes []*core.Mote) []string {
	sourceIssueSet := map[string]bool{}
	for _, m := range motes {
		if m.SourceIssue != "" {
			sourceIssueSet[m.SourceIssue] = true
		}
	}
	var candidates []string
	for _, m := range motes {
		if (m.Status == "completed" || m.Status == "archived") && !sourceIssueSet[m.ID] {
			candidates = append(candidates, m.ID)
		}
	}
	return candidates
}

// findStrataCandidates analyzes query_log.jsonl for frequently queried topics.
func (ps *PreScanner) findStrataCandidates() []StrataCrystallizationCandidate {
	queryPath := filepath.Join(ps.root, "strata", "query_log.jsonl")
	data, err := os.ReadFile(queryPath)
	if err != nil {
		return nil
	}

	type queryEntry struct {
		Corpus string `json:"corpus"`
		Query  string `json:"query"`
	}

	counts := map[string]int{} // "corpus:query" -> count
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e queryEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		key := e.Corpus + ":" + e.Query
		counts[key]++
	}

	var candidates []StrataCrystallizationCandidate
	for key, count := range counts {
		if count >= 3 {
			parts := strings.SplitN(key, ":", 2)
			candidates = append(candidates, StrataCrystallizationCandidate{
				Corpus:     parts[0],
				Query:      parts[1],
				QueryCount: count,
			})
		}
	}
	return candidates
}

// findSignalPatterns identifies motes with co-access patterns suggesting priming signals.
func (ps *PreScanner) findSignalPatterns(motes []*core.Mote) []SignalCandidate {
	// Heuristic: motes accessed many times relative to their age may indicate signal patterns
	now := time.Now()
	var candidates []SignalCandidate
	for _, m := range motes {
		if m.Status != "active" || m.AccessCount < 5 {
			continue
		}
		ageDays := now.Sub(m.CreatedAt).Hours() / 24
		if ageDays < 1 {
			ageDays = 1
		}
		accessRate := float64(m.AccessCount) / ageDays
		if accessRate > 0.5 { // accessed more than every other day
			candidates = append(candidates, SignalCandidate{
				MoteID:  m.ID,
				Pattern: "high_access_rate",
			})
		}
	}
	return candidates
}

func sharedTags(a, b *core.Mote) []string {
	bTags := map[string]bool{}
	for _, t := range b.Tags {
		bTags[t] = true
	}
	var shared []string
	for _, t := range a.Tags {
		if bTags[t] {
			shared = append(shared, t)
		}
	}
	return shared
}
