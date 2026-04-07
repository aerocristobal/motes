// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/security"
)

var diffCmd = &cobra.Command{
	Use:   "diff <id>",
	Short: "Show field-level changes to a mote since last access",
	Args:  cobra.ExactArgs(1),
	RunE:  runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	moteID := args[0]
	if err := security.ValidateMoteID(moteID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	m, err := mm.Read(moteID)
	if err != nil {
		return fmt.Errorf("read mote: %w", err)
	}

	snap, err := mm.LoadSnapshot(moteID)
	if err != nil {
		fmt.Printf("No prior snapshot for %s. Access this mote first to create a baseline.\n", moteID)
		return nil
	}

	diffs := core.DiffMote(m, snap)
	if len(diffs) == 0 {
		fmt.Printf("No changes to %s since last access (%s).\n", moteID, snap.SnapshotAt)
		return nil
	}

	fmt.Println(format.Header("Diff: " + moteID))
	fmt.Printf("  snapshot:        %s\n", snap.SnapshotAt)
	fmt.Println()
	for _, d := range diffs {
		if d.Field == "body" {
			fmt.Printf("  %-16s (modified)\n", d.Field+":")
		} else {
			fmt.Printf("  %-16s %s → %s\n", d.Field+":", d.OldValue, d.NewValue)
		}
	}

	return nil
}
