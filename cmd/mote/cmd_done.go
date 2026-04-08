// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var doneCmd = &cobra.Command{
	Use:   "done <title>",
	Short: "Record a completed task in one step",
	Long: `Create a task mote and immediately mark it completed.

Shorthand for: mote add --type=task --title="..." --status=completed

Use this for trivial changes completed in the same session they were
started — typo fixes, one-liners, small config tweaks.`,
	Args: cobra.ExactArgs(1),
	RunE: runDone,
}

var (
	doneTags   []string
	doneWeight float64
)

func init() {
	doneCmd.Flags().StringSliceVar(&doneTags, "tag", nil, "Tag (repeatable)")
	doneCmd.Flags().Float64Var(&doneWeight, "weight", 0.5, "Weight (0.0-1.0)")
	rootCmd.AddCommand(doneCmd)
}

func runDone(cmd *cobra.Command, args []string) error {
	title := args[0]

	if len(title) == 0 {
		return fmt.Errorf("title cannot be empty")
	}
	if len(title) > 200 {
		return fmt.Errorf("title too long (max 200 characters)")
	}
	if err := security.ValidateWeight(doneWeight); err != nil {
		return fmt.Errorf("invalid weight: %w", err)
	}
	for _, tag := range doneTags {
		if err := security.ValidateTag(tag); err != nil {
			return fmt.Errorf("invalid tag %q: %w", tag, err)
		}
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)

	m, err := mm.Create("task", title, core.CreateOpts{
		Tags:   doneTags,
		Weight: doneWeight,
	})
	if err != nil {
		return fmt.Errorf("create mote: %w", err)
	}

	completed := "completed"
	if err := mm.Update(m.ID, core.UpdateOpts{Status: &completed}); err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Recorded: %s — %s\n", m.ID, title)
	return nil
}
