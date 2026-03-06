package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/dream"
)

var dreamCmd = &cobra.Command{
	Use:   "dream",
	Short: "Run headless dream cycle for maintenance and analysis",
	RunE:  runDream,
}

var (
	dreamDryRun    bool
	dreamReview    bool
	dreamAutoApply bool
)

func init() {
	dreamCmd.Flags().BoolVar(&dreamDryRun, "dry-run", false, "Show plan without running Claude")
	dreamCmd.Flags().BoolVar(&dreamReview, "review", false, "Review pending visions interactively")
	dreamCmd.Flags().BoolVar(&dreamAutoApply, "auto-apply", false, "Auto-apply low-risk visions")
	rootCmd.AddCommand(dreamCmd)
}

func runDream(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return err
	}

	if dreamReview {
		return runDreamReview(root, cfg)
	}

	fmt.Println("Starting dream cycle...")
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
		if dreamAutoApply && result.Visions > 0 {
			applied, deferred, err := orch.AutoApply()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: auto-apply error: %v\n", err)
			} else {
				fmt.Printf("  Auto-applied: %d low-risk visions\n", applied)
				if deferred > 0 {
					fmt.Printf("  Deferred: %d high-risk visions (run: mote dream --review)\n", deferred)
				}
			}
		} else if result.Visions > 0 {
			fmt.Println("Run 'mote dream --review' to review pending visions.")
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
