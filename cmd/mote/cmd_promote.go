// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var promoteCmd = &cobra.Command{
	Use:   "promote <mote-id>",
	Short: "Promote a project mote to the global memory layer",
	Args:  cobra.ExactArgs(1),
	RunE:  runPromote,
}

func init() {
	rootCmd.AddCommand(promoteCmd)
}

func globalRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "memory")
}

func runPromote(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	source, err := mm.Read(args[0])
	if err != nil {
		return fmt.Errorf("read mote: %w", err)
	}

	// Knowledge types are now global by default — warn about deprecation
	if core.KnowledgeTypes[source.Type] {
		fmt.Fprintf(os.Stderr, "warning: %s motes are now global by default; promote is deprecated for knowledge types\n", source.Type)
	}

	// Warn if the source mote links to project-local motes — those refs won't resolve globally.
	if sourcePrefix := extractMotePrefix(source.ID); sourcePrefix != "" {
		allLinks := collectAllLinks(source)
		for linkType, targets := range allLinks {
			for _, target := range targets {
				if extractMotePrefix(target) == sourcePrefix {
					fmt.Fprintf(os.Stderr, "warning: %s has a project-local link that won't resolve globally: %s -> %s\n", source.ID, linkType, target)
				}
			}
		}
	}

	if source.PromotedTo != "" {
		return fmt.Errorf("mote %s already promoted to %s", source.ID, source.PromotedTo)
	}

	// Ensure global memory directory exists
	gRoot := globalRoot()
	globalNodesDir := filepath.Join(gRoot, "nodes")
	if err := os.MkdirAll(globalNodesDir, 0755); err != nil {
		return fmt.Errorf("create global memory dir: %w", err)
	}

	// Generate global ID
	globalID := core.GenerateID("global", source.Type)

	// Create global mote
	now := time.Now().UTC()
	globalMote := &core.Mote{
		ID:          globalID,
		Type:        source.Type,
		Status:      "active",
		Title:       source.Title,
		Tags:        source.Tags,
		Weight:      source.Weight,
		Origin:      source.Origin,
		CreatedAt:   now,
		AccessCount: 0,
		Body:        source.Body,
	}

	globalData, err := core.SerializeMote(globalMote)
	if err != nil {
		return fmt.Errorf("serialize global mote: %w", err)
	}
	globalPath := filepath.Join(globalNodesDir, globalID+".md")
	if err := core.AtomicWrite(globalPath, globalData, 0644); err != nil {
		return fmt.Errorf("write global mote: %w", err)
	}

	// Update source mote with promotion reference
	source.PromotedTo = globalID
	// Add relates_to link to global mote
	if !sliceContainsStr(source.RelatesTo, globalID) {
		source.RelatesTo = append(source.RelatesTo, globalID)
	}

	sourceData, err := core.SerializeMote(source)
	if err != nil {
		return fmt.Errorf("serialize source mote: %w", err)
	}
	if err := core.AtomicWrite(source.FilePath, sourceData, 0644); err != nil {
		return fmt.Errorf("update source mote: %w", err)
	}

	fmt.Printf("Promoted %s -> %s\n", source.ID, globalID)
	fmt.Printf("  Global: %s\n", globalPath)
	return nil
}

func sliceContainsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
