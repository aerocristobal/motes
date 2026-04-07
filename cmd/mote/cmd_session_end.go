// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
	"motes/internal/strata"
)

var sessionEndDryRun bool
var sessionEndNoSummary bool
var sessionEndHook bool

var sessionEndCmd = &cobra.Command{
	Use:   "session-end",
	Short: "Flush access batch and print session summary",
	RunE:  runSessionEnd,
}

func init() {
	rootCmd.AddCommand(sessionEndCmd)
	sessionEndCmd.Flags().BoolVar(&sessionEndDryRun, "dry-run", false, "Print link suggestions without creating them")
	sessionEndCmd.Flags().BoolVar(&sessionEndNoSummary, "no-summary", false, "Skip auto-creating session context mote")
	sessionEndCmd.Flags().BoolVar(&sessionEndHook, "hook", false, "Wrap output in {\"additionalContext\": ...} JSON for hooks")
}

func runSessionEnd(cmd *cobra.Command, args []string) error {
	if sessionEndHook {
		return runSessionEndHook(cmd, args)
	}
	return runSessionEndInner(cmd, args)
}

func runSessionEndHook(cmd *cobra.Command, args []string) error {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	os.Stdout = w

	runErr := runSessionEndInner(cmd, args)

	w.Close()
	os.Stdout = old

	captured, _ := io.ReadAll(r)
	if runErr != nil {
		return runErr
	}

	text := strings.TrimSpace(string(captured))
	if text == "" {
		fmt.Println("{}")
		return nil
	}

	out := struct {
		AdditionalContext string `json:"additionalContext"`
	}{AdditionalContext: text}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
	return nil
}

