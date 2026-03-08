package dream

// ScoreConfidence computes a deterministic confidence score (0.0-1.0) for a vision
// based on historical performance, structural completeness, severity, source mote quality,
// and self-consistency agreement.
func ScoreConfidence(v Vision, stats map[string]*FeedbackStats, preScores map[string]float64) float64 {
	const (
		wHistory    = 0.35
		wStructure  = 0.20
		wSeverity   = 0.15
		wMoteQuality = 0.10
		wAgreement  = 0.20
	)

	hist := scoreHistorical(v.Type, stats)
	structure := scoreStructure(v)
	severity := scoreSeverity(v.Severity)
	quality := scoreMoteQuality(v, preScores)
	agreement := scoreAgreement(v.Agreement)

	score := wHistory*hist + wStructure*structure + wSeverity*severity + wMoteQuality*quality + wAgreement*agreement

	// Clamp to [0.0, 1.0]
	if score < 0.0 {
		return 0.0
	}
	if score > 1.0 {
		return 1.0
	}
	return score
}

// scoreAgreement returns a 0.0-1.0 score based on self-consistency agreement.
// When agreement is 0 (self-consistency disabled / single run), returns 1.0 (neutral).
func scoreAgreement(agreement float64) float64 {
	if agreement <= 0 {
		return 1.0 // neutral when voting is disabled
	}
	return agreement
}

// scoreHistorical returns a 0.0-1.0 score based on past feedback for this vision type.
// Cold start (no history) returns 0.5 (neutral).
func scoreHistorical(visionType string, stats map[string]*FeedbackStats) float64 {
	s, ok := stats[visionType]
	if !ok || s.Total == 0 {
		return 0.5 // neutral cold start
	}

	// Three sub-signals, equally weighted:
	// 1. Persistence rate (checked entries that persisted)
	var persistRate float64
	if s.Checked > 0 {
		persistRate = float64(s.Persisted) / float64(s.Checked)
	} else {
		persistRate = 0.5
	}

	// 2. Positive delta percentage (0-100 → 0-1)
	posRate := s.PositivePct / 100.0
	if posRate > 1.0 {
		posRate = 1.0
	}

	// 3. Average delta mapped to 0-1 (delta typically -0.5 to +0.5)
	deltaScore := 0.5 + s.AvgDelta
	if deltaScore < 0.0 {
		deltaScore = 0.0
	}
	if deltaScore > 1.0 {
		deltaScore = 1.0
	}

	return (persistRate + posRate + deltaScore) / 3.0
}

// scoreStructure returns 0.0-1.0 based on how complete the vision's fields are for its type.
func scoreStructure(v Vision) float64 {
	var filled, total float64

	// Common required fields
	total += 3 // type, action, rationale
	if v.Type != "" {
		filled++
	}
	if v.Action != "" {
		filled++
	}
	if v.Rationale != "" {
		filled++
	}

	// Rationale length bonus (longer = more thought)
	if len(v.Rationale) > 50 {
		filled += 0.5
		total += 0.5
	} else {
		total += 0.5
	}

	// Type-specific fields
	switch v.Type {
	case "link_suggestion":
		total += 3 // source, target, link_type
		if len(v.SourceMotes) > 0 {
			filled++
		}
		if len(v.TargetMotes) > 0 {
			filled++
		}
		if v.LinkType != "" {
			filled++
		}
	case "contradiction":
		total += 1 // need at least 2 source motes
		if len(v.SourceMotes) >= 2 {
			filled++
		}
	case "tag_refinement":
		total += 2 // source motes, tags
		if len(v.SourceMotes) > 0 {
			filled++
		}
		if len(v.Tags) > 0 {
			filled++
		}
	case "staleness":
		total++ // source motes
		if len(v.SourceMotes) > 0 {
			filled++
		}
	case "compression":
		total += 2 // source motes, rationale (as compressed body)
		if len(v.SourceMotes) > 0 {
			filled++
		}
		if len(v.Rationale) > 100 {
			filled++
		}
	case "signal":
		total += 2 // action (signal name), tags
		if v.Action != "" {
			filled++ // already counted above, but signal needs it specifically
		}
		if len(v.Tags) > 0 {
			filled++
		}
	case "merge_suggestion":
		total += 3 // source motes >= 3, tags, rationale length
		if len(v.SourceMotes) >= 3 {
			filled++
		}
		if len(v.Tags) > 0 {
			filled++
		}
		if len(v.Rationale) > 100 {
			filled++
		}
	default:
		total++ // source motes for unknown types
		if len(v.SourceMotes) > 0 {
			filled++
		}
	}

	if total == 0 {
		return 0.5
	}
	return filled / total
}

// scoreSeverity maps severity string to a 0.0-1.0 score.
func scoreSeverity(severity string) float64 {
	switch severity {
	case "high":
		return 0.9
	case "medium":
		return 0.6
	case "low":
		return 0.3
	default:
		return 0.5 // empty or unknown
	}
}

// scoreMoteQuality returns the average of preScores for affected motes, normalized to 0.0-1.0.
func scoreMoteQuality(v Vision, preScores map[string]float64) float64 {
	if len(preScores) == 0 {
		return 0.5 // neutral when no scores available
	}

	ids := AffectedMoteIDs(v)
	if len(ids) == 0 {
		return 0.5
	}

	var sum float64
	var count int
	for _, id := range ids {
		if score, ok := preScores[id]; ok {
			sum += score
			count++
		}
	}

	if count == 0 {
		return 0.5
	}

	avg := sum / float64(count)
	// Normalize: scores typically range 0-2, map to 0-1
	normalized := avg / 2.0
	if normalized > 1.0 {
		return 1.0
	}
	if normalized < 0.0 {
		return 0.0
	}
	return normalized
}
