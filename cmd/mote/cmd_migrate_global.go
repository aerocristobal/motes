package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var migrateGlobalCmd = &cobra.Command{
	Use:   "migrate-global",
	Short: "Migrate local knowledge motes to global storage",
	Long: `Moves existing project-local knowledge motes (decision, lesson, explore, context, question)
to the global store at ~/.claude/memory/nodes/. Task, constellation, and anchor motes remain local.

Rewrites IDs from project scope to global scope, updates edges in all affected motes,
and leaves forwarding tombstones in the local store. Safe to run multiple times (idempotent).`,
	RunE: runMigrateGlobal,
}

var migrateGlobalDryRun bool

func init() {
	migrateGlobalCmd.Flags().BoolVar(&migrateGlobalDryRun, "dry-run", false, "Show what would be migrated without making changes")
	rootCmd.AddCommand(migrateGlobalCmd)
}

func runMigrateGlobal(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	local, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	// Find knowledge motes eligible for migration (skip already-global, skip tombstones)
	var toMigrate []*core.Mote
	for _, m := range local {
		if strings.HasPrefix(m.ID, "global-") {
			continue
		}
		if m.ForwardedTo != "" {
			continue
		}
		if !core.KnowledgeTypes[m.Type] {
			continue
		}
		toMigrate = append(toMigrate, m)
	}

	if len(toMigrate) == 0 {
		fmt.Println("No knowledge motes to migrate.")
		return nil
	}

	if migrateGlobalDryRun {
		fmt.Printf("Would migrate %d knowledge motes to global storage:\n", len(toMigrate))
		for _, m := range toMigrate {
			fmt.Printf("  %s [%s] %s\n", m.ID, m.Type, m.Title)
		}
		return nil
	}

	// Ensure global dir exists
	gDir, err := core.GlobalNodesDir()
	if err != nil {
		return fmt.Errorf("create global dir: %w", err)
	}

	// Build ID mapping: oldID -> newGlobalID
	idMap := make(map[string]string, len(toMigrate))
	for _, m := range toMigrate {
		newID := core.GenerateID("global", m.Type)
		idMap[m.ID] = newID
	}

	// Rewrite edges in all motes (both those being migrated and those staying local)
	allMotes, _ := mm.ReadAllParallel()
	for _, m := range allMotes {
		if rewriteEdges(m, idMap) {
			data, err := core.SerializeMote(m)
			if err != nil {
				return fmt.Errorf("serialize %s: %w", m.ID, err)
			}
			// Write to current location (local path)
			localPath := filepath.Join(root, "nodes", m.ID+".md")
			if err := core.AtomicWrite(localPath, data, 0644); err != nil {
				return fmt.Errorf("write %s: %w", m.ID, err)
			}
		}
	}

	// Write migrated motes to global and create tombstones
	scope := filepath.Base(filepath.Dir(root))
	migrated := 0
	for _, m := range toMigrate {
		newID := idMap[m.ID]
		oldID := m.ID

		// Update the mote for global storage
		m.ID = newID
		m.OriginProject = scope
		// Rewrite self-edges too
		rewriteEdges(m, idMap)

		// Write to global
		data, err := core.SerializeMote(m)
		if err != nil {
			return fmt.Errorf("serialize global %s: %w", newID, err)
		}
		globalPath := filepath.Join(gDir, newID+".md")
		if err := core.AtomicWrite(globalPath, data, 0644); err != nil {
			return fmt.Errorf("write global %s: %w", newID, err)
		}

		// Write tombstone at old local path
		tombstone := &core.Mote{
			ID:          oldID,
			ForwardedTo: newID,
		}
		tombData, err := core.SerializeMote(tombstone)
		if err != nil {
			return fmt.Errorf("serialize tombstone %s: %w", oldID, err)
		}
		localPath := filepath.Join(root, "nodes", oldID+".md")
		if err := core.AtomicWrite(localPath, tombData, 0644); err != nil {
			return fmt.Errorf("write tombstone %s: %w", oldID, err)
		}

		migrated++
		fmt.Printf("  %s -> %s [%s] %s\n", oldID, newID, m.Type, m.Title)
	}

	// Rebuild local index
	remaining, _ := mm.ReadAllParallel()
	im := core.NewIndexManager(root)
	im.Rebuild(remaining)

	fmt.Printf("Migrated %d knowledge motes to global storage.\n", migrated)
	return nil
}

// rewriteEdges replaces old IDs with new global IDs in all edge fields and body wikilinks.
// Returns true if any changes were made.
func rewriteEdges(m *core.Mote, idMap map[string]string) bool {
	changed := false
	changed = rewriteSlice(&m.DependsOn, idMap) || changed
	changed = rewriteSlice(&m.Blocks, idMap) || changed
	changed = rewriteSlice(&m.RelatesTo, idMap) || changed
	changed = rewriteSlice(&m.BuildsOn, idMap) || changed
	changed = rewriteSlice(&m.Contradicts, idMap) || changed
	changed = rewriteSlice(&m.Supersedes, idMap) || changed
	changed = rewriteSlice(&m.CausedBy, idMap) || changed
	changed = rewriteSlice(&m.InformedBy, idMap) || changed
	if m.Parent != "" {
		if newID, ok := idMap[m.Parent]; ok {
			m.Parent = newID
			changed = true
		}
	}
	// Rewrite [[id]] wikilinks in body
	for oldID, newID := range idMap {
		if strings.Contains(m.Body, "[["+oldID+"]]") {
			m.Body = strings.ReplaceAll(m.Body, "[["+oldID+"]]", "[["+newID+"]]")
			changed = true
		}
	}
	return changed
}

func rewriteSlice(slice *[]string, idMap map[string]string) bool {
	changed := false
	for i, id := range *slice {
		if newID, ok := idMap[id]; ok {
			(*slice)[i] = newID
			changed = true
		}
	}
	return changed
}