func runSessionEndInner(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	// Acquire ops lock for the entire session-end operation
	opsLock, err := mm.LockOps()
	if err != nil {
		return fmt.Errorf("acquire ops lock: %w", err)
	}
	defer opsLock.Unlock()

	hadOutput := false

	// Read access batch BEFORE flush to capture session mote IDs and prime hit data
	var sessionMoteIDs []string
	primedSet := map[string]bool{}
	accessedSet := map[string]bool{}
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
			if entry.Primed {
				primedSet[entry.MoteID] = true
			} else {
				accessedSet[entry.MoteID] = true
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

	// Read all motes now — used for nudge check, crystallization, and co-access sections
	allMotes, _ := mm.ReadAllParallel()

	// Read prime history for nudge throttle check
	primeHistory, _ := mm.ReadPrimeSessionStats(0)

	// Lingering task nudge: completed tasks > 7 days old with no crystallized_at
	nudgeCount := countLingeringTasks(allMotes)
	showNudge := nudgeCount >= 3 && !lastSessionHadNudge(primeHistory)

	// Record prime hit-rate stats when prime was used this session, or when nudge fires
	if len(primedSet) > 0 || showNudge {
		var primedIDs, hitIDs []string
		for id := range primedSet {
			primedIDs = append(primedIDs, id)
		}
		for id := range primedSet {
			if accessedSet[id] {
				hitIDs = append(hitIDs, id)
			}
		}
		hitRate := 0.0
		if len(primedIDs) > 0 {
			hitRate = float64(len(hitIDs)) / float64(len(primedIDs))
		}
		stats := core.PrimeSessionStats{
			SessionAt:      time.Now().UTC().Format(time.RFC3339),
			PrimedCount:    len(primedIDs),
			HitCount:       len(hitIDs),
			HitRate:        hitRate,
			PrimedIDs:      primedIDs,
			HitIDs:         hitIDs,
			LingeringNudge: showNudge,
		}
		if writeErr := mm.WritePrimeSessionStats(stats); writeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: write prime stats: %v\n", writeErr)
		}
		if len(primedSet) > 0 {
			fmt.Printf("Prime hit rate: %d/%d (%.0f%%)\n", len(hitIDs), len(primedIDs), hitRate*100)
			hadOutput = true
		}
	}

	if showNudge {
		fmt.Printf("📦 %d completed tasks not yet crystallized. `mote crystallize --candidates`\n", nudgeCount)
		hadOutput = true
	}

	// Auto-ingest changed files into _codebase corpus
	ingested, ingestErr := autoIngestChangedFiles(root)
	if ingestErr == nil && ingested > 0 {
		fmt.Printf("Auto-ingested %d changed files into _codebase corpus.\n", ingested)
		hadOutput = true
	}

	// Auto-crystallize session summary
	if !sessionEndNoSummary && len(sessionMoteIDs) >= 3 {
		// Build session summary body
		var bodyLines []string
		bodyLines = append(bodyLines, "Accessed motes:")
		for _, id := range sessionMoteIDs {
			bodyLines = append(bodyLines, fmt.Sprintf("- %s", id))
		}

		title := fmt.Sprintf("Session: %s", time.Now().UTC().Format("2006-01-02"))
		m, createErr := mm.Create("context", title, core.CreateOpts{
			Weight: 0.3,
			Tags:   []string{"session"},
			Body:   strings.Join(bodyLines, "\n"),
		})
		if createErr == nil {
			// Link session mote to accessed motes via informed_by
			simm := core.NewIndexManager(root)
			if _, loadErr := simm.Load(); loadErr == nil {
				for _, id := range sessionMoteIDs {
					_ = mm.Link(m.ID, "informed_by", id, simm)
				}
			}
			fmt.Printf("Created session summary: %s\n", m.ID)
			hadOutput = true
		}
	}

	// Crystallization suggestions: uncrystallized completed motes
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
				limit := 3
				if len(suggestions) < limit {
					limit = len(suggestions)
				}
				if sessionEndDryRun {
					fmt.Println("\nCo-access link suggestions:")
					for _, s := range suggestions[:limit] {
						fmt.Printf("  mote link %s relates_to %s  (shared: %s)\n",
							s.a, s.b, strings.Join(s.sharedTags, ", "))
					}
				} else {
					fmt.Println("\nAuto-linked co-accessed motes:")
					for _, s := range suggestions[:limit] {
						if err := mm.Link(s.a, "relates_to", s.b, im); err != nil {
							fmt.Fprintf(os.Stderr, "  warning: failed to link %s <-> %s: %v\n", s.a, s.b, err)
							continue
						}
						fmt.Printf("  %s <-> %s  (shared: %s)\n",
							s.a, s.b, strings.Join(s.sharedTags, ", "))
					}
				}
				hadOutput = true
			}
		}
	}

	// Concept auto-enrichment: suggest wiki-links for under-linked motes accessed this session
	if len(sessionMoteIDs) > 0 && allMotes != nil {
		bm25Idx, _ := loadMoteBM25(root)
		if bm25Idx != nil && bm25Idx.DocCount > 0 {
			moteMap := map[string]*core.Mote{}
			for _, m := range allMotes {
				moteMap[m.ID] = m
			}

			var enriched int
			for _, id := range sessionMoteIDs {
				m, ok := moteMap[id]
				if !ok || m.Status != "active" {
					continue
				}
				if core.CountConcepts(m) >= 2 {
					continue
				}
				terms := bm25Idx.DistinctiveTerms(id, 3)
				if len(terms) == 0 {
					continue
				}
				// Filter: valid tag chars, not already a tag or wiki-link
				existingTags := map[string]bool{}
				for _, tag := range m.Tags {
					existingTags[tag] = true
				}
				existingLinks := map[string]bool{}
				for _, link := range core.ExtractBodyLinks(m.Body, m.ID) {
					existingLinks[link] = true
				}
				var validTerms []string
				for _, term := range terms {
					if existingTags[term] || existingLinks[term] {
						continue
					}
					if security.ValidateTag(term) != nil {
						continue
					}
					validTerms = append(validTerms, term)
				}
				if len(validTerms) == 0 {
					continue
				}

				if sessionEndDryRun {
					wikiLinks := ""
					for _, t := range validTerms {
						wikiLinks += " [[" + t + "]]"
					}
					fmt.Printf("  Enrich %s:%s\n", id, wikiLinks)
				} else {
					wikiLinks := "\n\n"
					for i, t := range validTerms {
						if i > 0 {
							wikiLinks += " "
						}
						wikiLinks += "[[" + t + "]]"
					}
					m.Body += wikiLinks
					data, err := core.SerializeMote(m)
					if err == nil {
						_ = os.WriteFile(m.FilePath, data, 0644)
					}
				}
				enriched++
			}

			if enriched > 0 {
				if sessionEndDryRun {
					fmt.Printf("\nConcept enrichment suggestions: %d motes\n", enriched)
				} else {
					fmt.Printf("\nEnriched %d motes with concept wiki-links\n", enriched)
					// Rebuild index to update concept_ref edges
					rebuildMotes, _ := mm.ReadAllParallel()
					if rebuildMotes != nil {
						_ = im.Rebuild(rebuildMotes)
					}
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

	// Knowledge type summary: reinforce that decisions/lessons are high-value
	if allMotes != nil {
		today := time.Now().UTC().Format("2006-01-02")
		typeCounts := map[string]int{}
		for _, m := range allMotes {
			if !m.CreatedAt.IsZero() && strings.HasPrefix(m.CreatedAt.UTC().Format("2006-01-02"), today) {
				typeCounts[m.Type]++
			}
		}
		total := 0
		for _, c := range typeCounts {
			total += c
		}
		if total > 0 {
			var parts []string
			for _, t := range []string{"task", "decision", "lesson", "context", "explore", "question"} {
				if c, ok := typeCounts[t]; ok {
					parts = append(parts, fmt.Sprintf("%d %ss", c, t))
				}
			}
			fmt.Printf("\nSession captured: %s\n", strings.Join(parts, ", "))
			knowledge := typeCounts["decision"] + typeCounts["lesson"] + typeCounts["explore"]
			tasks := typeCounts["task"]
			if tasks > 0 && knowledge == 0 && tasks >= 3 {
				fmt.Println("Tip: decisions and lessons age better than tasks.")
			}
			hadOutput = true
		}
	}

	if !hadOutput {
		// Silent if nothing to do
	}

	core.ClearSessionState(root)

	return nil
}

// autoIngestChangedFiles ingests git-changed code files into the _codebase strata corpus.
func autoIngestChangedFiles(root string) (int, error) {
	ambient := core.CollectAmbientContext()
	if len(ambient.RecentFiles) == 0 {
		return 0, nil
	}

	var codeFiles []string
	for _, f := range ambient.RecentFiles {
		if strata.IsCodeFile(f) {
			codeFiles = append(codeFiles, f)
		}
	}
	if len(codeFiles) == 0 {
		return 0, nil
	}

	// Resolve to absolute paths for strata ingestion
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	var absPaths []string
	for _, f := range codeFiles {
		abs := f
		if !filepath.IsAbs(f) {
			abs = filepath.Join(cwd, f)
		}
		if _, err := os.Stat(abs); err == nil {
			absPaths = append(absPaths, abs)
		}
	}
	if len(absPaths) == 0 {
		return 0, nil
	}

	cfg, err := core.LoadConfig(root)
	if err != nil {
		return 0, err
	}

	sm := strata.NewStrataManager(root, cfg.Strata)
	return sm.EnsureCorpus("_codebase", absPaths)
}

// countLingeringTasks counts completed task motes older than 7 days with no crystallized_at.
func countLingeringTasks(motes []*core.Mote) int {
	threshold := time.Now().AddDate(0, 0, -7)
	count := 0
	for _, m := range motes {
		if m.Type != "task" || m.Status != "completed" || m.CrystallizedAt != nil {
			continue
		}
		if m.CreatedAt.Before(threshold) {
			count++
		}
	}
	return count
}

// lastSessionHadNudge returns true if the most recent prime session had LingeringNudge=true.
func lastSessionHadNudge(stats []core.PrimeSessionStats) bool {
	if len(stats) == 0 {
		return false
	}
	return stats[len(stats)-1].LingeringNudge
}
