package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var linkDryRun bool

var linkCmd = &cobra.Command{
	Use:   "link <source-id> <link-type> <target-id>",
	Short: "Create a link between two motes",
	Args:  cobra.ExactArgs(3),
	RunE:  runLink,
}

var unlinkCmd = &cobra.Command{
	Use:   "unlink <source-id> <link-type> <target-id>",
	Short: "Remove a link between two motes",
	Args:  cobra.ExactArgs(3),
	RunE:  runUnlink,
}

func init() {
	linkCmd.Flags().BoolVar(&linkDryRun, "dry-run", false, "Preview link effects without writing")
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	sourceID, linkType, targetID := args[0], args[1], args[2]

	// Validate input parameters
	if err := security.ValidateMoteID(sourceID); err != nil {
		return fmt.Errorf("invalid source ID: %w", err)
	}
	if err := security.ValidateMoteID(targetID); err != nil {
		return fmt.Errorf("invalid target ID: %w", err)
	}

	// Validate link type is known
	if _, ok := core.ValidLinkTypes[linkType]; !ok {
		return fmt.Errorf("unknown link type: %q", linkType)
	}

	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	if linkDryRun {
		effects, err := mm.PreviewLink(sourceID, linkType, targetID)
		if err != nil {
			return err
		}
		fmt.Printf("[dry-run] %s --%s--> %s\n", sourceID, linkType, targetID)
		for _, e := range effects {
			if e.OldValue != "" && e.OldValue == e.NewValue {
				fmt.Printf("  %-16s %s (no-op)\n", e.MoteID+":", e.Description)
			} else {
				fmt.Printf("  %-16s %s: %s -> %s\n", e.MoteID+":", e.Field, e.Description, e.NewValue)
			}
		}
		return nil
	}

	im := core.NewIndexManager(root)
	if _, err := im.Load(); err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	if err := mm.LinkLocked(sourceID, linkType, targetID, im); err != nil {
		return err
	}

	behavior := core.ValidLinkTypes[linkType]
	fmt.Printf("Linked %s --%s--> %s\n", sourceID, linkType, targetID)
	if behavior.Symmetric {
		fmt.Printf("  (symmetric: also %s --%s--> %s)\n", targetID, linkType, sourceID)
	} else if behavior.InverseType != "" {
		fmt.Printf("  (inverse: also %s --%s--> %s)\n", targetID, behavior.InverseType, sourceID)
	}
	if behavior.AutoDeprecate {
		fmt.Printf("  (auto-deprecated %s)\n", targetID)
	}

	return nil
}

func runUnlink(cmd *cobra.Command, args []string) error {
	sourceID, linkType, targetID := args[0], args[1], args[2]

	// Validate input parameters
	if err := security.ValidateMoteID(sourceID); err != nil {
		return fmt.Errorf("invalid source ID: %w", err)
	}
	if err := security.ValidateMoteID(targetID); err != nil {
		return fmt.Errorf("invalid target ID: %w", err)
	}

	// Validate link type is known
	if _, ok := core.ValidLinkTypes[linkType]; !ok {
		return fmt.Errorf("unknown link type: %q", linkType)
	}

	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	if _, err := im.Load(); err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	if err := mm.UnlinkLocked(sourceID, linkType, targetID, im); err != nil {
		return err
	}

	fmt.Printf("Unlinked %s --%s--> %s\n", sourceID, linkType, targetID)
	return nil
}
