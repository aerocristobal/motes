package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

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
	dreamDryRun bool
	dreamReview bool
	dreamManual bool
	dreamStats  bool
	dreamJSON   bool
)

func init() {
	dreamCmd.Flags().BoolVar(&dreamDryRun, "dry-run", false, "Show plan without running Claude")
	dreamCmd.Flags().BoolVar(&dreamReview, "review", false, "Review pending visions interactively")
	dreamCmd.Flags().BoolVar(&dreamManual, "manual", false, "Enter manual review mode instead of auto-applying")
	dreamCmd.Flags().BoolVar(&dreamStats, "stats", false, "Show feedback statistics for auto-applied visions")
	dreamCmd.Flags().BoolVar(&dreamJSON, "json", false, "Output pending visions in JSON format (use with --review)")
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

	orch := dream.NewDreamOrchestrator(root, cfg)
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
		fmt.Printf("\nDream cycle complete: %d batches, %d visions.\n", result.Batches, result.Visions)
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
