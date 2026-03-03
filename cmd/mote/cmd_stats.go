package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/strata"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show retrieval health dashboard",
	RunE:  runStats,
}

var statsDecayPreview bool

func init() {
	statsCmd.Flags().BoolVar(&statsDecayPreview, "decay-preview", false, "Show motes at risk of recency decay")
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

	fmt.Println(format.Header("Nebula Stats"))
	fmt.Println()
	fmt.Printf("  Total motes:  %d\n", len(motes))
	for _, s := range []string{"active", "deprecated", "archived", "completed"} {
		if c := statusCounts[s]; c > 0 {
			fmt.Printf("    %-12s %d\n", s+":", c)
		}
	}

	// Access distribution
	now := time.Now()
	var accessed7, accessed30, accessed90, neverAccessed int
	var recencySum float64
	var activeCount int

	for _, m := range motes {
		if m.LastAccessed == nil {
			neverAccessed++
			if m.Status == "active" {
				recencySum += recencyFactor(nil, cfg.Scoring)
				activeCount++
			}
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

	// Tag health
	overloaded := 0
	singletons := 0
	for _, count := range idx.TagStats {
		if count > 15 {
			overloaded++
		}
		if count == 1 {
			singletons++
		}
	}

	fmt.Println()
	fmt.Println(format.Header("Tag Health"))
	fmt.Println()
	fmt.Printf("  Total tags:      %d\n", len(idx.TagStats))
	fmt.Printf("  Overloaded (>15): %d\n", overloaded)
	fmt.Printf("  Singletons (=1): %d\n", singletons)

	// Active contradictions
	contradictions := countActiveContradictions(motes)
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
	dreamDir := filepath.Join(root, "dream")
	if _, err := os.Stat(dreamDir); os.IsNotExist(err) {
		fmt.Println("  Status: N/A (no dream directory)")
	} else {
		visionsPath := filepath.Join(dreamDir, "visions.jsonl")
		if data, err := os.ReadFile(visionsPath); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			count := 0
			for _, l := range lines {
				if l != "" {
					count++
				}
			}
			fmt.Printf("  Pending visions: %d\n", count)
		} else {
			fmt.Println("  Pending visions: 0")
		}
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
