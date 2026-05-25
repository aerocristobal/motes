// SPDX-License-Identifier: MIT
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import motes (and memories) from an export file",
	Long: `Import motes from an export file. Accepts both the JSON envelope
emitted by the current 'mote export' ({"motes":[...],"memories":[...]})
and the prior line-delimited JSONL format. Memories are imported only
from the envelope form.`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
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

	moteEntries, memoryEntries, err := readImportEntries(f)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var created, skipped int
	for _, em := range moteEntries {
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

	memCreated := importMemories(root, memoryEntries, importDryRun)

	if importDryRun {
		fmt.Printf("\nDry run: %d motes would be created, %d duplicates skipped",
			created, skipped)
		if len(memoryEntries) > 0 {
			fmt.Printf("; %d memories would be created", memCreated)
		}
		fmt.Println()
	} else {
		fmt.Printf("Imported %d motes (%d duplicates skipped)", created, skipped)
		if len(memoryEntries) > 0 {
			fmt.Printf("; imported %d memories", memCreated)
		}
		fmt.Println()
		allMotes, _ := mm.ReadAllParallel()
		if allMotes != nil {
			_ = rebuildMoteBM25(root, allMotes)
		}
	}
	return nil
}

// readImportEntries auto-detects the input format. Returns the motes
// and memories present in the file. JSONL inputs (pre-envelope format)
// produce no memory entries.
func readImportEntries(r io.Reader) ([]ExportMote, []ExportMemory, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return nil, nil, nil
	}
	// Envelope detection: a single top-level object whose first key is
	// "motes" or "memories". For JSONL the first object will instead
	// contain mote fields like "id" or "type", and the file will have
	// further objects on subsequent lines that envelope-parsing rejects.
	if trimmed[0] == '{' {
		var env ExportEnvelope
		if err := json.Unmarshal(data, &env); err == nil &&
			(env.Motes != nil || env.Memories != nil) {
			return env.Motes, env.Memories, nil
		}
	}
	// Fall back to JSONL.
	var entries []ExportMote
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
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
		entries = append(entries, em)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return entries, nil, nil
}

// importMemories inserts each memory using Put with NoClobber=false so
// existing keys are overwritten (matches the verb-as-intent default of
// `mote remember`). Returns the count actually persisted.
func importMemories(root string, mems []ExportMemory, dryRun bool) int {
	if len(mems) == 0 {
		return 0
	}
	if dryRun {
		for _, mm := range mems {
			fmt.Printf("[dry-run] would remember: %s\n", mm.Key)
		}
		return len(mems)
	}
	store := core.NewMemoryStore(root)
	actor := core.ResolveAgentID()
	n := 0
	for _, mm := range mems {
		if _, err := store.Put(mm.Key, mm.Body, actor, core.PutOpts{}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: import memory %q: %v\n", mm.Key, err)
			continue
		}
		n++
	}
	return n
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
