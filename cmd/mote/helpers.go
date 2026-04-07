// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"motes/internal/core"
	"motes/internal/security"
	"motes/internal/strata"
)

// FlowStats holds inflow/outflow counts for 7d, 30d, and 90d windows.
type FlowStats struct {
	Created7d, Created30d, Created90d          int
	Deprecated7d, Deprecated30d, Deprecated90d int
}

// computeFlowStats counts mote creation and status-transition outflows per time window.
// Outflow counts only motes with a recorded StatusChangedAt; pre-existing motes without
// that field are excluded from deprecated windows (backward-compatible).
func computeFlowStats(motes []*core.Mote) FlowStats {
	now := time.Now()
	var fs FlowStats
	for _, m := range motes {
		created := now.Sub(m.CreatedAt).Hours() / 24
		if created <= 7 {
			fs.Created7d++
		}
		if created <= 30 {
			fs.Created30d++
		}
		if created <= 90 {
			fs.Created90d++
		}
		if m.StatusChangedAt != nil &&
			(m.Status == "deprecated" || m.Status == "archived" || m.Status == "completed") {
			changed := now.Sub(*m.StatusChangedAt).Hours() / 24
			if changed <= 7 {
				fs.Deprecated7d++
			}
			if changed <= 30 {
				fs.Deprecated30d++
			}
			if changed <= 90 {
				fs.Deprecated90d++
			}
		}
	}
	return fs
}

// findMemoryRoot walks cwd upward looking for a .memory/ directory.
func findMemoryRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, ".memory")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .memory/ directory found (run 'mote init' to initialize)")
}

// mustFindRoot returns the .memory/ path or exits with an error message.
func mustFindRoot() string {
	root, err := findMemoryRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	return root
}

// initMemoryDir creates .memory/ and .memory/nodes/ if absent.
func initMemoryDir(root string) error {
	return os.MkdirAll(filepath.Join(root, "nodes"), 0755)
}

// readAllWithGlobal reads project-local motes and merges with global motes.
func readAllWithGlobal(mm *core.MoteManager) ([]*core.Mote, error) {
	return mm.ReadAllWithGlobal()
}

// openEditor opens the given file in $EDITOR (or vi as fallback).
func openEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Validate the editor command for security
	if err := security.ValidateCommand(editor); err != nil {
		return fmt.Errorf("invalid EDITOR command: %w", err)
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// loadMoteBM25 loads the persistent BM25 index from disk.
func loadMoteBM25(root string) (*strata.BM25Index, error) {
	bm25m := core.NewMoteBM25Manager(root)
	data, err := bm25m.LoadRaw()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var idx strata.BM25Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// bm25TextSearcher adapts *strata.BM25Index to core.TextSearcher.
type bm25TextSearcher struct {
	idx *strata.BM25Index
}

func (b *bm25TextSearcher) Search(query string, topK int) []core.TextSearchResult {
	results := b.idx.Search(query, topK)
	out := make([]core.TextSearchResult, len(results))
	for i, r := range results {
		out[i] = core.TextSearchResult{ID: r.Chunk.ID, Score: r.Score}
	}
	return out
}

// loadTextSearcher loads BM25 and wraps it as a core.TextSearcher. Returns nil on error.
func loadTextSearcher(root string) core.TextSearcher {
	idx, err := loadMoteBM25(root)
	if err != nil || idx == nil {
		return nil
	}
	return &bm25TextSearcher{idx: idx}
}

// rebuildMoteBM25 builds a BM25 index from motes and saves it to disk.
func rebuildMoteBM25(root string, motes []*core.Mote) error {
	chunks := make([]strata.Chunk, len(motes))
	for i, m := range motes {
		chunks[i] = strata.Chunk{
			ID:   m.ID,
			Text: m.Title + " " + m.Body,
		}
	}
	idx := strata.BuildBM25Index(chunks)
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	bm25m := core.NewMoteBM25Manager(root)
	return bm25m.SaveRaw(data)
}

// appendAutoLinks BM25-searches allMotes for matches to m's title+body,
// then appends top matches as [[id]] wikilinks to m's body.
func appendAutoLinks(mm *core.MoteManager, m *core.Mote, allMotes []*core.Mote, cfg *core.Config) error {
	if cfg.Linking.MaxAutoLinks <= 0 {
		return nil
	}

	// Build ephemeral BM25 index from all motes
	chunks := make([]strata.Chunk, len(allMotes))
	for i, am := range allMotes {
		chunks[i] = strata.Chunk{
			ID:   am.ID,
			Text: am.Title + " " + am.Body,
		}
	}
	idx := strata.BuildBM25Index(chunks)

	// Search using the new mote's content
	query := m.Title + " " + m.Body
	results := idx.Search(query, cfg.Linking.MaxAutoLinks+1) // +1 to allow for self-filtering

	// Filter: exclude self, below threshold
	var links []string
	for _, r := range results {
		if r.Chunk.ID == m.ID {
			continue
		}
		if r.Score < cfg.Linking.MinScore {
			continue
		}
		links = append(links, r.Chunk.ID)
		if len(links) >= cfg.Linking.MaxAutoLinks {
			break
		}
	}

	if len(links) == 0 {
		return nil
	}

	// Build "See also:" line
	seeAlso := "See also:"
	for _, id := range links {
		seeAlso += " [[" + id + "]]"
	}
	m.Body += "\n" + seeAlso

	// Persist updated body
	data, err := core.SerializeMote(m)
	if err != nil {
		return fmt.Errorf("serialize mote: %w", err)
	}
	path, err := mm.MoteFilePath(m.ID)
	if err != nil {
		return fmt.Errorf("mote path: %w", err)
	}
	return core.AtomicWrite(path, data, 0644)
}
