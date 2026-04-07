// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motes/internal/core"
)

// FeedbackEntry records a single auto-applied vision and its quality metrics.
type FeedbackEntry struct {
	Timestamp   string             `json:"timestamp"`
	VisionType  string             `json:"vision_type"`
	Action      string             `json:"action"`
	SourceMotes []string           `json:"source_motes"`
	TargetMotes []string           `json:"target_motes,omitempty"`
	LinkType    string             `json:"link_type,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Confidence  float64            `json:"confidence,omitempty"`
	PreScores   map[string]float64 `json:"pre_scores"`
	PostScores  map[string]float64 `json:"post_scores,omitempty"`
	ScoreDelta  *float64           `json:"score_delta,omitempty"`
	Persisted   *bool              `json:"persisted,omitempty"`
	CheckedAt   string             `json:"checked_at,omitempty"`
}

// FeedbackStats aggregates feedback metrics for a vision type.
type FeedbackStats struct {
	Total         int     `json:"total"`
	Checked       int     `json:"checked"`
	Persisted     int     `json:"persisted"`
	Reverted      int     `json:"reverted"`
	AvgDelta      float64 `json:"avg_score_delta"`
	PositivePct   float64 `json:"positive_delta_pct"`
	AvgConfidence float64 `json:"avg_confidence"`
}

const feedbackFile = "feedback.jsonl"

// RecordApplied writes a feedback entry after a vision is successfully applied.
func RecordApplied(root string, v Vision, preScores map[string]float64) {
	entry := FeedbackEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		VisionType:  v.Type,
		Action:      v.Action,
		SourceMotes: v.SourceMotes,
		TargetMotes: v.TargetMotes,
		LinkType:    v.LinkType,
		Tags:        v.Tags,
		Confidence:  v.Confidence,
		PreScores:   preScores,
	}
	appendFeedbackEntry(root, entry)
}

// SnapshotScores computes retrieval scores for the given mote IDs.
func SnapshotScores(mm *core.MoteManager, cfg *core.Config, moteIDs []string) map[string]float64 {
	scores := make(map[string]float64, len(moteIDs))
	// Build tag stats from all motes for the score engine
	allMotes, err := mm.ReadAllParallel()
	if err != nil {
		return scores
	}
	tagStats := make(map[string]int)
	for _, m := range allMotes {
		for _, tag := range m.Tags {
			tagStats[tag]++
		}
	}
	engine := core.NewScoreEngine(cfg.Scoring, tagStats)

	for _, id := range moteIDs {
		m, err := mm.Read(id)
		if err != nil {
			continue
		}
		ctx := core.ScoringContext{MatchedTags: m.Tags}
		scores[id] = engine.Score(m, ctx)
	}
	return scores
}

// CheckFeedback evaluates unchecked feedback entries by re-scoring affected motes
// and checking persistence. Called at the start of each dream run.
func CheckFeedback(root string, mm *core.MoteManager, im *core.IndexManager, cfg *core.Config) {
	entries := readFeedbackEntries(root)
	if len(entries) == 0 {
		return
	}

	// Build scoring infrastructure once
	allMotes, err := mm.ReadAllParallel()
	if err != nil {
		return
	}
	tagStats := make(map[string]int)
	for _, m := range allMotes {
		for _, tag := range m.Tags {
			tagStats[tag]++
		}
	}
	engine := core.NewScoreEngine(cfg.Scoring, tagStats)

	edgeIndex, _ := im.Load()

	updated := false
	for i := range entries {
		if entries[i].CheckedAt != "" {
			continue // Already checked
		}
		updated = true
		now := time.Now().UTC().Format(time.RFC3339)
		entries[i].CheckedAt = now

		// Compute post-scores and delta
		if entries[i].PreScores != nil {
			postScores := make(map[string]float64)
			var deltaSum float64
			var deltaCount int
			var positiveCount int

			for id, preSc := range entries[i].PreScores {
				m, err := mm.Read(id)
				if err != nil {
					continue
				}
				ctx := core.ScoringContext{MatchedTags: m.Tags}
				postSc := engine.Score(m, ctx)
				postScores[id] = postSc
				d := postSc - preSc
				deltaSum += d
				deltaCount++
				if d > 0 {
					positiveCount++
				}
			}
			entries[i].PostScores = postScores
			if deltaCount > 0 {
				avg := deltaSum / float64(deltaCount)
				entries[i].ScoreDelta = &avg
			}
		}

		// Check persistence
		persisted := checkPersistence(entries[i], mm, edgeIndex)
		entries[i].Persisted = &persisted
	}

	if updated {
		writeFeedbackEntries(root, entries)
	}
}

// checkPersistence verifies that the applied change still exists.
func checkPersistence(e FeedbackEntry, mm *core.MoteManager, edgeIndex *core.EdgeIndex) bool {
	switch e.VisionType {
	case "link_suggestion":
		if len(e.SourceMotes) == 0 || len(e.TargetMotes) == 0 || e.LinkType == "" {
			return false
		}
		if edgeIndex == nil {
			return false
		}
		edges := edgeIndex.Neighbors(e.SourceMotes[0], nil)
		for _, edge := range edges {
			if edge.Target == e.TargetMotes[0] && edge.EdgeType == e.LinkType {
				return true
			}
		}
		return false
	case "contradiction":
		if len(e.SourceMotes) < 2 {
			return false
		}
		if edgeIndex == nil {
			return false
		}
		edges := edgeIndex.Neighbors(e.SourceMotes[0], nil)
		for _, edge := range edges {
			if edge.Target == e.SourceMotes[1] && edge.EdgeType == "contradicts" {
				return true
			}
		}
		return false
	case "staleness":
		if len(e.SourceMotes) == 0 {
			return false
		}
		m, err := mm.Read(e.SourceMotes[0])
		if err != nil {
			return false
		}
		return m.Status == "deprecated"
	case "tag_refinement":
		if len(e.SourceMotes) == 0 || len(e.Tags) == 0 {
			return false
		}
		m, err := mm.Read(e.SourceMotes[0])
		if err != nil {
			return false
		}
		// Check if all expected tags are still present
		tagSet := make(map[string]bool, len(m.Tags))
		for _, t := range m.Tags {
			tagSet[t] = true
		}
		for _, t := range e.Tags {
			if !tagSet[t] {
				return false
			}
		}
		return true
	case "compression":
		if len(e.SourceMotes) == 0 {
			return false
		}
		_, err := mm.Read(e.SourceMotes[0])
		return err == nil // Mote still exists
	case "constellation":
		if len(e.SourceMotes) == 0 {
			return false
		}
		// Check if any constellation mote exists (we don't track the hub ID)
		return true
	case "merge_suggestion":
		// All source motes should be deprecated
		for _, id := range e.SourceMotes {
			m, err := mm.Read(id)
			if err != nil || m.Status != "deprecated" {
				return false
			}
		}
		return true
	case "signal":
		return true // Signal persistence is in config, hard to check generically
	}
	return true
}

// GetStats aggregates feedback entries by vision type.
func GetStats(root string) map[string]*FeedbackStats {
	entries := readFeedbackEntries(root)
	stats := make(map[string]*FeedbackStats)

	for _, e := range entries {
		s, ok := stats[e.VisionType]
		if !ok {
			s = &FeedbackStats{}
			stats[e.VisionType] = s
		}
		s.Total++
		s.AvgConfidence += e.Confidence

		if e.CheckedAt == "" {
			continue
		}
		s.Checked++

		if e.Persisted != nil {
			if *e.Persisted {
				s.Persisted++
			} else {
				s.Reverted++
			}
		}

		if e.ScoreDelta != nil {
			s.AvgDelta += *e.ScoreDelta
			if *e.ScoreDelta > 0 {
				s.PositivePct++
			}
		}
	}

	// Compute averages
	for _, s := range stats {
		if s.Total > 0 {
			s.AvgConfidence /= float64(s.Total)
		}
		if s.Checked > 0 {
			s.AvgDelta /= float64(s.Checked)
			s.PositivePct = (s.PositivePct / float64(s.Checked)) * 100
		}
	}

	return stats
}

// MigrateAutoApplied converts old auto_applied.jsonl to feedback.jsonl.
func MigrateAutoApplied(root string) {
	oldPath := filepath.Join(root, "dream", "auto_applied.jsonl")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return // No old file
	}

	type oldEntry struct {
		Timestamp string `json:"timestamp"`
		Vision    Vision `json:"vision"`
	}

	var entries []FeedbackEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var old oldEntry
		if err := json.Unmarshal([]byte(line), &old); err != nil {
			continue
		}
		entries = append(entries, FeedbackEntry{
			Timestamp:   old.Timestamp,
			VisionType:  old.Vision.Type,
			Action:      old.Vision.Action,
			SourceMotes: old.Vision.SourceMotes,
			TargetMotes: old.Vision.TargetMotes,
			LinkType:    old.Vision.LinkType,
			Tags:        old.Vision.Tags,
		})
	}

	if len(entries) > 0 {
		for _, e := range entries {
			appendFeedbackEntry(root, e)
		}
	}
	os.Remove(oldPath)
}

func feedbackPath(root string) string {
	return filepath.Join(root, "dream", feedbackFile)
}

func appendFeedbackEntry(root string, entry FeedbackEntry) {
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(feedbackPath(root), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte{'\n'})
}

func readFeedbackEntries(root string) []FeedbackEntry {
	f, err := os.Open(feedbackPath(root))
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []FeedbackEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e FeedbackEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

func writeFeedbackEntries(root string, entries []FeedbackEntry) {
	var buf strings.Builder
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	path := feedbackPath(root)
	_ = core.AtomicWrite(path, []byte(buf.String()), 0644)
}

// AffectedMoteIDs returns all mote IDs affected by a vision.
func AffectedMoteIDs(v Vision) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, id := range v.SourceMotes {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range v.TargetMotes {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

