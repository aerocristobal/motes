// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var updateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a mote's fields",
	Args:  cobra.ExactArgs(1),
	RunE:  runUpdate,
}

var (
	updateStatus string
	updateTitle  string
	updateWeight float64
	updateAddTag []string
	updateBody   string
	updateAccept []string
	updateSize   string
	updateParent string
	updateForce  bool
	updateQuiet  bool
)

func init() {
	updateCmd.Flags().StringVar(&updateStatus, "status", "", "New status (active|completed|archived|deprecated)")
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "New title")
	updateCmd.Flags().Float64Var(&updateWeight, "weight", 0, "New weight (0.0-1.0)")
	updateCmd.Flags().StringSliceVar(&updateAddTag, "add-tag", nil, "Tag to append (repeatable)")
	updateCmd.Flags().StringVar(&updateBody, "body", "", "New body content")
	updateCmd.Flags().StringSliceVar(&updateAccept, "accept", nil, "Acceptance criterion to append (repeatable)")
	updateCmd.Flags().StringVar(&updateSize, "size", "", "Effort size (xs|s|m|l|xl)")
	updateCmd.Flags().StringVar(&updateParent, "parent", "", "Parent mote ID")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Bypass security scan blocks (for false positives)")
	updateCmd.Flags().BoolVar(&updateQuiet, "quiet", false, "Suppress security scan warnings on stderr")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if !cmd.Flags().Changed("status") && !cmd.Flags().Changed("title") && !cmd.Flags().Changed("weight") && !cmd.Flags().Changed("add-tag") && !cmd.Flags().Changed("body") && !cmd.Flags().Changed("accept") && !cmd.Flags().Changed("size") && !cmd.Flags().Changed("parent") {
		return fmt.Errorf("at least one flag required: --status, --title, --weight, --add-tag, --body, --accept, --size, --parent")
	}

	moteID := args[0]

	// Validate mote ID
	if err := security.ValidateMoteID(moteID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	// Validate input parameters
	if cmd.Flags().Changed("status") {
		if err := security.ValidateEnum(updateStatus, core.ValidStatuses, "status"); err != nil {
			return fmt.Errorf("invalid status: %w", err)
		}
	}

	if cmd.Flags().Changed("title") {
		if updateTitle == "" {
			return fmt.Errorf("title cannot be empty")
		}
		if len(updateTitle) > 200 {
			return fmt.Errorf("title too long (max 200 characters)")
		}
	}

	if cmd.Flags().Changed("weight") {
		if err := security.ValidateWeight(updateWeight); err != nil {
			return fmt.Errorf("invalid weight: %w", err)
		}
	}

	if cmd.Flags().Changed("add-tag") {
		for _, tag := range updateAddTag {
			if err := security.ValidateTag(tag); err != nil {
				return fmt.Errorf("invalid tag %q: %w", tag, err)
			}
		}
	}

	if cmd.Flags().Changed("body") {
		if len(updateBody) > 10000 {
			return fmt.Errorf("body too long (max 10000 characters)")
		}
	}

	if cmd.Flags().Changed("size") {
		if err := security.ValidateEnum(updateSize, core.ValidSizes, "size"); err != nil {
			return fmt.Errorf("invalid size: %w", err)
		}
	}

	if cmd.Flags().Changed("parent") {
		if updateParent != "" {
			if err := security.ValidateMoteID(updateParent); err != nil {
				return fmt.Errorf("invalid parent ID: %w", err)
			}
		}
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)

	var opts core.UpdateOpts
	var parts []string

	if cmd.Flags().Changed("status") {
		opts.Status = &updateStatus
		parts = append(parts, fmt.Sprintf("status=%s", updateStatus))
	}
	if cmd.Flags().Changed("title") {
		opts.Title = &updateTitle
		parts = append(parts, fmt.Sprintf("title=%s", updateTitle))
	}
	if cmd.Flags().Changed("weight") {
		opts.Weight = &updateWeight
		parts = append(parts, fmt.Sprintf("weight=%v", updateWeight))
	}
	if cmd.Flags().Changed("add-tag") {
		m, err := mm.Read(moteID)
		if err != nil {
			return fmt.Errorf("read mote: %w", err)
		}
		tags := m.Tags
		for _, t := range updateAddTag {
			tags = append(tags, t)
		}
		opts.Tags = tags
		parts = append(parts, fmt.Sprintf("tags=%v", tags))
	}
	if cmd.Flags().Changed("body") {
		opts.Body = &updateBody
		parts = append(parts, fmt.Sprintf("body=%s", updateBody))
	}
	if cmd.Flags().Changed("accept") {
		m, err := mm.Read(moteID)
		if err != nil {
			return fmt.Errorf("read mote: %w", err)
		}
		acceptance := m.Acceptance
		acceptanceMet := m.AcceptanceMet
		for _, a := range updateAccept {
			acceptance = append(acceptance, a)
			acceptanceMet = append(acceptanceMet, false)
		}
		opts.Acceptance = acceptance
		opts.AcceptanceMet = acceptanceMet
		parts = append(parts, fmt.Sprintf("acceptance=%v", acceptance))
	}
	if cmd.Flags().Changed("size") {
		opts.Size = &updateSize
		parts = append(parts, fmt.Sprintf("size=%s", updateSize))
	}
	if cmd.Flags().Changed("parent") {
		opts.Parent = &updateParent
		parts = append(parts, fmt.Sprintf("parent=%s", updateParent))
	}

	opts.Force = updateForce
	opts.Quiet = updateQuiet

	if err := mm.Update(moteID, opts); err != nil {
		return fmt.Errorf("update mote: %w", err)
	}

	// Print confirmation
	fmt.Fprintf(os.Stdout, "Updated %s:", moteID)
	for _, p := range parts {
		fmt.Fprintf(os.Stdout, " %s", p)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}
