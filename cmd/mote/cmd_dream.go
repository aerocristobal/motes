// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/dream"
)

// DreamReviewOutput is the JSON output for mote dream --review --json.
type DreamReviewOutput struct {
	Visions []dream.Vision `json:"visions"`
}

var dreamCmd = &cobra.Command{
	Use:   "dream",
	Short: "Run headless dream cycle for maintenance and analysis",
	RunE:  runDream,
}

var (
	dreamDryRun        bool
	dreamReview        bool
	dreamManual        bool
	dreamStats         bool
	dreamJSON          bool
	dreamStructuredLog bool
	dreamQuality       bool
	dreamCompare       bool
	dreamLens          bool
	dreamProject       string
	dreamLast          int
)

func init() {
	dreamCmd.Flags().BoolVar(&dreamDryRun, "dry-run", false, "Show plan without running Claude")
	dreamCmd.Flags().BoolVar(&dreamReview, "review", false, "Review pending visions interactively")
	dreamCmd.Flags().BoolVar(&dreamManual, "manual", false, "Enter manual review mode instead of auto-applying")
	dreamCmd.Flags().BoolVar(&dreamStats, "stats", false, "Show feedback statistics for auto-applied visions")
	dreamCmd.Flags().BoolVar(&dreamJSON, "json", false, "Output pending visions in JSON format (use with --review)")
	dreamCmd.Flags().BoolVar(&dreamStructuredLog, "structured-log", false, "Emit JSON log lines to stderr for machine parsing")
	dreamCmd.Flags().BoolVar(&dreamQuality, "quality", false, "Show cycle quality time-series from global ledger")
	dreamCmd.Flags().BoolVar(&dreamCompare, "compare", false, "Compare voting configs across all projects")
	dreamCmd.Flags().BoolVar(&dreamLens, "lens", false, "Show per-lens vision breakdown (use with --quality)")
	dreamCmd.Flags().StringVar(&dreamProject, "project", "", "Filter quality/compare output by project name")
	dreamCmd.Flags().IntVar(&dreamLast, "last", 0, "Limit quality output to last N entries")
	rootCmd.AddCommand(dreamCmd)
}

