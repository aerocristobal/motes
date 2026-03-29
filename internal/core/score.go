package core

import (
	"math"
	"time"
)

// ScoreEngine computes relevance scores for motes using the 8-component formula.
type ScoreEngine struct {
	config   ScoringConfig
	tagStats map[string]int
}

// ScoringContext provides per-mote context for scoring.
type ScoringContext struct {
	MatchedTags          []string
	EdgeType             string // "seed" for initial seeds, edge type for traversed
	ActiveContradictions int
}

// NewScoreEngine creates a ScoreEngine from config and tag statistics.
func NewScoreEngine(cfg ScoringConfig, tagStats map[string]int) *ScoreEngine {
	return &ScoreEngine{config: cfg, tagStats: tagStats}
}

// Score computes the final relevance score for a mote.
//
// Formula: (base + edge_bonus + status_penalty + retrieval_bonus +
//
//	salience_bonus + tag_specificity + interference_penalty) × recency_factor
func (se *ScoreEngine) Score(mote *Mote, ctx ScoringContext) float64 {
	base := mote.Weight

	edgeBonus := se.config.EdgeBonuses[ctx.EdgeType]

	statusPenalty := se.config.StatusPenalties[mote.Status]

	recency := se.recencyFactor(mote.LastAccessed)

	retrievalBonus := math.Min(
		float64(mote.AccessCount)*se.config.RetrievalStrength.PerAccess,
		se.config.RetrievalStrength.MaxBonus,
	)

	salienceBonus := se.config.Salience[mote.Origin]
	if mote.Type == "explore" {
		salienceBonus += se.config.ExploreTypeBonus
	}

	tagBonus := se.tagSpecificity(ctx.MatchedTags)

	interference := float64(ctx.ActiveContradictions) * se.config.InterferencePenalty

	raw := base + edgeBonus + statusPenalty + retrievalBonus +
		salienceBonus + tagBonus + interference
	return raw * recency
}

func (se *ScoreEngine) recencyFactor(lastAccessed *time.Time) float64 {
	if lastAccessed == nil {
		// nil = never accessed → worst tier
		return se.config.RecencyDecay.Tiers[len(se.config.RecencyDecay.Tiers)-1].Factor
	}
	days := int(time.Since(*lastAccessed).Hours() / 24)
	for _, tier := range se.config.RecencyDecay.Tiers {
		if tier.MaxDays != nil && days < *tier.MaxDays {
			return tier.Factor
		}
	}
	return se.config.RecencyDecay.Tiers[len(se.config.RecencyDecay.Tiers)-1].Factor
}

func (se *ScoreEngine) tagSpecificity(tags []string) float64 {
	if len(tags) == 0 {
		return 0.0
	}
	var total float64
	for _, tag := range tags {
		count := se.tagStats[tag]
		total += 1.0 / math.Log2(float64(count)+2)
	}
	return (total / float64(len(tags))) * se.config.TagSpecificity.Weight
}

// DecayRiskInfo describes a high-value mote approaching the relevance threshold.
type DecayRiskInfo struct {
	MoteID        string
	Weight        float64
	RecencyFactor float64
	Score         float64
	ScoreGap      float64 // score - min_relevance_threshold (positive = still above threshold)
}

// DecayRiskMotes returns active motes with weight≥0.7 and failure/explore origin
// whose last-access is 30–90 days ago and whose conservative score is within 0.1
// of the min_relevance_threshold.
func DecayRiskMotes(motes []*Mote, cfg *Config) []DecayRiskInfo {
	se := NewScoreEngine(cfg.Scoring, nil)
	minThreshold := cfg.Scoring.MinThreshold
	if minThreshold <= 0 {
		minThreshold = 0.25
	}

	var results []DecayRiskInfo
	for _, m := range motes {
		if m.Status != "active" {
			continue
		}
		if m.Weight < 0.7 {
			continue
		}
		if m.Origin != "failure" && m.Type != "explore" {
			continue
		}
		if m.LastAccessed == nil {
			continue
		}
		days := int(time.Since(*m.LastAccessed).Hours() / 24)
		if days < 30 || days >= 90 {
			continue
		}

		rf := se.recencyFactor(m.LastAccessed)

		retrievalBonus := math.Min(
			float64(m.AccessCount)*cfg.Scoring.RetrievalStrength.PerAccess,
			cfg.Scoring.RetrievalStrength.MaxBonus,
		)
		salienceBonus := cfg.Scoring.Salience[m.Origin]
		if m.Type == "explore" {
			salienceBonus += cfg.Scoring.ExploreTypeBonus
		}
		score := (m.Weight + retrievalBonus + salienceBonus) * rf
		scoreGap := score - minThreshold
		if scoreGap < 0 || scoreGap > 0.1 {
			continue
		}
		results = append(results, DecayRiskInfo{
			MoteID:        m.ID,
			Weight:        m.Weight,
			RecencyFactor: rf,
			Score:         score,
			ScoreGap:      scoreGap,
		})
	}
	return results
}
