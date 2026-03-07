package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Soft-delete a mote (move to trash)",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	moteID := args[0]

	if err := security.ValidateMoteID(moteID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	if _, err := im.Load(); err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	// Read first to get title for confirmation message
	m, err := mm.Read(moteID)
	if err != nil {
		return fmt.Errorf("mote not found: %w", err)
	}

	if err := mm.Delete(moteID, im); err != nil {
		return fmt.Errorf("delete mote: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Deleted %s (%s) — moved to trash\n", moteID, m.Title)
	fmt.Fprintf(os.Stdout, "Restore with: mote trash restore %s\n", moteID)
	return nil
}
