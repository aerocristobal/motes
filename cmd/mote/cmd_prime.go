package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/dream"
	"motes/internal/format"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Output context priming for the current session",
	RunE:  runPrime,
}

func init() {
	rootCmd.AddCommand(primeCmd)
}

func runPrime(cmd *cobra.Command, args []string) error {
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

	// Merge global motes
	globalMotes := readGlobalMotes()
	motes = append(motes, globalMotes...)

	// Get active tasks sorted by weight
	var activeTasks []*core.Mote
	for _, m := range motes {
		if m.Type == "task" && m.Status == "active" {
			activeTasks = append(activeTasks, m)
		}
	}
	sort.Slice(activeTasks, func(i, j int) bool {
		return activeTasks[i].Weight > activeTasks[j].Weight
	})

	// Fallback: no active tasks → show top 5 by weight
	if len(activeTasks) == 0 {
		fmt.Println("No active tasks. Showing top motes by weight:")
		fmt.Println()
		var active []*core.Mote
		for _, m := range motes {
			if m.Status == "active" {
				active = append(active, m)
			}
		}
		sort.Slice(active, func(i, j int) bool {
			return active[i].Weight > active[j].Weight
		})
		if len(active) > 5 {
			active = active[:5]
		}
		for _, m := range active {
			fmt.Printf("  [%.2f] %s  %s — %s\n", m.Weight, m.ID, m.Type, m.Title)
		}
		return nil
	}

	// Take top 2 active tasks
	if len(activeTasks) > 2 {
		activeTasks = activeTasks[:2]
	}

	// Collect ambient context
	ambient := core.CollectAmbientContext()

	// Build scoring/traversal
	scorer := core.NewScoreEngine(cfg.Scoring, idx.TagStats)
	ss := core.NewSeedSelector(motes, idx.TagStats, cfg.Priming.Signals)

	var allResults []core.ScoredMote
	seen := make(map[string]bool)

	// Print active work section
	fmt.Println("## Active work")
	fmt.Println()
	for _, task := range activeTasks {
		fmt.Printf("  [%.2f] %s — %s\n", task.Weight, task.ID, task.Title)
		if len(task.Tags) > 0 {
			fmt.Printf("         tags: %s\n", format.TagList(task.Tags))
		}

		// Build topic from task tags + title keywords
		topic := task.Title
		if len(task.Tags) > 0 {
			topic += " " + strings.Join(task.Tags, " ")
		}

		seeds := ss.SelectSeeds(topic, ambient)
		matchedTags := core.ExtractKeywords(topic)
		traverser := core.NewGraphTraverser(idx, scorer, cfg.Scoring)
		results := traverser.Traverse(seeds, matchedTags, func(id string) (*core.Mote, error) {
			return mm.Read(id)
		})

		for _, sm := range results {
			if !seen[sm.Mote.ID] {
				seen[sm.Mote.ID] = true
				allResults = append(allResults, sm)
			}
		}
	}
	fmt.Println()

	// Group results by type
	decisions := filterByType(allResults, "decision")
	lessons := filterByType(allResults, "lesson")
	explores := filterByType(allResults, "explore")

	if len(decisions) > 0 {
		fmt.Println("## Relevant decisions")
		fmt.Println()
		for _, sm := range decisions {
			fmt.Printf("  %s[%.3f] %s — %s\n", motePrefix(sm.Mote), sm.Score, sm.Mote.ID, sm.Mote.Title)
		}
		fmt.Println()
	}

	if len(lessons) > 0 {
		fmt.Println("## Key lessons")
		fmt.Println()
		for _, sm := range lessons {
			fmt.Printf("  %s[%.3f] %s — %s\n", motePrefix(sm.Mote), sm.Score, sm.Mote.ID, sm.Mote.Title)
		}
		fmt.Println()
	}

	if len(explores) > 0 {
		fmt.Println("## Prior explorations")
		fmt.Println()
		for _, sm := range explores {
			fmt.Printf("  %s[%.3f] %s — %s\n", motePrefix(sm.Mote), sm.Score, sm.Mote.ID, sm.Mote.Title)
		}
		fmt.Println()
	}

	// Contradiction warnings
	printContradictions(allResults, idx)

	// Dream notices
	dreamDir := filepath.Join(root, "dream")
	printDreamNotices(dreamDir, cfg)

	// Available strata from anchor motes in results
	printStrataSection(allResults)

	// Batch access updates
	for _, sm := range allResults {
		_ = mm.AppendAccessBatch(sm.Mote.ID)
	}

	return nil
}

func printDreamNotices(dreamDir string, cfg *core.Config) {
	// Check last dream run
	logPath := filepath.Join(dreamDir, "log.jsonl")
	if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) > 0 {
			lastLine := lines[len(lines)-1]
			var entry dream.RunLogEntry
			if err := json.Unmarshal([]byte(lastLine), &entry); err == nil && entry.Timestamp != "" {
				if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
					daysSince := int(time.Since(t).Hours() / 24)
					hint := cfg.Dream.ScheduleHintDays
					if hint <= 0 {
						hint = 2
					}
					if daysSince > hint {
						fmt.Printf("## Dream cycle\n\n")
						fmt.Printf("  Last dream run: %d days ago (hint: every %d days)\n", daysSince, hint)
						fmt.Printf("  Consider running: mote dream\n\n")
					}
				}
			}
		}
	}

	// Count pending visions
	vw := dream.NewVisionWriter(dreamDir)
	pending := vw.ReadFinal()
	if len(pending) > 0 {
		fmt.Printf("## Pending visions\n\n")
		fmt.Printf("  %d visions pending review — run: mote dream review\n\n", len(pending))
	}
}

func printStrataSection(results []core.ScoredMote) {
	var anchors []core.ScoredMote
	for _, sm := range results {
		if sm.Mote.Type == "anchor" && sm.Mote.StrataCorpus != "" {
			anchors = append(anchors, sm)
		}
	}
	if len(anchors) == 0 {
		return
	}
	fmt.Println("## Available strata")
	fmt.Println()
	for _, sm := range anchors {
		hint := sm.Mote.StrataQueryHint
		if hint == "" {
			hint = sm.Mote.Title
		}
		fmt.Printf("  %s — corpus: %s (hint: %s)\n", sm.Mote.ID, sm.Mote.StrataCorpus, hint)
	}
	fmt.Println()
}

func filterByType(results []core.ScoredMote, moteType string) []core.ScoredMote {
	var filtered []core.ScoredMote
	for _, sm := range results {
		if sm.Mote.Type == moteType {
			filtered = append(filtered, sm)
		}
	}
	return filtered
}

func readGlobalMotes() []*core.Mote {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	globalDir := filepath.Join(home, ".claude", "memory", "nodes")
	entries, err := os.ReadDir(globalDir)
	if err != nil {
		return nil
	}
	var motes []*core.Mote
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		m, err := core.ParseMote(filepath.Join(globalDir, entry.Name()))
		if err != nil {
			continue
		}
		motes = append(motes, m)
	}
	return motes
}

func isGlobalMote(m *core.Mote) bool {
	return strings.HasPrefix(m.ID, "global-")
}

func motePrefix(m *core.Mote) string {
	if isGlobalMote(m) {
		return "[global] "
	}
	return ""
}
