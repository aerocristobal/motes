// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// motesOwnedRelPaths lists relative paths under the global memory root that
// motes owns. Anything not on this list (MEMORY.md, top-level *.md from
// Claude's auto-memory, .obsidian/, *.canvas, etc.) is left untouched during
// legacy-to-new migration.
var motesOwnedRelPaths = []string{
	"nodes",
	"dream",
	"strata",
	"index.jsonl",
	"constellations.jsonl",
	"dream_quality.jsonl",
	"config.yaml",
}

// migrationMarkerName is the filename written into the destination root when
// a legacy-path migration completes. Its presence short-circuits subsequent
// migration attempts.
const migrationMarkerName = ".migrated_from_legacy"

// MigrateLegacyGlobal moves motes-owned files from a legacy global memory
// directory (e.g. ~/.claude/memory/) to a new motes home (e.g. ~/.motes/).
//
// Only entries listed in motesOwnedRelPaths are moved; everything else
// (notably Claude Code's auto-memory MEMORY.md and top-level *.md notes) is
// intentionally preserved at the legacy location.
//
// The function is idempotent: if the destination already carries the
// migration marker, it returns (false, nil) immediately. If a destination
// path already exists, that entry is skipped to avoid clobbering newer data.
//
// On cross-device renames it falls back to copy-then-delete.
func MigrateLegacyGlobal(legacyRoot, newRoot string) (bool, error) {
	if err := os.MkdirAll(newRoot, 0755); err != nil {
		return false, fmt.Errorf("create new root: %w", err)
	}

	markerPath := filepath.Join(newRoot, migrationMarkerName)
	if _, err := os.Stat(markerPath); err == nil {
		return false, nil
	}

	moved := 0
	for _, rel := range motesOwnedRelPaths {
		src := filepath.Join(legacyRoot, rel)
		dst := filepath.Join(newRoot, rel)

		info, err := os.Stat(src)
		if err != nil {
			continue // source doesn't exist; nothing to move
		}
		if _, err := os.Stat(dst); err == nil {
			continue // destination already populated; don't clobber
		}

		if err := movePath(src, dst, info.IsDir()); err != nil {
			return false, fmt.Errorf("move %s: %w", rel, err)
		}
		moved++
	}

	if err := os.WriteFile(markerPath, []byte("migrated\n"), 0644); err != nil {
		return false, fmt.Errorf("write migration marker: %w", err)
	}

	if moved > 0 {
		fmt.Fprintf(os.Stderr, "motes: migrated %d entries from %s to %s\n", moved, legacyRoot, newRoot)
	}
	return moved > 0, nil
}

// movePath relocates src to dst, falling back from rename to copy+delete on
// cross-device errors.
func movePath(src, dst string, isDir bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !isCrossDeviceErr(err) {
		return err
	}
	if isDir {
		if err := copyDirContents(src, dst); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}
	if err := copyFileContents(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func isCrossDeviceErr(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		var errno syscall.Errno
		if errors.As(linkErr.Err, &errno) {
			return errno == syscall.EXDEV
		}
	}
	return false
}

func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyDirContents(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFileContents(path, target)
	})
}
