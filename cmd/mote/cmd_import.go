// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var importCmd = &cobra.Command{
	Use:   "import <file.jsonl>",
	Short: "Import motes from JSONL file",
	Args:  cobra.ExactArgs(1),
	RunE:  runImport,
}

var importDryRun bool

func init() {
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Preview import without writing")
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	// Build content hash set of existing motes for dedup
	existing, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read existing motes: %w", err)
	}
	existingHashes := make(map[string]bool, len(existing))
	for _, m := range existing {
		existingHashes[moteContentHash(m)] = true
	}

	f, err := os.Open(args[0])
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var created, skipped int
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB line buffer
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var em ExportMote
		if err := json.Unmarshal([]byte(line), &em); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skip invalid line: %v\n", err)
			continue
		}

		hash := exportMoteContentHash(&em)
		if existingHashes[hash] {
			skipped++
			continue
		}

		if importDryRun {
			fmt.Printf("[dry-run] would create: %s %q\n", em.Type, em.Title)
			created++
			continue
		}

		m, err := mm.Create(em.Type, em.Title, core.CreateOpts{
			Tags:       em.Tags,
			Weight:     em.Weight,
			Origin:     em.Origin,
			Body:       em.Body,
			Parent:     em.Parent,
			Acceptance: em.Acceptance,
			Size:       em.Size,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: create failed for %q: %v\n", em.Title, err)
			continue
		}

		// Set external refs if present
		if len(em.ExternalRefs) > 0 {
			m.ExternalRefs = em.ExternalRefs
			data, err := core.SerializeMote(m)
			if err == nil {
				path, _ := mm.MoteFilePath(m.ID)
				_ = core.AtomicWrite(path, data, 0644)
			}
		}

		existingHashes[hash] = true
		created++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if importDryRun {
		fmt.Printf("\nDry run: %d would be created, %d duplicates skipped\n", created, skipped)
	} else {
		fmt.Printf("Imported %d motes (%d duplicates skipped)\n", created, skipped)
		// Rebuild BM25 index
		allMotes, _ := mm.ReadAllParallel()
		if allMotes != nil {
			_ = rebuildMoteBM25(root, allMotes)
		}
	}
	return nil
}

func moteContentHash(m *core.Mote) string {
	h := sha256.New()
	h.Write([]byte(m.Type))
	h.Write([]byte(m.Title))
	h.Write([]byte(m.Body))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func exportMoteContentHash(em *ExportMote) string {
	h := sha256.New()
	h.Write([]byte(em.Type))
	h.Write([]byte(em.Title))
	h.Write([]byte(em.Body))
	return fmt.Sprintf("%x", h.Sum(nil))
}
