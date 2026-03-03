package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var constellationCmd = &cobra.Command{
	Use:   "constellation",
	Short: "Manage constellation discovery",
}

var constellationListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show tag frequency and constellation status",
	RunE:  runConstellationList,
}

func init() {
	constellationCmd.AddCommand(constellationListCmd)
	rootCmd.AddCommand(constellationCmd)
}

type tagEntry struct {
	Tag           string
	Count         int
	Specificity   float64
	Constellation string // mote ID if exists, "-" otherwise
}

func runConstellationList(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	motes, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	// Build map of tag -> constellation mote ID
	constellationMap := make(map[string]string)
	for _, m := range motes {
		if m.Type == "constellation" {
			for _, tag := range m.Tags {
				constellationMap[tag] = m.ID
			}
		}
	}

	// Build tag entries
	var entries []tagEntry
	for tag, count := range idx.TagStats {
		specificity := 1.0 / math.Log2(float64(count)+2)
		constellation := "-"
		if cID, ok := constellationMap[tag]; ok {
			constellation = cID
		}
		entries = append(entries, tagEntry{
			Tag:           tag,
			Count:         count,
			Specificity:   specificity,
			Constellation: constellation,
		})
	}

	// Sort by count descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})

	if len(entries) == 0 {
		fmt.Println("No tags found.")
		return nil
	}

	fmt.Printf("%-20s  %-6s  %-12s  %s\n", "TAG", "COUNT", "SPECIFICITY", "CONSTELLATION")
	fmt.Println(strings.Repeat("-", 70))
	for _, e := range entries {
		fmt.Printf("%-20s  %-6d  %-12.2f  %s\n",
			e.Tag, e.Count, e.Specificity, e.Constellation)
	}

	return nil
}
