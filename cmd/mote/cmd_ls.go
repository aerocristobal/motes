package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List motes with optional filters",
	RunE:  runLs,
}

var (
	lsType    string
	lsTag     string
	lsStatus  string
	lsStale   bool
	lsReady   bool
	lsCompact bool
)

func init() {
	lsCmd.Flags().StringVar(&lsType, "type", "", "Filter by mote type")
	lsCmd.Flags().StringVar(&lsTag, "tag", "", "Filter by tag")
	lsCmd.Flags().StringVar(&lsStatus, "status", "", "Filter by status")
	lsCmd.Flags().BoolVar(&lsStale, "stale", false, "Show motes with no access in 90+ days")
	lsCmd.Flags().BoolVar(&lsReady, "ready", false, "Show tasks with zero unfinished blockers")
	lsCmd.Flags().BoolVar(&lsCompact, "compact", false, "One-line-per-mote compact output: ID: Title")
	rootCmd.AddCommand(lsCmd)
}

func runLs(cmd *cobra.Command, args []string) error {
	return doLs(core.ListFilters{
		Type:   lsType,
		Tag:    lsTag,
		Status: lsStatus,
		Stale:  lsStale,
		Ready:  lsReady,
	}, false, lsCompact)
}

func doLs(filters core.ListFilters, sortByWeight bool, compact bool) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	motes, err := mm.List(filters)
	if err != nil {
		return err
	}

	if len(motes) == 0 {
		fmt.Println("No motes found.")
		return nil
	}

	if sortByWeight {
		sort.Slice(motes, func(i, j int) bool {
			return motes[i].Weight > motes[j].Weight
		})
	}

	if compact {
		for _, m := range motes {
			fmt.Printf("%s: %s\n", m.ID, m.Title)
		}
		return nil
	}

	fmt.Printf("%-24s  %-14s  %-12s  %-8s  %s\n",
		"ID", "TYPE", "STATUS", "WEIGHT", "TITLE")
	fmt.Println(strings.Repeat("-", 80))

	for _, m := range motes {
		title := format.Truncate(m.Title, 40)
		if m.Status == "deprecated" {
			title = "[deprecated] " + title
		}
		fmt.Printf("%-24s  %-14s  %-12s  %-8.2f  %s\n",
			m.ID, m.Type, m.Status, m.Weight, title)
	}
	return nil
}
