package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage the mote index",
}

var indexRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild index.jsonl from mote frontmatter",
	RunE:  runIndexRebuild,
}

func init() {
	indexCmd.AddCommand(indexRebuildCmd)
	rootCmd.AddCommand(indexCmd)
}

func runIndexRebuild(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	if err := im.Rebuild(motes); err != nil {
		return fmt.Errorf("rebuild: %w", err)
	}

	idx, _ := im.Load()
	fmt.Printf("Index rebuilt: %d motes, %d edges, %d unique tags\n",
		len(motes), len(idx.Edges), len(idx.TagStats))
	return nil
}
