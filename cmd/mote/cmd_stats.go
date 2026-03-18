package main

import (
	"bufio"
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
	"motes/internal/strata"
)

// StatsOutput is the JSON output structure for mote stats --json.
type StatsOutput struct {
	TotalMotes         int            `json:"total_motes"`
	StatusCounts       map[string]int `json:"status_counts"`
	Accessed7          int            `json:"accessed_7d"`
	Accessed30         int            `json:"accessed_30d"`
	Accessed90         int            `json:"accessed_90d"`
	NeverAccessed      int            `json:"never_accessed"`
	TotalTags          int            `json:"total_tags"`
	OverloadedTags     int            `json:"overloaded_tags"`
	SingletonTags      int            `json:"singleton_tags"`
	Contradictions     int            `json:"contradictions"`
	PendingVisions     int            `json:"pending_visions"`
	DreamRuns          int            `json:"dream_runs,omitempty"`
	DreamInputTokens   int            `json:"dream_input_tokens,omitempty"`
	DreamOutputTokens  int            `json:"dream_output_tokens,omitempty"`
	DreamEstimatedCost float64        `json:"dream_estimated_cost,omitempty"`
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show retrieval health dashboard",
	RunE:  runStats,
}

var (
	statsDecayPreview bool
	statsJSON         bool
)

