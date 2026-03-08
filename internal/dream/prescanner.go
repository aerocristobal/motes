package dream

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"motes/internal/core"
	"motes/internal/strata"
)

// PreScanner deterministically identifies dream cycle candidates.
type PreScanner struct {
	moteManager  *core.MoteManager
	indexManager  *core.IndexManager
	root         string
	config       core.DreamConfig
	moteBM25     *strata.BM25Index
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

// SetMoteBM25 sets the mote BM25 index for content similarity scanning.
func (ps *PreScanner) SetMoteBM25(idx *strata.BM25Index) {
	ps.moteBM25 = idx
}

// Scan reads all motes and the index, returning candidates across 9 categories.
// It uses a content-hash cache to skip unchanged motes where possible.
func (ps *PreScanner) Scan() (*ScanResult, error) {
	motes, err := ps.moteManager.ReadAllParallel()
	if err != nil {
		return nil, err
	}
	idx, err := ps.indexManager.Load()
	if err != nil {
		return nil, err
	}

	// Load scan cache and identify changed motes
	cache := LoadScanCache(ps.root)
	changedMotes := FilterChanged(motes, cache)
	_ = SaveScanCache(ps.root, cache)

	// Use changed motes for candidate detection where possible.
	// Some categories (pair comparisons) still need all motes.
	candidateMotes := motes
	if len(changedMotes) > 0 && len(changedMotes) < len(motes) {
		// For single-mote scans (staleness, compression, uncrystallized), only check changed
		candidateMotes = changedMotes
	}

	// findLinkCandidates must run first — contentLinkCandidates filters its results
	linkCandidates := ps.findLinkCandidates(motes, idx)

	var sr ScanResult
	sr.LinkCandidates = linkCandidates

	var wg sync.WaitGroup
	wg.Add(10)
	go func() { defer wg.Done(); sr.ContentLinkCandidates = ps.findContentLinkCandidates(motes, idx, linkCandidates) }()
	go func() { defer wg.Done(); sr.ContradictionCandidates = ps.findContradictionCandidates(motes) }()
	go func() { defer wg.Done(); sr.OverloadedTags = ps.findOverloadedTags(idx.TagStats) }()
	go func() { defer wg.Done(); sr.StaleMotes = ps.findStaleMotes(candidateMotes) }()
	go func() { defer wg.Done(); sr.ConstellationEvolution = ps.findConstellationCandidates(motes) }()
	go func() { defer wg.Done(); sr.CompressionCandidates = ps.findCompressionCandidates(candidateMotes) }()
	go func() { defer wg.Done(); sr.UncrystallizedIssues = ps.findUncrystallized(candidateMotes) }()
	go func() { defer wg.Done(); sr.StrataCrystallization = ps.findStrataCandidates() }()
	go func() { defer wg.Done(); sr.MergeCandidates = ps.findMergeCandidates(motes, idx) }()
	go func() { defer wg.Done(); sr.SummarizationCandidates = ps.findSummarizationCandidates(motes, idx) }()
	sr.SignalCandidates = ps.findSignalPatterns(motes)
	wg.Wait()

	return &sr, nil
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

// findContentLinkCandidates finds mote pairs with high BM25 content similarity
// that don't already have explicit links or tag-overlap candidates.
func (ps *PreScanner) findContentLinkCandidates(motes []*core.Mote, idx *core.EdgeIndex, tagCandidates []MotePair) []MotePair {
	csCfg := ps.config.PreScan.ContentSimilarity
	if !csCfg.Enabled {
		return nil
	}

	bm25Idx := ps.moteBM25
	if bm25Idx == nil || bm25Idx.DocCount == 0 {
		// Build ephemeral index if none provided
		chunks := make([]strata.Chunk, 0, len(motes))
		for _, m := range motes {
			if m.Status == "deprecated" {
				continue
			}
			chunks = append(chunks, strata.Chunk{ID: m.ID, Text: m.Title + " " + m.Body})
		}
		bm25Idx = strata.BuildBM25Index(chunks)
	}

	topK := csCfg.TopK
	if topK <= 0 {
		topK = 3
	}
	minScore := csCfg.MinScore
	if minScore <= 0 {
		minScore = 1.0
	}
	maxTerms := csCfg.MaxTerms
	if maxTerms <= 0 {
		maxTerms = 8
	}

	// Build sets for deduplication: existing links and tag-overlap candidates
	type pairKey struct{ a, b string }
	existingPairs := make(map[pairKey]bool)
	for _, p := range tagCandidates {
		a, b := p.A, p.B
		if a > b {
			a, b = b, a
		}
		existingPairs[pairKey{a, b}] = true
	}

	// Build linked set from edge index
	linkedSet := make(map[pairKey]bool)
	for _, m := range motes {
		if m.Status == "deprecated" {
			continue
		}
		edges := idx.Neighbors(m.ID, nil)
		for _, e := range edges {
			a, b := m.ID, e.Target
			if a > b {
				a, b = b, a
			}
			linkedSet[pairKey{a, b}] = true
		}
	}

	// Use adaptive threshold when config doesn't specify one
	if minScore <= 0 || minScore == 1.0 {
		minScore = bm25Idx.ThresholdFor("content_similarity")
	}

	seen := make(map[pairKey]bool)
	var candidates []MotePair
	var allScores []float64

	for _, m := range motes {
		if m.Status == "deprecated" {
			continue
		}
		similar := bm25Idx.FindSimilar(m.ID, topK, minScore, maxTerms)
		for _, sr := range similar {
			allScores = append(allScores, sr.Score)
			a, b := m.ID, sr.DocID
			if a > b {
				a, b = b, a
			}
			pk := pairKey{a, b}
			if seen[pk] || existingPairs[pk] || linkedSet[pk] {
				continue
			}
			seen[pk] = true
			candidates = append(candidates, MotePair{
				A:          pk.a,
				B:          pk.b,
				Similarity: sr.Score,
				Source:      "content_similarity",
			})
		}
	}

	// Calibrate BM25 index with observed scores for future use
	if len(allScores) > 0 {
		bm25Idx.SetCalibration(allScores)
	}

	return candidates
}

// findMergeCandidates identifies clusters of 3+ highly similar motes using union-find
// over BM25 similarity pairs with a threshold of 2x the content_similarity minScore.
func (ps *PreScanner) findMergeCandidates(motes []*core.Mote, idx *core.EdgeIndex) []MergeCluster {
	csCfg := ps.config.PreScan.ContentSimilarity
	if !csCfg.Enabled {
		return nil
	}

	bm25Idx := ps.moteBM25
	if bm25Idx == nil || bm25Idx.DocCount == 0 {
		chunks := make([]strata.Chunk, 0, len(motes))
		for _, m := range motes {
			if m.Status == "deprecated" {
				continue
			}
			chunks = append(chunks, strata.Chunk{ID: m.ID, Text: m.Title + " " + m.Body})
		}
		bm25Idx = strata.BuildBM25Index(chunks)
	}

	minScore := csCfg.MinScore
	if minScore <= 0 {
		minScore = 1.0
	}
	multiplier := ps.config.PreScan.MergeSimilarityMultiplier
	if multiplier <= 0 {
		multiplier = 2.0
	}
	mergeThreshold := minScore * multiplier

	maxTerms := csCfg.MaxTerms
	if maxTerms <= 0 {
		maxTerms = 8
	}

	// Build adjacency list from BM25 similarity pairs above threshold
	type pairKey struct{ a, b string }
	pairScores := map[pairKey]float64{}
	adj := map[string][]string{}

	for _, m := range motes {
		if m.Status == "deprecated" {
			continue
		}
		similar := bm25Idx.FindSimilar(m.ID, 5, mergeThreshold, maxTerms)
		for _, sr := range similar {
			a, b := m.ID, sr.DocID
			if a > b {
				a, b = b, a
			}
			pk := pairKey{a, b}
			if _, seen := pairScores[pk]; !seen {
				pairScores[pk] = sr.Score
				adj[a] = append(adj[a], b)
				adj[b] = append(adj[b], a)
			}
		}
	}

	// Union-find to extract connected components
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" {
			parent[x] = x
		}
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	for pk := range pairScores {
		union(pk.a, pk.b)
	}

	// Group into components
	components := map[string][]string{}
	for id := range adj {
		root := find(id)
		components[root] = append(components[root], id)
	}

	// Filter to size >= 3 and compute avg similarity
	var clusters []MergeCluster
	for _, members := range components {
		if len(members) < 3 {
			continue
		}
		var totalScore float64
		var count int
		for i := 0; i < len(members); i++ {
			for j := i + 1; j < len(members); j++ {
				a, b := members[i], members[j]
				if a > b {
					a, b = b, a
				}
				if s, ok := pairScores[pairKey{a, b}]; ok {
					totalScore += s
					count++
				}
			}
		}
		avgSim := 0.0
		if count > 0 {
			avgSim = totalScore / float64(count)
		}
		clusters = append(clusters, MergeCluster{
			MoteIDs:       members,
			AvgSimilarity: avgSim,
		})
	}

	return clusters
}

// findSummarizationCandidates finds clusters of 5+ completed motes with 2+ shared tags
// that haven't been summarized yet.
func (ps *PreScanner) findSummarizationCandidates(motes []*core.Mote, idx *core.EdgeIndex) []SummarizationCluster {
	// Collect completed motes
	var completed []*core.Mote
	for _, m := range motes {
		if m.Status == "completed" {
			completed = append(completed, m)
		}
	}
	if len(completed) < 5 {
		return nil
	}

	// Find motes that are already summarized (linked from a builds_on of a context mote)
	summarized := map[string]bool{}
	for _, m := range motes {
		if m.Type == "context" && strings.Contains(m.Title, "Summary") {
			for _, id := range m.BuildsOn {
				summarized[id] = true
			}
		}
	}

	// Filter out already-summarized motes
	var unsummarized []*core.Mote
	for _, m := range completed {
		if !summarized[m.ID] {
			unsummarized = append(unsummarized, m)
		}
	}
	if len(unsummarized) < 5 {
		return nil
	}

	// Build tag->mote index for completed motes
	tagMotes := map[string][]*core.Mote{}
	for _, m := range unsummarized {
		for _, tag := range m.Tags {
			tagMotes[tag] = append(tagMotes[tag], m)
		}
	}

	// Find tag pairs with 5+ members
	type tagPair struct{ a, b string }
	pairMembers := map[tagPair]map[string]*core.Mote{}
	for tag1, motes1 := range tagMotes {
		moteSet1 := map[string]bool{}
		for _, m := range motes1 {
			moteSet1[m.ID] = true
		}
		for tag2, motes2 := range tagMotes {
			if tag2 <= tag1 {
				continue
			}
			for _, m := range motes2 {
				if moteSet1[m.ID] {
					pair := tagPair{tag1, tag2}
					if pairMembers[pair] == nil {
						pairMembers[pair] = map[string]*core.Mote{}
					}
					pairMembers[pair][m.ID] = m
				}
			}
		}
	}

	var clusters []SummarizationCluster
	seen := map[string]bool{} // prevent same mote appearing in multiple clusters
	for pair, members := range pairMembers {
		if len(members) < 5 {
			continue
		}
		var ids []string
		for id := range members {
			if !seen[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) < 5 {
			continue
		}
		for _, id := range ids {
			seen[id] = true
		}
		clusters = append(clusters, SummarizationCluster{
			SharedTags: []string{pair.a, pair.b},
			MoteIDs:    ids,
		})
	}
	return clusters
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
