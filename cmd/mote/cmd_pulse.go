package main

import (
	"github.com/spf13/cobra"
	"motes/internal/core"
)

var pulseCmd = &cobra.Command{
	Use:   "pulse",
	Short: "Show active tasks sorted by weight (alias for ls --status=active --type=task)",
	RunE:  runPulse,
}

func init() {
	rootCmd.AddCommand(pulseCmd)
}

func runPulse(cmd *cobra.Command, args []string) error {
	return doLs(core.ListFilters{
		Status: "active",
		Type:   "task",
	}, true)
}
