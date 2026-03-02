package main

import (
	"fmt"
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

	sm := strata.NewStrataManager(root, cfg.Strata)
	if err := sm.RemoveCorpus(args[0]); err != nil {
		return err
	}
	fmt.Printf("Removed corpus '%s'\n", args[0])
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
