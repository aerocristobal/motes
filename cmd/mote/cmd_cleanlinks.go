package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var cleanLinksCmd = &cobra.Command{
	Use:   "clean-links",
	Short: "Remove dead link references from mote frontmatter",
	Long: `Scan mote link fields (depends_on, informed_by, etc.) and remove entries
that point to motes which no longer exist.

By default, cross-project references (links to motes in other projects) are
preserved because they cannot be validated without loading those projects.
Use --cross-project to load sibling projects and remove confirmed dead refs.`,
	RunE: runCleanLinks,
}

func init() {
	rootCmd.AddCommand(cleanLinksCmd)
	cleanLinksCmd.Flags().Bool("dry-run", false, "show what would be removed without making changes")
	cleanLinksCmd.Flags().Bool("global", false, "target only global motes (global-* prefix)")
	cleanLinksCmd.Flags().Bool("cross-project", false, "load sibling projects to validate cross-project refs")
	cleanLinksCmd.Flags().String("projects-root", "", "root directory to scan for sibling projects (default: parent of current project)")
}

func runCleanLinks(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	globalOnly, _ := cmd.Flags().GetBool("global")
	crossProject, _ := cmd.Flags().GetBool("cross-project")
	projectsRoot, _ := cmd.Flags().GetString("projects-root")

	motes, err := readAllWithGlobal(mm)
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	knownPrefixes := make(map[string]bool)
	for id := range moteMap {
		if p := extractMotePrefix(id); p != "" {
			knownPrefixes[p] = true
		}
	}

	if crossProject {
		if projectsRoot == "" {
			projectsRoot = filepath.Dir(filepath.Dir(root))
		}
		extra, err := discoverProjectMotes(projectsRoot, root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cross-project scan: %v\n", err)
		}
		for _, m := range extra {
			if _, exists := moteMap[m.ID]; !exists {
				moteMap[m.ID] = m
				if p := extractMotePrefix(m.ID); p != "" {
					knownPrefixes[p] = true
				}
			}
		}
	}

	cleanedMotes := 0
	removedRefs := 0

	for _, m := range motes {
		if globalOnly && !strings.HasPrefix(m.ID, "global-") {
			continue
		}

		removed := 0
		m.DependsOn = filterDeadLinks(m.ID, "depends_on", m.DependsOn, moteMap, knownPrefixes, crossProject, dryRun, &removed)
		m.Blocks = filterDeadLinks(m.ID, "blocks", m.Blocks, moteMap, knownPrefixes, crossProject, dryRun, &removed)
		m.RelatesTo = filterDeadLinks(m.ID, "relates_to", m.RelatesTo, moteMap, knownPrefixes, crossProject, dryRun, &removed)
		m.BuildsOn = filterDeadLinks(m.ID, "builds_on", m.BuildsOn, moteMap, knownPrefixes, crossProject, dryRun, &removed)
		m.Contradicts = filterDeadLinks(m.ID, "contradicts", m.Contradicts, moteMap, knownPrefixes, crossProject, dryRun, &removed)
		m.Supersedes = filterDeadLinks(m.ID, "supersedes", m.Supersedes, moteMap, knownPrefixes, crossProject, dryRun, &removed)
		m.CausedBy = filterDeadLinks(m.ID, "caused_by", m.CausedBy, moteMap, knownPrefixes, crossProject, dryRun, &removed)
		m.InformedBy = filterDeadLinks(m.ID, "informed_by", m.InformedBy, moteMap, knownPrefixes, crossProject, dryRun, &removed)

		if removed == 0 {
			continue
		}
		removedRefs += removed
		cleanedMotes++

		if dryRun {
			continue
		}
		data, err := core.SerializeMote(m)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: serialize %s: %v\n", m.ID, err)
			continue
		}
		if err := core.AtomicWrite(m.FilePath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write %s: %v\n", m.ID, err)
		}
	}

	if dryRun {
		fmt.Printf("Dry run: %d ref(s) in %d mote(s) would be removed.\n", removedRefs, cleanedMotes)
	} else if cleanedMotes > 0 {
		fmt.Printf("Removed %d dead ref(s) from %d mote(s).\n", removedRefs, cleanedMotes)
		fmt.Println("Run `mote index rebuild` to refresh the graph.")
	} else {
		fmt.Println("No dead links found.")
	}
	return nil
}

// filterDeadLinks removes entries from targets that are confirmed dead (don't exist
// in moteMap and can be validated as missing). Cross-project refs with an unknown
// prefix are preserved unless crossProject mode is enabled.
func filterDeadLinks(
	moteID, linkType string,
	targets []string,
	moteMap map[string]*core.Mote,
	knownPrefixes map[string]bool,
	crossProject, dryRun bool,
	removedCount *int,
) []string {
	if len(targets) == 0 {
		return targets
	}
	kept := make([]string, 0, len(targets))
	for _, target := range targets {
		if _, ok := moteMap[target]; ok {
			kept = append(kept, target)
			continue
		}
		prefix := extractMotePrefix(target)
		if !crossProject && prefix != "" && !knownPrefixes[prefix] {
			// Unknown project prefix — can't confirm dead without --cross-project, keep it.
			kept = append(kept, target)
			continue
		}
		// Confirmed dead: known prefix but missing, or cross-project validated and still absent.
		if dryRun {
			fmt.Printf("  would remove %s.%s -> %s\n", moteID, linkType, target)
		} else {
			fmt.Printf("  removing %s.%s -> %s\n", moteID, linkType, target)
		}
		*removedCount++
	}
	return kept
}
