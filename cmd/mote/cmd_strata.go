package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/strata"
)

var strataCmd = &cobra.Command{
	Use:   "strata",
	Short: "Manage strata reference knowledge corpora",
}

var strataAddCmd = &cobra.Command{
	Use:   "add <path> --corpus=<name>",
	Short: "Ingest reference material into a strata corpus",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runStrataAdd,
}

var strataQueryCmd = &cobra.Command{
	Use:   "query <topic> [--corpus=<name>]",
	Short: "Search strata corpora for relevant chunks",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runStrataQuery,
}

var strataLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List available strata corpora",
	RunE:  runStrataLs,
}

var strataRmCmd = &cobra.Command{
	Use:   "rm <corpus-name>",
	Short: "Remove a strata corpus",
	Args:  cobra.ExactArgs(1),
	RunE:  runStrataRm,
}

var strataUpdateCmd = &cobra.Command{
	Use:   "update <corpus-name>",
	Short: "Re-ingest changed files for an existing corpus",
	Args:  cobra.ExactArgs(1),
	RunE:  runStrataUpdate,
}

var strataRebuildCmd = &cobra.Command{
	Use:   "rebuild <corpus-name>",
	Short: "Fully rebuild a corpus from its source paths",
	Args:  cobra.ExactArgs(1),
	RunE:  runStrataRebuild,
}

var strataStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show corpus statistics and query activity",
	RunE:  runStrataStats,
}

var (
	strataCorpus   string
	strataNoAnchor bool
	strataTopK     int
)

func init() {
	strataAddCmd.Flags().StringVar(&strataCorpus, "corpus", "", "Corpus name (required)")
	strataAddCmd.MarkFlagRequired("corpus")
	strataAddCmd.Flags().BoolVar(&strataNoAnchor, "no-anchor", false, "Don't create an anchor mote")

	strataQueryCmd.Flags().StringVar(&strataCorpus, "corpus", "", "Search a specific corpus")
	strataQueryCmd.Flags().IntVar(&strataTopK, "top-k", 0, "Number of results (default from config)")

	strataCmd.AddCommand(strataAddCmd)
	strataCmd.AddCommand(strataQueryCmd)
	strataCmd.AddCommand(strataLsCmd)
	strataCmd.AddCommand(strataRmCmd)
	strataCmd.AddCommand(strataUpdateCmd)
	strataCmd.AddCommand(strataRebuildCmd)
	strataCmd.AddCommand(strataStatsCmd)
	rootCmd.AddCommand(strataCmd)
}

func runStrataAdd(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	sm := strata.NewStrataManager(root, cfg.Strata)
	var mm *core.MoteManager
	if !strataNoAnchor {
		mm = core.NewMoteManager(root)
	}

	if err := sm.AddCorpus(strataCorpus, args, !strataNoAnchor, mm); err != nil {
		return err
	}

	corpora, _ := sm.ListCorpora()
	for _, c := range corpora {
		if c.Manifest.Name == strataCorpus {
			fmt.Printf("Corpus '%s': %d chunks from %d sources\n",
				strataCorpus, c.Manifest.ChunkCount, len(c.Manifest.SourcePaths))
			break
		}
	}
	return nil
}

func runStrataQuery(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	sm := strata.NewStrataManager(root, cfg.Strata)
	topic := strings.Join(args, " ")
	topK := strataTopK
	if topK <= 0 {
		topK = cfg.Strata.Retrieval.DefaultTopK
	}

	var results []strata.ChunkResult
	if strataCorpus != "" {
		results, err = sm.Query(topic, strataCorpus, topK)
	} else {
		results, err = sm.QueryAll(topic, topK)
	}
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("No matching chunks found.")
		return nil
	}

	// Update anchor mote query count
	if strataCorpus != "" {
		updateAnchorQueryCount(root, strataCorpus)
	}

	fmt.Printf("%-8s  %-30s  %-16s  %s\n", "SCORE", "CHUNK", "SOURCE", "HEADING")
	fmt.Println(strings.Repeat("-", 90))
	for _, r := range results {
		source := format.Truncate(r.Chunk.SourcePath, 16)
		heading := format.Truncate(r.Chunk.Heading, 20)
		fmt.Printf("%-8.3f  %-30s  %-16s  %s\n",
			r.Score, r.Chunk.ID, source, heading)
		// Print first 120 chars of text
		text := strings.ReplaceAll(r.Chunk.Text, "\n", " ")
		fmt.Printf("          %s\n", format.Truncate(text, 120))
	}
	return nil
}

func runStrataLs(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	sm := strata.NewStrataManager(root, cfg.Strata)
	corpora, err := sm.ListCorpora()
	if err != nil {
		return err
	}

	if len(corpora) == 0 {
		fmt.Println("No strata corpora found.")
		return nil
	}

	fmt.Printf("%-20s  %-8s  %-10s  %s\n", "NAME", "CHUNKS", "UPDATED", "SOURCES")
	fmt.Println(strings.Repeat("-", 70))
	for _, c := range corpora {
		updated := c.Manifest.LastUpdated
		if t, err := time.Parse(time.RFC3339, updated); err == nil {
			updated = t.Format("2006-01-02")
		}
		sources := fmt.Sprintf("%d files", len(c.Manifest.SourcePaths))
		fmt.Printf("%-20s  %-8d  %-10s  %s\n",
			c.Manifest.Name, c.Manifest.ChunkCount, updated, sources)
	}
	return nil
}

