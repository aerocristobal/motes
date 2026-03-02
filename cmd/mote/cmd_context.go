package main

import (
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
	Args:  cobra.MinimumNArgs(1),
	RunE:  runContext,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
	topic := strings.Join(args, " ")

	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	motes, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	// Seed selection
	ss := core.NewSeedSelector(motes, idx.TagStats, cfg.Priming.Signals)
	seeds := ss.SelectSeeds(topic, nil)
	if len(seeds) == 0 {
		fmt.Println("No matching motes found.")
		return nil
	}

	// Matched tags for scoring context
	matchedTags := core.ExtractKeywords(topic)

	// Score and traverse
	scorer := core.NewScoreEngine(cfg.Scoring, idx.TagStats)
	traverser := core.NewGraphTraverser(idx, scorer, cfg.Scoring)
	results := traverser.Traverse(seeds, matchedTags, func(id string) (*core.Mote, error) {
		return mm.Read(id)
	})

	if len(results) == 0 {
		fmt.Println("No matching motes found.")
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
