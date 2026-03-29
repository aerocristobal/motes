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
	TotalMotes            int            `json:"total_motes"`
	StatusCounts          map[string]int `json:"status_counts"`
	Accessed7             int            `json:"accessed_7d"`
	Accessed30            int            `json:"accessed_30d"`
	Accessed90            int            `json:"accessed_90d"`
	NeverAccessed         int            `json:"never_accessed"`
	TotalTags             int            `json:"total_tags"`
	OverloadedTags        int            `json:"overloaded_tags"`
	SingletonTags         int            `json:"singleton_tags"`
	Contradictions        int            `json:"contradictions"`
	PendingVisions        int            `json:"pending_visions"`
	DreamRuns             int            `json:"dream_runs,omitempty"`
	DreamInputTokens      int            `json:"dream_input_tokens,omitempty"`
	DreamOutputTokens     int            `json:"dream_output_tokens,omitempty"`
	DreamEstimatedCost    float64        `json:"dream_estimated_cost,omitempty"`
	DreamTotalVisions     int            `json:"dream_total_visions,omitempty"`
	DreamTotalApplied     int            `json:"dream_total_applied,omitempty"`
	DreamTotalDeferred    int            `json:"dream_total_deferred,omitempty"`
	DreamAcceptanceRate   float64        `json:"dream_acceptance_rate,omitempty"`
	DreamCostPerAccepted  float64        `json:"dream_cost_per_accepted,omitempty"`
	PrimeHitRate          float64        `json:"prime_hit_rate,omitempty"`
	PrimeSessions         int            `json:"prime_sessions,omitempty"`
	Created7d             int            `json:"created_7d,omitempty"`
	Created30d            int            `json:"created_30d,omitempty"`
	Created90d            int            `json:"created_90d,omitempty"`
	Deprecated7d          int            `json:"deprecated_7d,omitempty"`
	Deprecated30d         int            `json:"deprecated_30d,omitempty"`
	Deprecated90d         int            `json:"deprecated_90d,omitempty"`
	NetGrowth7d           int            `json:"net_growth_7d,omitempty"`
	NetGrowth30d          int            `json:"net_growth_30d,omitempty"`
	NetGrowth90d          int            `json:"net_growth_90d,omitempty"`
	GraphDecisions        int            `json:"graph_decisions,omitempty"`
	GraphLessons          int            `json:"graph_lessons,omitempty"`
	GraphExplorations     int            `json:"graph_explorations,omitempty"`
	GraphKnowledgeCount   int            `json:"graph_knowledge_count,omitempty"`
	GraphAvgLinks         float64        `json:"graph_avg_links,omitempty"`
	GraphCrossSession     int            `json:"graph_cross_session_motes,omitempty"`
	GraphAgeDays          int            `json:"graph_age_days,omitempty"`
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

	motes, err := readAllWithGlobal(mm)
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

	// Read cumulative dream cost data and run entries for last-5 acceptance check
	dreamCost := readDreamCostStats(dreamDir)
	dreamEntries := readDreamRunEntries(dreamDir)

	// Compute graph flow metrics
	flow := computeFlowStats(motes)

	// Read prime session stats for hit-rate feedback and graph value
	primeStats, _ := mm.ReadPrimeSessionStats(0)
	var totalPrimed, totalHit int
	for _, s := range primeStats {
		totalPrimed += s.PrimedCount
		totalHit += s.HitCount
	}
	rollingHitRate := 0.0
	if totalPrimed > 0 {
		rollingHitRate = float64(totalHit) / float64(totalPrimed)
	}

	gv := computeGraphValue(motes, idx, primeStats)

	// Dream ROI: acceptance rate and cost per accepted vision
	dreamAcceptRate := 0.0
	if dreamCost.totalVisions > 0 {
		dreamAcceptRate = float64(dreamCost.totalApplied) / float64(dreamCost.totalVisions)
	}
	dreamCostPerAccepted := 0.0
	if dreamCost.totalApplied > 0 {
		dreamCostPerAccepted = dreamCost.estimatedCost / float64(dreamCost.totalApplied)
	}

	if statsJSON {
		out := StatsOutput{
			TotalMotes:           len(motes),
			StatusCounts:         statusCounts,
			Accessed7:            accessed7,
			Accessed30:           accessed30,
			Accessed90:           accessed90,
			NeverAccessed:        neverAccessed,
			TotalTags:            len(idx.TagStats),
			OverloadedTags:       overloaded,
			SingletonTags:        singletons,
			Contradictions:       contradictions,
			PendingVisions:       pendingVisions,
			DreamRuns:            dreamCost.runs,
			DreamInputTokens:     dreamCost.inputTokens,
			DreamOutputTokens:    dreamCost.outputTokens,
			DreamEstimatedCost:   dreamCost.estimatedCost,
			DreamTotalVisions:    dreamCost.totalVisions,
			DreamTotalApplied:    dreamCost.totalApplied,
			DreamTotalDeferred:   dreamCost.totalDeferred,
			DreamAcceptanceRate:  dreamAcceptRate,
			DreamCostPerAccepted: dreamCostPerAccepted,
			PrimeHitRate:         rollingHitRate,
			PrimeSessions:        len(primeStats),
			Created7d:            flow.Created7d,
			Created30d:           flow.Created30d,
			Created90d:           flow.Created90d,
			Deprecated7d:         flow.Deprecated7d,
			Deprecated30d:        flow.Deprecated30d,
			Deprecated90d:        flow.Deprecated90d,
			NetGrowth7d:          flow.Created7d - flow.Deprecated7d,
			NetGrowth30d:         flow.Created30d - flow.Deprecated30d,
			NetGrowth90d:         flow.Created90d - flow.Deprecated90d,
			GraphDecisions:       gv.decisions,
			GraphLessons:         gv.lessons,
			GraphExplorations:    gv.explorations,
			GraphKnowledgeCount:  gv.decisions + gv.lessons + gv.explorations,
			GraphAvgLinks:        gv.avgLinks,
			GraphCrossSession:    gv.crossSession,
			GraphAgeDays:         gv.ageDays,
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

	if len(primeStats) > 0 {
		fmt.Println()
		fmt.Println(format.Header("Prime Feedback"))
		fmt.Println()
		fmt.Printf("  Sessions tracked: %d\n", len(primeStats))
		fmt.Printf("  Rolling hit rate: %.0f%%\n", rollingHitRate*100)
	}

	knowledgeCount := gv.decisions + gv.lessons + gv.explorations
	if knowledgeCount > 0 {
		fmt.Println()
		fmt.Println(format.Header("Graph Value"))
		fmt.Println()
		fmt.Printf("  Knowledge motes: %d (%d decisions, %d lessons, %d explorations)\n",
			knowledgeCount, gv.decisions, gv.lessons, gv.explorations)
		fmt.Printf("  Avg links/knowledge mote: %.1f\n", gv.avgLinks)
		fmt.Printf("  Cross-session retrievals: %d motes\n", gv.crossSession)
		fmt.Printf("  Graph age: %d days\n", gv.ageDays)
		fmt.Printf("  %d decisions, %d lessons, and %d explorations captured over %d days\n",
			gv.decisions, gv.lessons, gv.explorations, gv.ageDays)
	}

	fmt.Println()
	fmt.Println(format.Header("Graph Flow"))
	fmt.Println()
	fmt.Printf("  %-8s  %8s  %10s  %4s\n", "Window", "Created", "Deprecated", "Net")
	fmt.Printf("  %-8s  %8s  %10s  %4s\n", "--------", "-------", "----------", "----")
	for _, row := range []struct {
		label string
		c, d  int
	}{
		{"Last 7d", flow.Created7d, flow.Deprecated7d},
		{"Last 30d", flow.Created30d, flow.Deprecated30d},
		{"Last 90d", flow.Created90d, flow.Deprecated90d},
	} {
		net := row.c - row.d
		sign := "+"
		if net < 0 {
			sign = ""
		}
		fmt.Printf("  %-8s  %8d  %10d  %s%d\n", row.label, row.c, row.d, sign, net)
	}
	fmt.Printf("\n  Stock (active): %d\n", statusCounts["active"])

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

	// Dream Cycle section (merged with ROI metrics)
	fmt.Println()
	fmt.Println(format.Header("Dream Cycle"))
	fmt.Println()
	if _, err := os.Stat(dreamDir); os.IsNotExist(err) {
		fmt.Println("  Status: N/A (no dream directory)")
	} else {
		fmt.Printf("  Pending visions: %d\n", pendingVisions)
		if dreamCost.runs > 0 {
			fmt.Printf("  Total runs:      %d\n", dreamCost.runs)
			fmt.Printf("  Total visions:   %d\n", dreamCost.totalVisions)
			fmt.Printf("  Applied:         %d\n", dreamCost.totalApplied)
			fmt.Printf("  Deferred:        %d\n", dreamCost.totalDeferred)
			if dreamCost.totalVisions > 0 {
				fmt.Printf("  Acceptance rate: %.0f%%\n", dreamAcceptRate*100)
			} else {
				fmt.Printf("  Acceptance rate: N/A\n")
			}
			fmt.Printf("  Estimated cost:  $%.4f\n", dreamCost.estimatedCost)
			if dreamCost.totalApplied > 0 {
				fmt.Printf("  Cost/accepted:   $%.4f\n", dreamCostPerAccepted)
			}
			// Low acceptance rate note: check last 5 runs
			if len(dreamEntries) >= 5 {
				last5 := dreamEntries[len(dreamEntries)-5:]
				var l5visions, l5applied int
				for _, e := range last5 {
					l5visions += e.Visions
					l5applied += e.AutoApplied
				}
				if l5visions > 0 && float64(l5applied)/float64(l5visions) < 0.25 {
					fmt.Printf("  Note: Low vision acceptance rate. Consider adjusting dream cycle\n")
					fmt.Printf("  frequency or reviewing scoring thresholds in config.yaml.\n")
				}
			}
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

	// High-value decay risk: weight≥0.7 + failure/explore + 30-90d + within 0.1 of threshold
	risks := core.DecayRiskMotes(motes, cfg)
	if len(risks) > 0 {
		fmt.Println()
		fmt.Println(format.Header("High-value decay risk"))
		fmt.Println()
		fmt.Println("  High-weight motes approaching min_relevance_threshold (failure/explore origin, 30-90 days old)")
		fmt.Println()
		fmt.Printf("%-24s  %-6s  %-8s  %-6s  %-6s  %s\n", "ID", "DAYS", "RECENCY", "WEIGHT", "GAP", "TITLE")
		fmt.Println(strings.Repeat("-", 84))
		for _, r := range risks {
			m := findMote(motes, r.MoteID)
			days := "?"
			if m != nil && m.LastAccessed != nil {
				days = fmt.Sprintf("%d", int(time.Since(*m.LastAccessed).Hours()/24))
			}
			title := ""
			if m != nil {
				title = format.Truncate(m.Title, 30)
			}
			fmt.Printf("%-24s  %-6s  %-8.2f  %-6.2f  %-6.3f  %s\n",
				r.MoteID, days, r.RecencyFactor, r.Weight, r.ScoreGap, title)
		}
	}

	return nil
}

func findMote(motes []*core.Mote, id string) *core.Mote {
	for _, m := range motes {
		if m.ID == id {
			return m
		}
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
	totalVisions  int
	totalApplied  int
	totalDeferred int
}

func readDreamCostStats(dreamDir string) dreamCostStats {
	var stats dreamCostStats
	for _, e := range readDreamRunEntries(dreamDir) {
		stats.runs++
		stats.inputTokens += e.InputTokens
		stats.outputTokens += e.OutputTokens
		stats.estimatedCost += e.EstimatedCost
		stats.totalVisions += e.Visions
		stats.totalApplied += e.AutoApplied
		stats.totalDeferred += e.Deferred
	}
	return stats
}

func readDreamRunEntries(dreamDir string) []dream.RunLogEntry {
	logPath := filepath.Join(dreamDir, "log.jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []dream.RunLogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry dream.RunLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
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

type graphValueStats struct {
	decisions    int
	lessons      int
	explorations int
	avgLinks     float64
	crossSession int
	ageDays      int
}

func computeGraphValue(motes []*core.Mote, idx *core.EdgeIndex, primeStats []core.PrimeSessionStats) graphValueStats {
	var gv graphValueStats

	// Knowledge motes: decision, lesson, explore — not deprecated/archived
	var knowledgeIDs []string
	var oldestCreated *time.Time
	for _, m := range motes {
		if m.Status == "deprecated" || m.Status == "archived" {
			continue
		}
		switch m.Type {
		case "decision":
			gv.decisions++
			knowledgeIDs = append(knowledgeIDs, m.ID)
		case "lesson":
			gv.lessons++
			knowledgeIDs = append(knowledgeIDs, m.ID)
		case "explore":
			gv.explorations++
			knowledgeIDs = append(knowledgeIDs, m.ID)
		}
		if !m.CreatedAt.IsZero() {
			if oldestCreated == nil || m.CreatedAt.Before(*oldestCreated) {
				t := m.CreatedAt
				oldestCreated = &t
			}
		}
	}

	// Avg outgoing links per knowledge mote
	if len(knowledgeIDs) > 0 {
		var totalLinks int
		for _, id := range knowledgeIDs {
			totalLinks += len(idx.Neighbors(id, nil))
		}
		gv.avgLinks = float64(totalLinks) / float64(len(knowledgeIDs))
	}

	// Cross-session retrievals: motes appearing in hit_ids of 3+ sessions
	hitFreq := map[string]int{}
	for _, s := range primeStats {
		for _, id := range s.HitIDs {
			hitFreq[id]++
		}
	}
	for _, freq := range hitFreq {
		if freq >= 3 {
			gv.crossSession++
		}
	}

	// Graph age in days
	if oldestCreated != nil {
		gv.ageDays = int(time.Since(*oldestCreated).Hours() / 24)
	}

	return gv
}
