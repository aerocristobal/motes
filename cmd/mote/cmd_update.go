package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/core"
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
)

func init() {
	updateCmd.Flags().StringVar(&updateStatus, "status", "", "New status (active|completed|archived|deprecated)")
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "New title")
	updateCmd.Flags().Float64Var(&updateWeight, "weight", 0, "New weight (0.0-1.0)")
	updateCmd.Flags().StringArrayVar(&updateAddTag, "add-tag", nil, "Tag to append (repeatable)")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if !cmd.Flags().Changed("status") && !cmd.Flags().Changed("title") && !cmd.Flags().Changed("weight") && !cmd.Flags().Changed("add-tag") {
		return fmt.Errorf("at least one flag required: --status, --title, --weight, --add-tag")
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	moteID := args[0]
	mm := core.NewMoteManager(root)

	fields := map[string]interface{}{}

	if cmd.Flags().Changed("status") {
		fields["status"] = updateStatus
	}
	if cmd.Flags().Changed("title") {
		fields["title"] = updateTitle
	}
	if cmd.Flags().Changed("weight") {
		fields["weight"] = updateWeight
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
		fields["tags"] = tags
	}

	if err := mm.Update(moteID, fields); err != nil {
		return fmt.Errorf("update mote: %w", err)
	}

	// Print confirmation
	var parts []string
	for k, v := range fields {
		if k == "tags" {
			parts = append(parts, fmt.Sprintf("tags=%v", v))
		} else {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	fmt.Fprintf(os.Stdout, "Updated %s:", moteID)
	for _, p := range parts {
		fmt.Fprintf(os.Stdout, " %s", p)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}
