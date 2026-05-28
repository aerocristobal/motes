// SPDX-License-Identifier: MIT

package githooks

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
)

// ClassifyHookState inspects the file at path and reports the install action
// that would be taken given the embedded template body. Read-only: never
// modifies the file.
func ClassifyHookState(path string, embedded []byte) (ActionCode, error) {
	action, _, err := classify(path, embedded)
	return action, err
}

func classify(path string, embedded []byte) (ActionCode, string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ActionInstall, "", nil
	}
	if err != nil {
		return "", "", err
	}
	// Symlinks under .git/hooks/ are never something mote installed. The
	// story flags "symlink pointing outside the repo" as conflict; we
	// generalize to all symlinks because mote's writer would never produce
	// one, so any symlink is by definition foreign.
	if info.Mode()&os.ModeSymlink != 0 {
		return ActionConflict, "symlink (refusing to write through)", nil
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if !hasMoteSentinel(existing) {
		return ActionConflict, "user-authored (no mote sentinel)", nil
	}
	stripped := stripSentinel(existing)
	if sha256.Sum256(stripped) == sha256.Sum256(embedded) {
		return ActionUnchanged, "", nil
	}
	return ActionUpdate, "drifted from embedded template", nil
}

// hasMoteSentinel returns true when the script contains the managed-by
// sentinel line within its first ~10 lines. Scanning a window rather than
// requiring an exact position keeps detection robust against future template
// layouts that add lines before the sentinel.
func hasMoteSentinel(content []byte) bool {
	lines := bytes.SplitN(content, []byte("\n"), 11)
	for _, l := range lines {
		if bytes.HasPrefix(l, []byte(sentinelManagedBy)) {
			return true
		}
	}
	return false
}

// stripSentinel removes any line whose prefix matches one of the three
// sentinel forms injected by renderTemplate. The remainder must equal the
// bare embedded body when the file has not drifted.
func stripSentinel(content []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	out := lines[:0]
	for _, l := range lines {
		switch {
		case bytes.HasPrefix(l, []byte(sentinelManagedBy)):
		case bytes.HasPrefix(l, []byte(sentinelVersionPrefix)):
		case bytes.HasPrefix(l, []byte(sentinelShaPrefix)):
		default:
			out = append(out, l)
		}
	}
	return bytes.Join(out, []byte("\n"))
}
