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
	im := core.NewIndexManager(root)
	hadOutput := false

	// Read access batch BEFORE flush to capture session mote IDs
	var sessionMoteIDs []string
	batchPath := filepath.Join(root, ".access_batch.jsonl")
	if batchData, err := os.ReadFile(batchPath); err == nil && len(batchData) > 0 {
		seen := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(string(batchData)), "\n") {
			if line == "" {
				continue
			}
			var entry core.AccessBatchEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			if !seen[entry.MoteID] {
				seen[entry.MoteID] = true
				sessionMoteIDs = append(sessionMoteIDs, entry.MoteID)
			}
		}
	}

	// Flush access batch
	accessCount, moteCount, err := mm.FlushAccessBatchStats()
	if err != nil {
		return fmt.Errorf("flush access batch: %w", err)
	}
	if accessCount > 0 {
		fmt.Printf("Flushed %d access updates to %d motes.\n", accessCount, moteCount)
		hadOutput = true
	}

	// Crystallization suggestions: uncrystallized completed motes
	allMotes, _ := mm.ReadAllParallel()
	if allMotes != nil {
		sourceIssueSet := map[string]bool{}
		for _, m := range allMotes {
			if m.SourceIssue != "" {
				sourceIssueSet[m.SourceIssue] = true
			}
		}
		var uncrystallized []string
		for _, m := range allMotes {
			if (m.Status == "completed" || m.Status == "archived") && !sourceIssueSet[m.ID] {
				uncrystallized = append(uncrystallized, m.ID)
			}
		}
		if len(uncrystallized) > 0 {
			fmt.Println("\nCrystallization candidates:")
			limit := 5
			if len(uncrystallized) < limit {
				limit = len(uncrystallized)
			}
			for _, id := range uncrystallized[:limit] {
				fmt.Printf("  mote crystallize %s\n", id)
			}
			if len(uncrystallized) > 5 {
				fmt.Printf("  ...and %d more\n", len(uncrystallized)-5)
			}
			hadOutput = true
		}
	}

	// Co-access link suggestions: session mote pairs with 2+ shared tags but no existing edge
	if len(sessionMoteIDs) >= 2 && allMotes != nil {
		idx, idxErr := im.Load()
		if idxErr == nil {
			moteMap := map[string]*core.Mote{}
			for _, m := range allMotes {
				moteMap[m.ID] = m
			}

			type suggestion struct {
				a, b       string
				sharedTags []string
			}
			var suggestions []suggestion

			for i := 0; i < len(sessionMoteIDs); i++ {
				mA, okA := moteMap[sessionMoteIDs[i]]
				if !okA {
					continue
				}
				for j := i + 1; j < len(sessionMoteIDs); j++ {
					mB, okB := moteMap[sessionMoteIDs[j]]
					if !okB {
						continue
					}
					// Find shared tags
					bTags := map[string]bool{}
					for _, t := range mB.Tags {
						bTags[t] = true
					}
					var shared []string
					for _, t := range mA.Tags {
						if bTags[t] {
							shared = append(shared, t)
						}
					}
					if len(shared) < 2 {
						continue
					}
					// Check if already linked
					edges := idx.Neighbors(mA.ID, nil)
					alreadyLinked := false
					for _, e := range edges {
						if e.Target == mB.ID {
							alreadyLinked = true
							break
						}
					}
					if !alreadyLinked {
						suggestions = append(suggestions, suggestion{a: mA.ID, b: mB.ID, sharedTags: shared})
					}
				}
			}

			if len(suggestions) > 0 {
				fmt.Println("\nCo-access link suggestions:")
				limit := 3
				if len(suggestions) < limit {
					limit = len(suggestions)
				}
				for _, s := range suggestions[:limit] {
					fmt.Printf("  mote link %s relates_to %s  (shared: %s)\n",
						s.a, s.b, strings.Join(s.sharedTags, ", "))
				}
				hadOutput = true
			}
		}
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
