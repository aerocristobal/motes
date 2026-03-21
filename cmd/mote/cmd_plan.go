package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var planCmd = &cobra.Command{
	Use:   "plan <parent-id>",
	Short: "Create child tasks under a parent mote",
	Long:  "Batch-create child tasks under a parent. Use --sequential to chain them with depends_on links.",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlan,
}

var (
	planChildren   []string
	planSequential bool
	planTag        []string
)

func init() {
	planCmd.Flags().StringSliceVar(&planChildren, "child", nil, "Child task title (repeatable)")
	planCmd.Flags().BoolVar(&planSequential, "sequential", false, "Chain children with depends_on links")
	planCmd.Flags().StringSliceVar(&planTag, "tag", nil, "Additional tag for children (repeatable)")
	_ = planCmd.MarkFlagRequired("child")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	parentID := args[0]
	if err := security.ValidateMoteID(parentID); err != nil {
		return fmt.Errorf("invalid parent ID: %w", err)
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	parent, err := mm.Read(parentID)
	if err != nil {
		return fmt.Errorf("read parent: %w", err)
	}

	// Inherit parent tags and merge with extra tags
	tags := make([]string, len(parent.Tags))
	copy(tags, parent.Tags)
	for _, t := range planTag {
		if err := security.ValidateTag(t); err != nil {
			return fmt.Errorf("invalid tag %q: %w", t, err)
		}
		if !sliceContainsStr(tags, t) {
			tags = append(tags, t)
		}
	}

	var created []*core.Mote
	for _, title := range planChildren {
		if title == "" {
			continue
		}
		if len(title) > 200 {
			return fmt.Errorf("child title too long (max 200): %q", title)
		}
		m, err := mm.Create("task", title, core.CreateOpts{
			Parent: parentID,
			Tags:   tags,
		})
		if err != nil {
			return fmt.Errorf("create child %q: %w", title, err)
		}
		created = append(created, m)
	}

	if len(created) == 0 {
		return fmt.Errorf("no children created")
	}

	// Chain with depends_on if sequential
	if planSequential && len(created) > 1 {
		for i := 1; i < len(created); i++ {
			if err := mm.Link(created[i].ID, "depends_on", created[i-1].ID, im); err != nil {
				return fmt.Errorf("link %s -> %s: %w", created[i].ID, created[i-1].ID, err)
			}
		}
	}

	// Rebuild index
	allMotes, _ := mm.ReadAllParallel()
	if allMotes != nil {
		im.Rebuild(allMotes)
	}

	fmt.Printf("Created %d children under %s:\n", len(created), parentID)
	for _, m := range created {
		fmt.Printf("  %s: %s\n", m.ID, m.Title)
	}
	return nil
}