func runStrataRm(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	corpusName := args[0]

	// Deprecate anchor mote if one exists
	mm := core.NewMoteManager(root)
	motes, _ := mm.ReadAllParallel()
	for _, m := range motes {
		if m.StrataCorpus == corpusName {
			m.Status = "deprecated"
			m.Body += "\n\nStrata corpus removed."
			data, err := core.SerializeMote(m)
			if err == nil {
				_ = core.AtomicWrite(m.FilePath, data, 0644)
				fmt.Printf("Deprecated anchor mote %s\n", m.ID)
			}
			break
		}
	}

	sm := strata.NewStrataManager(root, cfg.Strata)
	if err := sm.RemoveCorpus(corpusName); err != nil {
		return err
	}
	fmt.Printf("Removed corpus '%s'\n", corpusName)
	return nil
}

func runStrataUpdate(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	sm := strata.NewStrataManager(root, cfg.Strata)
	changed, err := sm.UpdateCorpus(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Updated corpus '%s': %d files re-ingested\n", args[0], changed)
	return nil
}

func runStrataRebuild(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	sm := strata.NewStrataManager(root, cfg.Strata)

	// Load manifest for source paths before removing
	corpora, err := sm.ListCorpora()
	if err != nil {
		return err
	}
	var sourcePaths []string
	for _, c := range corpora {
		if c.Manifest.Name == args[0] {
			sourcePaths = c.Manifest.SourcePaths
			break
		}
	}
	if len(sourcePaths) == 0 {
		return fmt.Errorf("corpus %q not found or has no source paths", args[0])
	}

	// Remove and re-add without anchor
	if err := sm.RemoveCorpus(args[0]); err != nil {
		return err
	}
	if err := sm.AddCorpus(args[0], sourcePaths, false, nil); err != nil {
		return err
	}

	// Report
	corpora, _ = sm.ListCorpora()
	for _, c := range corpora {
		if c.Manifest.Name == args[0] {
			fmt.Printf("Rebuilt corpus '%s': %d chunks from %d sources\n",
				args[0], c.Manifest.ChunkCount, len(c.Manifest.SourcePaths))
			break
		}
	}
	return nil
}

func runStrataStats(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	sm := strata.NewStrataManager(root, cfg.Strata)
	corpora, err := sm.ListCorpora()
	if err != nil {
		return err
	}

	if len(corpora) == 0 {
		fmt.Println("No strata corpora found.")
		return nil
	}

	// Load query log
	queryPath := filepath.Join(root, "strata", "query_log.jsonl")
	type queryEntry struct {
		Corpus string `json:"corpus"`
		Query  string `json:"query"`
	}
	corpusQueryCounts := map[string]int{}
	corpusTopics := map[string]map[string]int{}
	if data, err := os.ReadFile(queryPath); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var e queryEntry
			if err := json.Unmarshal([]byte(line), &e); err == nil {
				corpusQueryCounts[e.Corpus]++
				if corpusTopics[e.Corpus] == nil {
					corpusTopics[e.Corpus] = map[string]int{}
				}
				corpusTopics[e.Corpus][e.Query]++
			}
		}
	}

	// Find anchor motes
	mm := core.NewMoteManager(root)
	motes, _ := mm.ReadAllParallel()
	anchorMap := map[string]string{} // corpus → mote ID
	for _, m := range motes {
		if m.StrataCorpus != "" {
			anchorMap[m.StrataCorpus] = m.ID
		}
	}

	for _, c := range corpora {
		name := c.Manifest.Name
		updated := c.Manifest.LastUpdated
		if t, err := time.Parse(time.RFC3339, updated); err == nil {
			updated = t.Format("2006-01-02")
		}

		fmt.Printf("Corpus: %s\n", name)
		fmt.Printf("  Chunks: %d  Sources: %d  Updated: %s\n",
			c.Manifest.ChunkCount, len(c.Manifest.SourcePaths), updated)

		if anchor, ok := anchorMap[name]; ok {
			fmt.Printf("  Anchor: %s\n", anchor)
		}

		qc := corpusQueryCounts[name]
		fmt.Printf("  Queries: %d total\n", qc)

		// Top 3 topics
		if topics, ok := corpusTopics[name]; ok && len(topics) > 0 {
			type topicCount struct {
				topic string
				count int
			}
			var sorted []topicCount
			for t, c := range topics {
				sorted = append(sorted, topicCount{t, c})
			}
			for i := range sorted {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[j].count > sorted[i].count {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			limit := 3
			if len(sorted) < limit {
				limit = len(sorted)
			}
			fmt.Printf("  Top topics:")
			for _, tc := range sorted[:limit] {
				fmt.Printf(" %s(%d)", format.Truncate(tc.topic, 20), tc.count)
			}
			fmt.Println()
		}
		fmt.Println()
	}
	return nil
}

func updateAnchorQueryCount(root, corpus string) {
	mm := core.NewMoteManager(root)
	motes, err := mm.ReadAllParallel()
	if err != nil {
		return
	}
	for _, m := range motes {
		if m.StrataCorpus == corpus {
			now := time.Now().UTC()
			m.StrataQueryCount++
			m.StrataLastQueried = &now
			data, err := core.SerializeMote(m)
			if err != nil {
				return
			}
			_ = core.AtomicWrite(m.FilePath, data, 0644)
			return
		}
	}
}
