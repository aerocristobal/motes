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

var primeJSON bool

// PrimeOutput is the JSON output structure for mote prime --json.
type PrimeOutput struct {
	ActiveTasks []MoteEntry   `json:"active_tasks"`
	Decisions   []MoteEntry   `json:"decisions"`
	Lessons     []MoteEntry   `json:"lessons"`
	Explores    []MoteEntry   `json:"explores"`
	Strata      []StrataEntry `json:"strata,omitempty"`
}

// MoteEntry represents a single mote in JSON output.
type MoteEntry struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Score   float64  `json:"score"`
	Tags    []string `json:"tags"`
	Snippet string   `json:"snippet"`
	Global  bool     `json:"global,omitempty"`
}

// StrataEntry represents a strata anchor in JSON output.
type StrataEntry struct {
	ID     string `json:"id"`
	Corpus string `json:"corpus"`
	Hint   string `json:"hint"`
}

var primeCmd = &cobra.Command{
	Use:   "prime [topic...]",
	Short: "Output context priming for the current session",
	RunE:  runPrime,
}

func init() {
	primeCmd.Flags().BoolVar(&primeJSON, "json", false, "Output in JSON format")
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

	// Read session state
	session := core.ReadSessionState(root)

	// Write/update session state on prime
	if session == nil {
		session = &core.SessionState{
			StartTime: time.Now().UTC().Format(time.RFC3339),
		}
	}
	if len(args) > 0 {
		session.Topics = args
	}
	_ = core.WriteSessionState(root, session)

	// Use session topics as fallback if no args given
	if len(args) == 0 && session != nil && len(session.Topics) > 0 {
		args = session.Topics
	}

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

		if len(args) == 0 {
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
	}
	fmt.Println()

	// If topic args given, use them for seed selection instead of task-based
	if len(args) > 0 {
		topic := strings.Join(args, " ")
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

	// Group results by type
	decisions := filterByType(allResults, "decision")
	lessons := filterByType(allResults, "lesson")
	explores := filterByType(allResults, "explore")

	// JSON output mode
	if primeJSON {
		out := PrimeOutput{
			ActiveTasks: scoredMotesToEntries(activeTasks, nil),
			Decisions:   scoredMotesToEntriesFromScored(decisions),
			Lessons:     scoredMotesToEntriesFromScored(lessons),
			Explores:    scoredMotesToEntriesFromScored(explores),
		}
		for _, sm := range allResults {
			if sm.Mote.Type == "anchor" && sm.Mote.StrataCorpus != "" {
				hint := sm.Mote.StrataQueryHint
				if hint == "" {
					hint = sm.Mote.Title
				}
				out.Strata = append(out.Strata, StrataEntry{
					ID:     sm.Mote.ID,
					Corpus: sm.Mote.StrataCorpus,
					Hint:   hint,
				})
			}
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		// Batch access updates
		for _, sm := range allResults {
			_ = mm.AppendAccessBatch(sm.Mote.ID)
		}
		return nil
	}

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

	// Proactive strata suggestions
	var effectiveTopic string
	if len(args) > 0 {
		effectiveTopic = strings.Join(args, " ")
	} else if len(activeTasks) > 0 {
		effectiveTopic = activeTasks[0].Title
		if len(activeTasks[0].Tags) > 0 {
			effectiveTopic += " " + strings.Join(activeTasks[0].Tags, " ")
		}
	}
	printProactiveStrata(motes, effectiveTopic)

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

func printProactiveStrata(motes []*core.Mote, topic string) {
	if topic == "" {
		return
	}
	topicKeywords := core.ExtractKeywords(topic)
	if len(topicKeywords) == 0 {
		return
	}

	type suggestion struct {
		id      string
		corpus  string
		hint    string
		overlap int
	}
	var suggestions []suggestion

	for _, m := range motes {
		if m.Type != "anchor" || m.StrataCorpus == "" || m.StrataQueryHint == "" {
			continue
		}
		hintKeywords := core.ExtractKeywords(m.StrataQueryHint)
		overlap := 0
		for _, tk := range topicKeywords {
			for _, hk := range hintKeywords {
				if strings.EqualFold(tk, hk) {
					overlap++
					break
				}
			}
		}
		if overlap >= 2 {
			suggestions = append(suggestions, suggestion{
				id: m.ID, corpus: m.StrataCorpus, hint: m.StrataQueryHint, overlap: overlap,
			})
		}
	}

	if len(suggestions) == 0 {
		return
	}

	fmt.Println("## Suggested strata queries")
	fmt.Println()
	for _, s := range suggestions {
		fmt.Printf("  mote strata query %s %q  (overlap: %d keywords)\n",
			s.corpus, s.hint, s.overlap)
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

func moteToEntry(m *core.Mote, score float64) MoteEntry {
	return MoteEntry{
		ID:      m.ID,
		Title:   m.Title,
		Score:   score,
		Tags:    m.Tags,
		Snippet: format.Truncate(m.Body, 200),
		Global:  isGlobalMote(m),
	}
}

func scoredMotesToEntries(motes []*core.Mote, scores map[string]float64) []MoteEntry {
	entries := make([]MoteEntry, 0, len(motes))
	for _, m := range motes {
		score := m.Weight
		if scores != nil {
			if s, ok := scores[m.ID]; ok {
				score = s
			}
		}
		entries = append(entries, moteToEntry(m, score))
	}
	return entries
}

func scoredMotesToEntriesFromScored(scored []core.ScoredMote) []MoteEntry {
	entries := make([]MoteEntry, 0, len(scored))
	for _, sm := range scored {
		entries = append(entries, moteToEntry(sm.Mote, sm.Score))
	}
	return entries
}
