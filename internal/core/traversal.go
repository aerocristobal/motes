package core

import (
	"sort"
)

// GraphTraverser performs BFS with hop-limited spreading activation.
type GraphTraverser struct {
	index        *EdgeIndex
	scorer       *ScoreEngine
	maxHops      int
	maxResults   int
	minThreshold float64
}

// ScoredMote is a mote with its computed relevance score.
type ScoredMote struct {
	Mote  *Mote
	Score float64
	Hop   int
}

// NewGraphTraverser creates a traverser from an edge index and scorer.
func NewGraphTraverser(index *EdgeIndex, scorer *ScoreEngine, cfg ScoringConfig) *GraphTraverser {
	return &GraphTraverser{
		index:        index,
		scorer:       scorer,
		maxHops:      cfg.MaxHops,
		maxResults:   cfg.MaxResults,
		minThreshold: cfg.MinThreshold,
	}
}

type frontierItem struct {
	id       string
	hop      int
	edgeType string
}

// Traverse performs BFS from seed motes, scoring and filtering by threshold.
// reader is called to load motes by ID (typically mm.Read).
func (gt *GraphTraverser) Traverse(seeds []*Mote, matchedTags []string, reader func(string) (*Mote, error)) []ScoredMote {
	visited := make(map[string]*ScoredMote)
	var queue []frontierItem

	// Score seeds
	for _, seed := range seeds {
		ctx := ScoringContext{
			MatchedTags:          matchedTags,
			EdgeType:             "seed",
			ActiveContradictions: gt.countActiveContradictions(seed.ID, visited),
		}
		score := gt.scorer.Score(seed, ctx)
		visited[seed.ID] = &ScoredMote{Mote: seed, Score: score, Hop: 0}
		queue = append(queue, frontierItem{id: seed.ID, hop: 0})
	}

	// BFS expansion
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.hop >= gt.maxHops {
			continue
		}

		neighbors := gt.index.Neighbors(current.id, nil)
		for _, edge := range neighbors {
			if _, seen := visited[edge.Target]; seen {
				continue
			}

			targetMote, err := reader(edge.Target)
			if err != nil {
				continue
			}

			ctx := ScoringContext{
				EdgeType:             edge.EdgeType,
				ActiveContradictions: gt.countActiveContradictions(edge.Target, visited),
			}
			score := gt.scorer.Score(targetMote, ctx)
			if score < gt.minThreshold {
				continue
			}

			nextHop := current.hop + 1
			visited[edge.Target] = &ScoredMote{Mote: targetMote, Score: score, Hop: nextHop}
			queue = append(queue, frontierItem{id: edge.Target, hop: nextHop})
		}
	}

	// Collect, sort, cap
	result := make([]ScoredMote, 0, len(visited))
	for _, sm := range visited {
		result = append(result, *sm)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	if len(result) > gt.maxResults {
		result = result[:gt.maxResults]
	}
	return result
}

// countActiveContradictions counts how many active motes in the visited set
// contradict the given moteID.
func (gt *GraphTraverser) countActiveContradictions(moteID string, visited map[string]*ScoredMote) int {
	count := 0
	contradictions := gt.index.Neighbors(moteID, map[string]bool{"contradicts": true})
	for _, edge := range contradictions {
		if sm, ok := visited[edge.Target]; ok && sm.Mote.Status != "deprecated" {
			count++
		}
	}
	return count
}
