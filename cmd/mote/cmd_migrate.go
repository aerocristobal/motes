// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate <path-to-MEMORY.md>",
	Short: "Convert a MEMORY.md file into typed motes",
	Long: `Parses a MEMORY.md file into sections, infers mote types and origins,
creates individual motes with appropriate links, rebuilds the index,
and archives the original file.`,
	Args: cobra.ExactArgs(1),
	RunE: runMigrate,
}

var migrateDryRun bool

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Show what would be created without writing")
	rootCmd.AddCommand(migrateCmd)
}

type migrateSection struct {
	heading string
	level   int
	body    string
	moteType string
	origin  string
	tags    []string
}

func runMigrate(cmd *cobra.Command, args []string) error {
	memPath := args[0]
	data, err := os.ReadFile(memPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", memPath, err)
	}

	root, err := findMemoryRoot()
	if err != nil {
		cwd, _ := os.Getwd()
		root = filepath.Join(cwd, ".memory")
	}
	if err := initMemoryDir(root); err != nil {
		return fmt.Errorf("init memory dir: %w", err)
	}

	sections := parseSections(string(data))
	if len(sections) == 0 {
		fmt.Println("No sections found in the file.")
		return nil
	}

	fmt.Printf("Found %d sections in %s\n\n", len(sections), memPath)

	if migrateDryRun {
		for _, s := range sections {
			fmt.Printf("  [%s] %s (origin=%s, tags=%v)\n", s.moteType, s.heading, s.origin, s.tags)
		}
		fmt.Printf("\nDry run: would create %d motes.\n", len(sections))
		return nil
	}

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	var created []string
	for _, s := range sections {
		m, err := mm.Create(s.moteType, s.heading, core.CreateOpts{
			Tags:   s.tags,
			Origin: s.origin,
			Body:   strings.TrimSpace(s.body),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create mote for %q: %v\n", s.heading, err)
			continue
		}
		created = append(created, m.ID)
		fmt.Printf("  Created %s [%s] %s\n", m.ID, s.moteType, s.heading)
	}

	// Rebuild index
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	// Archive original
	archivePath := memPath + ".migrated." + time.Now().Format("20060102")
	if err := os.Rename(memPath, archivePath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not archive %s: %v\n", memPath, err)
	} else {
		fmt.Printf("\nArchived original to %s\n", archivePath)
	}

	fmt.Printf("Migration complete: %d motes created.\n", len(created))
	return nil
}

var headingRe = regexp.MustCompile(`^(#{1,4})\s+(.+)$`)

func parseSections(content string) []migrateSection {
	lines := strings.Split(content, "\n")
	var sections []migrateSection
	var current *migrateSection

	for _, line := range lines {
		if match := headingRe.FindStringSubmatch(line); match != nil {
			// Save previous section
			if current != nil && strings.TrimSpace(current.body) != "" {
				sections = append(sections, *current)
			}
			heading := match[2]
			level := len(match[1])
			moteType, origin := inferSectionType(heading)
			tags := inferTags(heading)
			current = &migrateSection{
				heading:  heading,
				level:    level,
				moteType: moteType,
				origin:   origin,
				tags:     tags,
			}
		} else if current != nil {
			current.body += line + "\n"
		}
	}

	// Save last section
	if current != nil && strings.TrimSpace(current.body) != "" {
		sections = append(sections, *current)
	}

	return sections
}

func inferSectionType(heading string) (string, string) {
	h := strings.ToLower(heading)

	// Decision markers
	decisionMarkers := []string{"decision", "chose", "decided", "selected", "architecture"}
	for _, m := range decisionMarkers {
		if strings.Contains(h, m) {
			return "decision", "normal"
		}
	}

	// Lesson markers
	lessonMarkers := []string{"lesson", "learned", "takeaway", "insight", "gotcha", "pitfall"}
	for _, m := range lessonMarkers {
		if strings.Contains(h, m) {
			return "lesson", "normal"
		}
	}

	// Bug/failure markers
	failureMarkers := []string{"bug", "fix", "issue", "problem", "error", "crash", "regression"}
	for _, m := range failureMarkers {
		if strings.Contains(h, m) {
			return "lesson", "failure"
		}
	}

	// Exploration markers
	exploreMarkers := []string{"explore", "research", "investigate", "comparison", "analysis", "benchmark"}
	for _, m := range exploreMarkers {
		if strings.Contains(h, m) {
			return "explore", "normal"
		}
	}

	// Default to context
	return "context", "normal"
}

func inferTags(heading string) []string {
	// Extract meaningful words from heading as tags
	words := strings.Fields(strings.ToLower(heading))
	var tags []string
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "is": true,
		"are": true, "was": true, "were": true, "be": true, "been": true,
		"how": true, "what": true, "when": true, "where": true, "why": true,
	}
	for _, w := range words {
		// Strip non-alphanumeric
		cleaned := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			return -1
		}, w)
		if len(cleaned) > 2 && !stopWords[cleaned] {
			tags = append(tags, cleaned)
		}
	}
	if len(tags) > 5 {
		tags = tags[:5]
	}
	return tags
}