func init() {
	statsCmd.Flags().BoolVar(&statsDecayPreview, "decay-preview", false, "Show motes at risk of recency decay")
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output in JSON format")
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) error {
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

	if statsDecayPreview {
		return showDecayPreview(motes, cfg)
	}

	// Status counts
	statusCounts := map[string]int{}
	for _, m := range motes {
		statusCounts[m.Status]++
	}

	// Access distribution (computed for both JSON and human output)
	now := time.Now()
	var accessed7, accessed30, accessed90, neverAccessed int
	for _, m := range motes {
		if m.LastAccessed == nil {
			neverAccessed++
			continue
		}
		days := now.Sub(*m.LastAccessed).Hours() / 24
		if days <= 7 {
			accessed7++
		}
		if days <= 30 {
			accessed30++
		}
		if days <= 90 {
			accessed90++
		}
	}

	tagOverloadThreshold := cfg.Dream.PreScan.TagOverloadThreshold
	if tagOverloadThreshold <= 0 {
		tagOverloadThreshold = 15
	}
	overloaded := 0
	singletons := 0
	for _, count := range idx.TagStats {
		if count > tagOverloadThreshold {
			overloaded++
		}
		if count == 1 {
			singletons++
		}
	}

	contradictions := countActiveContradictions(motes)

	pendingVisions := 0
	dreamDir := filepath.Join(root, "dream")
	if data, err := os.ReadFile(filepath.Join(dreamDir, "visions.jsonl")); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if l != "" {
				pendingVisions++
			}
		}
	}

	// Read cumulative dream cost data
	dreamCost := readDreamCostStats(dreamDir)

	if statsJSON {
		out := StatsOutput{
			TotalMotes:         len(motes),
			StatusCounts:       statusCounts,
			Accessed7:          accessed7,
			Accessed30:         accessed30,
			Accessed90:         accessed90,
			NeverAccessed:      neverAccessed,
			TotalTags:          len(idx.TagStats),
			OverloadedTags:     overloaded,
			SingletonTags:      singletons,
			Contradictions:     contradictions,
			PendingVisions:     pendingVisions,
			DreamRuns:          dreamCost.runs,
			DreamInputTokens:   dreamCost.inputTokens,
			DreamOutputTokens:  dreamCost.outputTokens,
			DreamEstimatedCost: dreamCost.estimatedCost,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println(format.Header("Nebula Stats"))
	fmt.Println()
	fmt.Printf("  Total motes:  %d\n", len(motes))
	for _, s := range []string{"active", "deprecated", "archived", "completed"} {
		if c := statusCounts[s]; c > 0 {
			fmt.Printf("    %-12s %d\n", s+":", c)
		}
	}

	// Recency stats for human output
	var recencySum float64
	var activeCount int
	for _, m := range motes {
		if m.Status == "active" {
			recencySum += recencyFactor(m.LastAccessed, cfg.Scoring)
			activeCount++
		}
	}

	fmt.Println()
	fmt.Println(format.Header("Access Distribution"))
	fmt.Println()
	fmt.Printf("  Last 7 days:   %d\n", accessed7)
	fmt.Printf("  Last 30 days:  %d\n", accessed30)
	fmt.Printf("  Last 90 days:  %d\n", accessed90)
	fmt.Printf("  Never accessed: %d\n", neverAccessed)

	if activeCount > 0 {
		fmt.Printf("  Avg recency (active): %.2f\n", recencySum/float64(activeCount))
	}

	// Top 5 most accessed
	sort.Slice(motes, func(i, j int) bool {
		return motes[i].AccessCount > motes[j].AccessCount
	})
	fmt.Println()
	fmt.Println(format.Header("Top Accessed"))
	fmt.Println()
	limit := 5
	if len(motes) < limit {
		limit = len(motes)
	}
	for i := 0; i < limit; i++ {
		m := motes[i]
		if m.AccessCount == 0 {
			break
		}
		fmt.Printf("  %-24s  %-4d  %s\n", m.ID, m.AccessCount, format.Truncate(m.Title, 40))
	}

	fmt.Println()
	fmt.Println(format.Header("Tag Health"))
	fmt.Println()
	fmt.Printf("  Total tags:      %d\n", len(idx.TagStats))
	fmt.Printf("  Overloaded (>%d): %d\n", tagOverloadThreshold, overloaded)
	fmt.Printf("  Singletons (=1): %d\n", singletons)
	fmt.Printf("  Active contradictions: %d\n", contradictions)

	// Strata info
	sm := strata.NewStrataManager(root, cfg.Strata)
	corpora, _ := sm.ListCorpora()
	if len(corpora) > 0 {
		totalChunks := 0
		for _, c := range corpora {
			totalChunks += c.Manifest.ChunkCount
		}
		fmt.Println()
		fmt.Println(format.Header("Strata"))
		fmt.Println()
		fmt.Printf("  Corpora: %d (%d total chunks)\n", len(corpora), totalChunks)
	}

	// Dream status
	fmt.Println()
	fmt.Println(format.Header("Dream Cycle"))
	fmt.Println()
	if _, err := os.Stat(dreamDir); os.IsNotExist(err) {
		fmt.Println("  Status: N/A (no dream directory)")
	} else {
		fmt.Printf("  Pending visions: %d\n", pendingVisions)
	}

	// Cumulative dream cost
	if dreamCost.runs > 0 {
		fmt.Println()
		fmt.Println(format.Header("Cumulative Dream Cost"))
		fmt.Println()
		fmt.Printf("  Total runs:      %d\n", dreamCost.runs)
		fmt.Printf("  Input tokens:    %d\n", dreamCost.inputTokens)
		fmt.Printf("  Output tokens:   %d\n", dreamCost.outputTokens)
		fmt.Printf("  Estimated cost:  $%.4f\n", dreamCost.estimatedCost)
	}

	return nil
}

func showDecayPreview(motes []*core.Mote, cfg *core.Config) error {
	fmt.Println(format.Header("Decay Preview"))
	fmt.Println()
	fmt.Printf("%-24s  %-6s  %-8s  %s\n", "ID", "DAYS", "RECENCY", "TITLE")
	fmt.Println(strings.Repeat("-", 76))

	found := false
	for _, m := range motes {
		if m.Status != "active" {
			continue
		}
		rf := recencyFactor(m.LastAccessed, cfg.Scoring)
		// Show motes that have lost >= 30% (recency factor <= 0.7)
		if rf <= 0.7 {
			days := "never"
			if m.LastAccessed != nil {
				d := int(time.Since(*m.LastAccessed).Hours() / 24)
				days = fmt.Sprintf("%d", d)
			}
			fmt.Printf("%-24s  %-6s  %-8.2f  %s\n",
				m.ID, days, rf, format.Truncate(m.Title, 40))
			found = true
		}
	}

	if !found {
		fmt.Println("  No motes at risk of significant decay.")
	}
	return nil
}

func recencyFactor(lastAccessed *time.Time, cfg core.ScoringConfig) float64 {
	tiers := cfg.RecencyDecay.Tiers
	if len(tiers) == 0 {
		return 1.0
	}
	if lastAccessed == nil {
		return tiers[len(tiers)-1].Factor
	}
	days := int(time.Since(*lastAccessed).Hours() / 24)
	for _, tier := range tiers {
		if tier.MaxDays != nil && days < *tier.MaxDays {
			return tier.Factor
		}
	}
	return tiers[len(tiers)-1].Factor
}

type dreamCostStats struct {
	runs          int
	inputTokens   int
	outputTokens  int
	estimatedCost float64
}

func readDreamCostStats(dreamDir string) dreamCostStats {
	var stats dreamCostStats
	logPath := filepath.Join(dreamDir, "log.jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		return stats
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry dream.RunLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		stats.runs++
		stats.inputTokens += entry.InputTokens
		stats.outputTokens += entry.OutputTokens
		stats.estimatedCost += entry.EstimatedCost
	}
	return stats
}

func countActiveContradictions(motes []*core.Mote) int {
	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	type pair struct{ a, b string }
	seen := make(map[pair]bool)
	count := 0

	for _, m := range motes {
		if m.Status == "deprecated" || m.Status == "archived" {
			continue
		}
		for _, cID := range m.Contradicts {
			other, ok := moteMap[cID]
			if !ok || other.Status == "deprecated" || other.Status == "archived" {
				continue
			}
			p := pair{m.ID, cID}
			pr := pair{cID, m.ID}
			if !seen[p] && !seen[pr] {
				seen[p] = true
				count++
			}
		}
	}
	return count
}
