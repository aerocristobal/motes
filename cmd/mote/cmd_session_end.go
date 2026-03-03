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
	"motes/internal/strata"
)

var sessionEndCmd = &cobra.Command{
	Use:   "session-end",
	Short: "Flush access batch and print session summary",
	RunE:  runSessionEnd,
}

func init() {
	rootCmd.AddCommand(sessionEndCmd)
}

func runSessionEnd(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()

	mm := core.NewMoteManager(root)
	hadOutput := false

	// Flush access batch
	accessCount, moteCount, err := mm.FlushAccessBatchStats()
	if err != nil {
		return fmt.Errorf("flush access batch: %w", err)
	}
	if accessCount > 0 {
		fmt.Printf("Flushed %d access updates to %d motes.\n", accessCount, moteCount)
		hadOutput = true
	}

	// Strata query summary
	queryPath := filepath.Join(root, "strata", "query_log.jsonl")
	data, err := os.ReadFile(queryPath)
	if err == nil && len(data) > 0 {
		today := time.Now().UTC().Format("2006-01-02")
		corpusCounts := map[string]int{}
		totalQueries := 0

		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var entry strata.QueryLogEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			// Filter to today's queries
			if strings.HasPrefix(entry.Timestamp, today) {
				totalQueries++
				corpusCounts[entry.Corpus]++
			}
		}

		if totalQueries > 0 {
			fmt.Printf("Strata queries this session: %d queries across %d corpora\n",
				totalQueries, len(corpusCounts))
			for corpus, count := range corpusCounts {
				if count >= 3 {
					fmt.Printf("  %s queried %d times — consider if key insights should be motes\n",
						corpus, count)
				}
			}
			hadOutput = true
		}
	}

	if !hadOutput {
		// Silent if nothing to do
	}

	return nil
}
