package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/strata"
)

var contextCmd = &cobra.Command{
	Use:   "context <topic...>",
	Short: "Load context for a topic via scoring and graph traversal",
	Args:  cobra.ArbitraryArgs,
	RunE:  runContext,
}

var contextPlanning bool
var contextJSON bool

// ContextOutput is the JSON output structure for mote context --json.
type ContextOutput struct {
	Topic   string      `json:"topic"`
	Results []MoteEntry `json:"results"`
}

func init() {
	contextCmd.Flags().BoolVar(&contextPlanning, "planning", false, "Show dependency chain for a mote ID")
	contextCmd.Flags().BoolVar(&contextJSON, "json", false, "Output in JSON format")
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()

	if len(args) == 0 {
		session := core.ReadSessionState(root)
		if session != nil && len(session.Topics) > 0 {
			args = session.Topics
		}
		if len(args) == 0 {
			return fmt.Errorf("no topic given and no session topics set (use 'mote prime <topic>' first)")
		}
	}

	topic := strings.Join(args, " ")
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	if contextPlanning {
		return runPlanningContext(root, args[0])
	}

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, err := readAllWithGlobal(mm)
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	// Build unified cross-scope edge index
	idx := im.BuildInMemory(motes)

	// Seed selection
	ss := core.NewSeedSelector(motes, idx.TagStats, cfg.Priming.Signals, loadTextSearcher(root))
	ss.SetConceptIndex(core.BuildConceptIndex(idx))
	seeds := ss.SelectSeeds(topic, nil)
	if len(seeds) == 0 {
		fmt.Println("No matching motes found.")
		return nil
	}

	// Matched tags for scoring context
	matchedTags := core.ExtractKeywords(topic)

	// Score and traverse
	scorer := core.NewScoreEngine(cfg.Scoring, idx.ConceptStats)
	traverser := core.NewGraphTraverser(idx, scorer, cfg.Scoring)
	results := traverser.Traverse(seeds, matchedTags, func(id string) (*core.Mote, error) {
		return mm.Read(id)
	})

	if len(results) == 0 {
		fmt.Println("No matching motes found.")
		return nil
	}

	// JSON output mode
	if contextJSON {
		out := ContextOutput{
			Topic:   topic,
			Results: scoredMotesToEntriesFromScored(results),
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		// Batch access updates
		for _, sm := range results {
			_ = mm.AppendAccessBatch(sm.Mote.ID)
		}
		return nil
	}

	// Print results
	fmt.Printf("%-8s  %-24s  %-14s  %s\n", "SCORE", "ID", "TYPE", "TITLE")
	fmt.Println(strings.Repeat("-", 76))
	for _, sm := range results {
		fmt.Printf("%-8.3f  %-24s  %-14s  %s\n",
			sm.Score,
			sm.Mote.ID,
			sm.Mote.Type,
			format.Truncate(sm.Mote.Title, 40))
	}

	// Contradiction detection
	printContradictions(results, idx)

	// Strata augmentation: if anchor motes scored, query their corpora
	if cfg.Strata.ContextAugment.Enabled {
		augmentFromStrata(root, cfg, topic, results)
	}

	// Proactive strata suggestions
	printProactiveStrata(motes, topic)

	// Batch access updates
	for _, sm := range results {
		_ = mm.AppendAccessBatch(sm.Mote.ID)
	}

	return nil
}

func augmentFromStrata(root string, cfg *core.Config, topic string, results []core.ScoredMote) {
	sm := strata.NewStrataManager(root, cfg.Strata)
	augCfg := cfg.Strata.ContextAugment

	var corporaQueried int
	for _, r := range results {
		if corporaQueried >= augCfg.MaxAugmentCorpora {
			break
		}
		corpus := r.Mote.StrataCorpus
		if corpus == "" {
			continue
		}

		topK := augCfg.ChunksPerCorpus
		if topK <= 0 {
			topK = 3
		}
		chunks, err := sm.Query(topic, corpus, topK)
		if err != nil || len(chunks) == 0 {
			continue
		}

		// Filter by min relevance
		minScore := cfg.Strata.Retrieval.MinRelevanceScore
		var relevant []strata.ChunkResult
		for _, c := range chunks {
			if c.Score >= minScore {
				relevant = append(relevant, c)
			}
		}
		if len(relevant) == 0 {
			continue
		}

		fmt.Printf("\n--- Reference (from %s) ---\n", corpus)
		for _, c := range relevant {
			heading := c.Chunk.Heading
			if heading == "" {
				heading = c.Chunk.SourcePath
			}
			text := strings.ReplaceAll(c.Chunk.Text, "\n", " ")
			fmt.Printf("  [%.2f] %s\n", c.Score, heading)
			fmt.Printf("         %s\n", format.Truncate(text, 120))
		}
		corporaQueried++

		// Update anchor query count
		updateAnchorQueryCount(root, corpus)
	}
}

func runPlanningContext(root, moteID string) error {
	mm := core.NewMoteManager(root)

	target, err := mm.Read(moteID)
	if err != nil {
		return fmt.Errorf("read mote %s: %w", moteID, err)
	}

	// Show hierarchy context
	if target.Parent != "" {
		if parent, err := mm.Read(target.Parent); err == nil {
			fmt.Printf("Parent: %s %q [%s]\n", parent.ID, parent.Title, parent.Status)
			if siblings, err := mm.Children(target.Parent); err == nil {
				for _, s := range siblings {
					if s.ID == moteID {
						continue
					}
					fmt.Printf("  Sibling: %s %q [%s]\n", s.ID, s.Title, s.Status)
				}
			}
			fmt.Println()
		}
	}

	// BFS backward through depends_on to find all prerequisites
	// BFS forward through blocks to find all dependents
	visited := map[string]*core.Mote{moteID: target}
	levels := map[string]int{moteID: 0}

	// Walk backwards (depends_on chain)
	queue := []string{moteID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		m := visited[current]
		for _, depID := range m.DependsOn {
			if _, ok := visited[depID]; ok {
				continue
			}
			dep, err := mm.Read(depID)
			if err != nil {
				continue
			}
			visited[depID] = dep
			levels[depID] = levels[current] - 1
			queue = append(queue, depID)
		}
	}

	// Walk forwards (blocks chain)
	queue = []string{moteID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		m := visited[current]
		for _, blockID := range m.Blocks {
			if _, ok := visited[blockID]; ok {
				continue
			}
			dep, err := mm.Read(blockID)
			if err != nil {
				continue
			}
			visited[blockID] = dep
			levels[blockID] = levels[current] + 1
			queue = append(queue, blockID)
		}
	}

	// Normalize levels so minimum is 0
	minLevel := 0
	for _, l := range levels {
		if l < minLevel {
			minLevel = l
		}
	}
	for id := range levels {
		levels[id] -= minLevel
	}

	// Group by level
	maxLevel := 0
	for _, l := range levels {
		if l > maxLevel {
			maxLevel = l
		}
	}

	fmt.Printf("Execution chain for %s %q:\n\n", target.ID, target.Title)
	totalMotes := 0
	maxParallel := 0
	for level := 0; level <= maxLevel; level++ {
		var atLevel []*core.Mote
		for id, l := range levels {
			if l == level {
				atLevel = append(atLevel, visited[id])
			}
		}
		if len(atLevel) > maxParallel {
			maxParallel = len(atLevel)
		}
		for i, m := range atLevel {
			marker := ""
			if m.ID == moteID {
				marker = " <- target"
			} else if len(atLevel) > 1 && i > 0 {
				marker = " <- parallel"
			}
			fmt.Printf("  Level %d: %s %q [%s]%s\n",
				level, m.ID, format.Truncate(m.Title, 40), m.Status, marker)
		}
		totalMotes += len(atLevel)
	}
	fmt.Printf("\n  Chain: %d motes", totalMotes)
	if maxParallel > 1 {
		fmt.Printf(", max %d parallel at a level", maxParallel)
	}
	fmt.Println()

	return nil
}

func printContradictions(results []core.ScoredMote, idx *core.EdgeIndex) {
	resultSet := make(map[string]*core.Mote)
	for _, sm := range results {
		resultSet[sm.Mote.ID] = sm.Mote
	}

	type pair struct{ a, b string }
	seen := make(map[pair]bool)
	var contradictions []pair

	for _, sm := range results {
		if sm.Mote.Status == "deprecated" {
			continue
		}
		for _, cID := range sm.Mote.Contradicts {
			other, ok := resultSet[cID]
			if !ok || other.Status == "deprecated" {
				continue
			}
			p := pair{sm.Mote.ID, cID}
			pr := pair{cID, sm.Mote.ID}
			if !seen[p] && !seen[pr] {
				seen[p] = true
				contradictions = append(contradictions, p)
			}
		}
	}

	if len(contradictions) > 0 {
		fmt.Printf("\nWarning: Contradictions\n")
		for _, c := range contradictions {
			aTitle := resultSet[c.a].Title
			bTitle := resultSet[c.b].Title
			fmt.Printf("  %s (%s) <-> %s (%s)\n", c.a, aTitle, c.b, bTitle)
		}
		fmt.Println("  Consider resolving: deprecate one, or create a superseding mote")
	}
}
