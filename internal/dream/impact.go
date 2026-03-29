package dream

import (
	"fmt"
	"sort"

	"motes/internal/core"
)

// impactContext holds the pre-loaded scoring context for vision impact preview.
// Built once at the start of Review() and reused across all visions in the session.
type impactContext struct {
	motes  []*core.Mote
	idx    *core.EdgeIndex
	scorer *core.ScoreEngine
	cfg    *core.Config
}

// computeVisionImpact returns a short human-readable scoring impact string for a vision.
// Returns "" for vision types without meaningful scoring impact.
func computeVisionImpact(v Vision, ic *impactContext) string {
	switch v.Type {
	case "link_suggestion":
		return linkImpact(v, ic)
	case "staleness":
		if v.Action == "deprecate" {
			return deprecateImpact(v, ic)
		}
	case "constellation":
		return constellationImpact(v, ic)
	}
	return ""
}

func linkImpact(v Vision, ic *impactContext) string {
	if len(v.SourceMotes) == 0 || len(v.TargetMotes) == 0 {
		return ""
	}
	bonus := ic.cfg.Scoring.EdgeBonuses[v.LinkType]
	return fmt.Sprintf("%s gains +%.2f edge bonus when %s is a seed (and vice versa). Both more likely to co-appear in prime output.",
		v.SourceMotes[0], bonus, v.TargetMotes[0])
}

func deprecateImpact(v Vision, ic *impactContext) string {
	if len(v.SourceMotes) == 0 {
		return ""
	}
	sourceID := v.SourceMotes[0]

	type scored struct {
		id    string
		title string
		score float64
	}

	var candidates []scored
	for _, m := range ic.motes {
		if m.Status != "active" {
			continue
		}
		s := ic.scorer.Score(m, core.ScoringContext{EdgeType: "seed"})
		candidates = append(candidates, scored{id: m.ID, title: m.Title, score: s})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	var sourceScore float64
	sourceRank := -1
	for i, c := range candidates {
		if c.id == sourceID {
			sourceScore = c.score
			sourceRank = i
			break
		}
	}

	if sourceRank == -1 {
		return fmt.Sprintf("%s not found in active candidate pool.", sourceID)
	}

	// Find next-in-line: first active mote ranked below source
	for i := sourceRank + 1; i < len(candidates); i++ {
		next := candidates[i]
		title := next.title
		if len(title) > 40 {
			title = title[:40] + "…"
		}
		return fmt.Sprintf("%s (score: %.2f) removed from candidate pool. Next-in-line: %s (score: %.2f, %q).",
			sourceID, sourceScore, next.id, next.score, title)
	}

	return fmt.Sprintf("%s (score: %.2f) removed from candidate pool. No active mote currently below it in ranking.",
		sourceID, sourceScore)
}

func constellationImpact(v Vision, ic *impactContext) string {
	n := len(v.SourceMotes)
	if n == 0 {
		return ""
	}
	bonus := ic.cfg.Scoring.EdgeBonuses["relates_to"]
	theme := ""
	if len(v.Tags) > 0 {
		theme = v.Tags[0]
	}
	if theme != "" {
		return fmt.Sprintf("%d motes gain +%.2f edge bonus via constellation hub %q.", n, bonus, theme)
	}
	return fmt.Sprintf("%d motes gain +%.2f edge bonus via new constellation hub.", n, bonus)
}
