package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/security"
)

var progressCmd = &cobra.Command{
	Use:   "progress <parent-id>",
	Short: "Show hierarchical progress for a parent mote",
	Args:  cobra.ExactArgs(1),
	RunE:  runProgress,
}

func init() {
	rootCmd.AddCommand(progressCmd)
}

func runProgress(cmd *cobra.Command, args []string) error {
	parentID := args[0]
	if err := security.ValidateMoteID(parentID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}
	mm := core.NewMoteManager(root)

	parent, err := mm.Read(parentID)
	if err != nil {
		return fmt.Errorf("read parent: %w", err)
	}

	children, err := mm.Children(parentID)
	if err != nil {
		return fmt.Errorf("get children: %w", err)
	}

	// Header
	sizeStr := ""
	if parent.Size != "" {
		sizeStr = fmt.Sprintf(" (size: %s)", parent.Size)
	}
	fmt.Printf("Progress for %s %q [%s]%s\n", parent.ID, parent.Title, parent.Status, sizeStr)

	if len(children) == 0 {
		fmt.Println("  No children.")
		return nil
	}

	// Count completion
	completed := 0
	for _, c := range children {
		if c.Status == "completed" {
			completed++
		}
	}
	fmt.Printf("  Subtasks: %d/%d completed\n\n", completed, len(children))

	// Build mote map for ready detection
	allMotes, _ := mm.ReadAllParallel()
	moteMap := make(map[string]*core.Mote, len(allMotes))
	for _, m := range allMotes {
		moteMap[m.ID] = m
	}

	for _, c := range children {
		marker := "[ ]"
		if c.Status == "completed" {
			marker = "[x]"
		}

		extra := ""
		if c.Status == "active" {
			if isReady(c, moteMap) {
				extra = " <- READY"
			} else {
				// Show what's blocking
				for _, depID := range c.DependsOn {
					if dep, ok := moteMap[depID]; ok && dep.Status == "active" {
						extra = fmt.Sprintf(" (blocked by %s)", depID)
						break
					}
				}
			}
		}

		fmt.Printf("  %s %s %q [%s]%s\n", marker, c.ID, format.Truncate(c.Title, 40), c.Status, extra)

		// Show acceptance progress if any
		if len(c.Acceptance) > 0 {
			met := 0
			for i, a := range c.AcceptanceMet {
				if i < len(c.Acceptance) && a {
					met++
				}
			}
			fmt.Printf("      Acceptance: %d/%d met\n", met, len(c.Acceptance))
		}
	}

	return nil
}

func isReady(m *core.Mote, moteMap map[string]*core.Mote) bool {
	if m.Status != "active" {
		return false
	}
	if len(m.DependsOn) == 0 {
		return true
	}
	for _, depID := range m.DependsOn {
		dep, ok := moteMap[depID]
		if !ok || dep.Status == "active" {
			return false
		}
	}
	return true
}
