package main

import (
	"github.com/spf13/cobra"
	"motes/internal/core"
)

var (
	pulseCompact bool
	pulseJSON    bool
)

var pulseCmd = &cobra.Command{
	Use:   "pulse",
	Short: "Show active tasks sorted by weight (alias for ls --status=active --type=task)",
	RunE:  runPulse,
}

func init() {
	pulseCmd.Flags().BoolVar(&pulseCompact, "compact", false, "One-line-per-mote compact output: ID: Title")
	pulseCmd.Flags().BoolVar(&pulseJSON, "json", false, "Output in JSON format")
	rootCmd.AddCommand(pulseCmd)
}

func runPulse(cmd *cobra.Command, args []string) error {
	return doLs(core.ListFilters{
		Status: "active",
		Type:   "task",
	}, true, pulseCompact, pulseJSON)
}
