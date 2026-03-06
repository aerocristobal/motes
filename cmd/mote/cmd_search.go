package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/strata"
)

var searchTopK int

var searchCmd = &cobra.Command{
	Use:   "search <query...>",
	Short: "Full-text search across all motes",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSearch,
}

func init() {
	searchCmd.Flags().IntVarP(&searchTopK, "top", "k", 10, "Number of results to return")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	motes, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	if len(motes) == 0 {
		fmt.Println("No motes found.")
		return nil
	}

	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	// Try persistent BM25 index first
	persistentIdx, loadErr := loadMoteBM25(root)

	var results []strata.ChunkResult
	if persistentIdx != nil && loadErr == nil {
		results = persistentIdx.Search(query, searchTopK)
	} else {
		// Fall back to ephemeral index
		chunks := make([]strata.Chunk, len(motes))
		for i, m := range motes {
			chunks[i] = strata.Chunk{
				ID:   m.ID,
				Text: m.Title + " " + m.Body,
			}
		}
		idx := strata.BuildBM25Index(chunks)
		results = idx.Search(query, searchTopK)
	}

	if len(results) == 0 {
		fmt.Println("No matching motes found.")
		return nil
	}

	fmt.Printf("%-8s  %-24s  %-14s  %s\n", "SCORE", "ID", "TYPE", "TITLE")
	fmt.Println(strings.Repeat("-", 76))
	for _, r := range results {
		m := moteMap[r.Chunk.ID]
		if m == nil {
			continue
		}
		fmt.Printf("%-8.3f  %-24s  %-14s  %s\n",
			r.Score,
			m.ID,
			m.Type,
			format.Truncate(m.Title, 40))
	}

	return nil
}
