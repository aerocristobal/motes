package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motes/internal/core"
)

// runMigrateMarkdown migrates MEMORY.md sections into motes and archives the file.
func runMigrateMarkdown(mm *core.MoteManager, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	sections := parseSections(string(data))
	if len(sections) == 0 {
		return 0, nil
	}

	fmt.Printf("Migrating %s (%d sections)...\n", filepath.Base(path), len(sections))
	var created int
	for _, s := range sections {
		m, err := mm.Create(s.moteType, s.heading, core.CreateOpts{
			Tags:   s.tags,
			Origin: s.origin,
			Body:   strings.TrimSpace(s.body),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: %q: %v\n", s.heading, err)
			continue
		}
		created++
		fmt.Printf("  created %s [%s] %s\n", m.ID, s.moteType, s.heading)
	}

	// Archive original
	archivePath := path + ".migrated." + time.Now().Format("20060102")
	if err := os.Rename(path, archivePath); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not archive: %v\n", err)
	} else {
		fmt.Printf("  archived to %s\n", filepath.Base(archivePath))
	}

	return created, nil
}