func runDream(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	if dreamStats {
		return runDreamStats(root)
	}
	if dreamQuality {
		return runDreamQuality(dreamProject, dreamLast, dreamLens)
	}
	if dreamCompare {
		return runDreamCompare(dreamProject)
	}

	// Acquire exclusive dream lock (non-blocking)
	dreamLockMM := core.NewMoteManager(root)
	dreamLock, acquired, lockErr := dreamLockMM.TryLockDream()
	if lockErr != nil {
		return fmt.Errorf("dream lock: %w", lockErr)
	}
	if !acquired {
		return fmt.Errorf("dream cycle already running")
	}
	defer dreamLock.Unlock()

	if dreamJSON && dreamReview {
		vw := dream.NewVisionWriter(root + "/dream")
		visions := vw.ReadFinal()
		data, err := json.MarshalIndent(DreamReviewOutput{Visions: visions}, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	manualMode := dreamManual || dreamReview
	if !manualMode && cfg.Dream.ReviewMode == "manual" {
		manualMode = true
	}

	if manualMode {
		return runDreamReview(root, cfg)
	}

	fmt.Println("Starting dream cycle...")

	// Migrate old auto_applied.jsonl if present
	dream.MigrateAutoApplied(root)

	// Check feedback from previous runs
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	im.Load()
	dream.CheckFeedback(root, mm, im, cfg)

	orch, err := dream.NewDreamOrchestrator(root, cfg)
	if err != nil {
		return fmt.Errorf("dream orchestrator: %w", err)
	}
	orch.SetMoteLoader(func() ([]*core.Mote, error) {
		return readAllWithGlobal(mm)
	})
	if dreamStructuredLog {
		orch.SetLogger(dream.NewDreamLogger(os.Stderr, true))
	}
	result, err := orch.Run(dreamDryRun)
	if err != nil {
		return fmt.Errorf("dream cycle: %w", err)
	}

	switch result.Status {
	case "clean":
		fmt.Println("Nothing to do. Nebula is clean.")
	case "dry-run":
		fmt.Printf("\nDry run complete. Would create %d batches.\n", result.Batches)
	case "complete":
		fmt.Printf("\nDream cycle complete: %d batches, %d visions (~$%.4f estimated).\n", result.Batches, result.Visions, result.EstimatedCost)
		if result.Visions > 0 {
			applied, failed, deferred, err := orch.AutoApply(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: auto-apply error: %v\n", err)
			} else {
				fmt.Printf("  Auto-applied: %d visions\n", applied)
				if failed > 0 {
					fmt.Printf("  Failed: %d (run: mote dream --manual)\n", failed)
				}
				if deferred > 0 {
					fmt.Printf("  Deferred: %d low-confidence (run: mote dream --review)\n", deferred)
				}
				// Update quality entries with auto-apply stats
				avgConf := orch.LastAvgConfidence()
				orch.UpdateRunLogAutoApply(applied, deferred, avgConf)
				_ = dream.UpdateLastEntryAutoApply(applied, deferred, avgConf)
			}
		}
	}
	return nil
}

func runDreamReview(root string, cfg *core.Config) error {
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	vw := dream.NewVisionWriter(root + "/dream")
	reviewer := dream.NewVisionReviewerWithConfig(vw, mm, im, root, cfg)

	result, err := reviewer.Review()
	if err != nil {
		return err
	}

	if result.Accepted+result.Rejected+result.Deferred > 0 {
		fmt.Printf("\nReview complete: %d accepted, %d rejected, %d deferred.\n",
			result.Accepted, result.Rejected, result.Deferred)
	}
	return nil
}

func runDreamQuality(project string, last int, showLens bool) error {
	entries, err := dream.ReadQualityLedger()
	if err != nil {
		return fmt.Errorf("read quality ledger: %w", err)
	}
	if len(entries) == 0 {
		fmt.Println("No quality data yet. Run 'mote dream' to collect cycle quality metrics.")
		return nil
	}

	// Filter by project if specified
	if project != "" {
		var filtered []dream.QualityEntry
		for _, e := range entries {
			if e.Project == project {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Limit to last N
	if last > 0 && len(entries) > last {
		entries = entries[len(entries)-last:]
	}

	if len(entries) == 0 {
		fmt.Println("No matching quality entries.")
		return nil
	}

	fmt.Println("Dream Cycle Quality")
	fmt.Println("====================")
	fmt.Printf("%-12s %-10s %-12s %7s %13s %14s %8s %8s %9s\n",
		"Date", "Project", "Config", "Batches", "Batch→Recon", "Applied/Defer", "AvgConf", "Cost", "$/Vision")
	fmt.Println(strings.Repeat("-", 105))

	for _, e := range entries {
		date := e.Timestamp
		if len(date) >= 10 {
			date = date[:10]
		}
		proj := e.Project
		if len(proj) > 10 {
			proj = proj[:10]
		}
		fmt.Printf("%-12s %-10s %-12s %7d %5d → %-5d %6d / %-5d %7.0f%% %7s %9s\n",
			date, proj, e.VotingConfig, e.Batches,
			e.BatchVisions, e.ReconVisions,
			e.AutoApplied, e.Deferred,
			e.AvgConfidence*100,
			formatCost(e.EstimatedCost),
			formatCost(e.CostPerVision))

		if showLens {
			if len(e.LensBreakdown) == 0 {
				fmt.Printf("  Per-lens: N/A (legacy mode)\n")
			} else {
				lenses := make([]string, 0, len(e.LensBreakdown))
				for l := range e.LensBreakdown {
					lenses = append(lenses, l)
				}
				sort.Strings(lenses)
				for _, l := range lenses {
					count := e.LensBreakdown[l]
					warn := ""
					if count == 0 {
						warn = " ⚠ 0 findings"
					}
					fmt.Printf("  %-22s %d visions%s\n", l+":", count, warn)
				}
			}
		}
	}
	return nil
}

func runDreamCompare(project string) error {
	entries, err := dream.ReadQualityLedger()
	if err != nil {
		return fmt.Errorf("read quality ledger: %w", err)
	}
	if len(entries) == 0 {
		fmt.Println("No quality data yet. Run 'mote dream' to collect cycle quality metrics.")
		return nil
	}

	if project != "" {
		var filtered []dream.QualityEntry
		for _, e := range entries {
			if e.Project == project {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	comparisons := dream.CompareConfigs(entries)
	if len(comparisons) == 0 {
		fmt.Println("No matching quality entries.")
		return nil
	}

	fmt.Println("A/B Comparison (global)")
	fmt.Println("========================")

	// Header
	fmt.Printf("%-22s", "Metric")
	for _, c := range comparisons {
		fmt.Printf(" %20s", fmt.Sprintf("%s (n=%d)", c.Config, c.Cycles))
	}
	fmt.Println()
	fmt.Println(strings.Repeat("-", 22+21*len(comparisons)))

	// Rows
	fmt.Printf("%-22s", "Visions/batch")
	for _, c := range comparisons {
		fmt.Printf(" %20.2f", c.AvgVisionsPerBatch)
	}
	fmt.Println()

	fmt.Printf("%-22s", "Recon filter rate")
	for _, c := range comparisons {
		fmt.Printf(" %19.0f%%", c.AvgReconFilter*100)
	}
	fmt.Println()

	fmt.Printf("%-22s", "Avg confidence")
	for _, c := range comparisons {
		fmt.Printf(" %19.0f%%", c.AvgConfidence*100)
	}
	fmt.Println()

	fmt.Printf("%-22s", "Cost/cycle")
	for _, c := range comparisons {
		fmt.Printf(" %20s", formatCost(c.AvgCostPerCycle))
	}
	fmt.Println()

	fmt.Printf("%-22s", "Cost/vision")
	for _, c := range comparisons {
		fmt.Printf(" %20s", formatCost(c.AvgCostPerVision))
	}
	fmt.Println()

	return nil
}

func formatCost(cost float64) string {
	if cost <= 0 {
		return "-"
	}
	return fmt.Sprintf("$%.4f", cost)
}

func runDreamStats(root string) error {
	stats := dream.GetStats(root)
	if len(stats) == 0 {
		fmt.Println("No feedback data yet. Run 'mote dream' to auto-apply visions and collect feedback.")
		return nil
	}

	fmt.Println("Dream Feedback Statistics")
	fmt.Println("=========================")
	fmt.Printf("%-18s %6s %7s %9s %8s %10s %10s %8s\n",
		"Vision Type", "Total", "Checked", "Persisted", "Reverted", "Avg Delta", "Positive%", "AvgConf")
	fmt.Println("--------------------------------------------------------------------------------------------")

	// Sort keys for stable output
	types := make([]string, 0, len(stats))
	for k := range stats {
		types = append(types, k)
	}
	sort.Strings(types)

	for _, vt := range types {
		s := stats[vt]
		fmt.Printf("%-18s %6d %7d %9d %8d %+9.4f %9.1f%% %7.0f%%\n",
			vt, s.Total, s.Checked, s.Persisted, s.Reverted, s.AvgDelta, s.PositivePct, s.AvgConfidence*100)
	}
	return nil
}
