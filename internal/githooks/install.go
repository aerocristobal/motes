// SPDX-License-Identifier: MIT

package githooks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"motes/internal/core"
	"motes/internal/version"
)

// ActionCode classifies what would happen to a single hook event during
// Install. Each EventReport carries one of these.
type ActionCode string

const (
	ActionInstall   ActionCode = "install"
	ActionUpdate    ActionCode = "update"
	ActionUnchanged ActionCode = "unchanged"
	ActionConflict  ActionCode = "conflict"
)

// Sentinel line prefixes injected after the shebang at install time. The
// classify path looks for these to distinguish mote-managed hooks from
// user-authored ones and to compute drift.
const (
	sentinelManagedBy     = "# managed-by: mote githooks install"
	sentinelVersionPrefix = "# mote-binary-version: "
	sentinelShaPrefix     = "# template-sha256: "
)

// InstallOpts tunes the Install entry point. DryRun classifies every event
// but performs no writes. Force overrides ActionConflict, replacing the
// user-authored file with the embedded template.
type InstallOpts struct {
	DryRun bool
	Force  bool
}

// EventReport is one row in InstallReport.Events, describing what happened
// (or would happen, under DryRun) for a single hook event.
type EventReport struct {
	Event  string
	Path   string
	Action ActionCode
	Reason string
}

// InstallReport is the structured return value of Install. CLI surfaces
// render this as a one-line-per-event summary; tests assert against the
// fields directly.
type InstallReport struct {
	GitDir string
	Events []EventReport
}

// Install plans and (unless DryRun) writes the embedded git-hook templates
// into the repository's .git/hooks/ directory. Returns ErrNotGitRepo when
// no .git directory or gitfile is found in repoRoot or any parent. Returns
// ErrConflict when at least one event was blocked by a user-authored file
// and Force was not set; in that case the report still describes every
// event including the conflicting one, and writes that did succeed are
// preserved.
func Install(repoRoot string, opts InstallOpts) (InstallReport, error) {
	gitDir, err := findGitDir(repoRoot)
	if err != nil {
		return InstallReport{}, err
	}
	hooksDir := filepath.Join(gitDir, "hooks")

	report := InstallReport{GitDir: gitDir}
	var conflict bool

	for _, tpl := range Templates() {
		path := filepath.Join(hooksDir, tpl.Event)
		action, reason, err := classify(path, tpl.Body)
		if err != nil {
			return report, fmt.Errorf("classify %s: %w", tpl.Event, err)
		}
		ev := EventReport{Event: tpl.Event, Path: path, Action: action, Reason: reason}

		switch action {
		case ActionConflict:
			if opts.Force {
				ev.Action = ActionUpdate
				ev.Reason = "overwritten via --force"
				if !opts.DryRun {
					if err := writeRendered(path, tpl.Body, hooksDir); err != nil {
						return report, err
					}
				}
			} else {
				conflict = true
			}
		case ActionInstall, ActionUpdate:
			if !opts.DryRun {
				if err := writeRendered(path, tpl.Body, hooksDir); err != nil {
					return report, err
				}
			}
		case ActionUnchanged:
			// no-op
		}

		report.Events = append(report.Events, ev)
	}

	// DryRun is observation only: never surface ErrConflict, even when one
	// would block a real install. The CLI layer relies on this so --dry-run
	// always exits 0 (Scenario 6).
	if conflict && !opts.DryRun {
		return report, ErrConflict
	}
	return report, nil
}

// findGitDir locates the repository's git directory starting from repoRoot
// and walking up. Supports both classic .git directories and the gitfile
// form (`gitdir: <path>`) used by submodules and worktrees. Returns
// ErrNotGitRepo when no .git is reachable.
func findGitDir(repoRoot string) (string, error) {
	dir, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".git")
		info, statErr := os.Stat(candidate)
		if statErr == nil {
			if info.IsDir() {
				return candidate, nil
			}
			data, readErr := os.ReadFile(candidate)
			if readErr != nil {
				return "", fmt.Errorf("read gitfile %s: %w", candidate, readErr)
			}
			line := strings.TrimSpace(string(data))
			const prefix = "gitdir: "
			if !strings.HasPrefix(line, prefix) {
				return "", fmt.Errorf("%w: malformed gitfile at %s", ErrNotGitRepo, candidate)
			}
			resolved := strings.TrimPrefix(line, prefix)
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(dir, resolved)
			}
			return resolved, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: %s", ErrNotGitRepo, repoRoot)
		}
		dir = parent
	}
}

// writeRendered injects the sentinel header into the embedded body and
// atomically writes the result to path with mode 0755. Creates hooksDir
// if absent. core.AtomicWrite preserves the source perm bits on rename,
// so an explicit Chmod follows to ensure 0755 even when the destination
// previously existed with a different mode.
func writeRendered(path string, body []byte, hooksDir string) error {
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}
	rendered, err := renderTemplate(body)
	if err != nil {
		return err
	}
	if err := core.AtomicWrite(path, rendered, 0o755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("chmod hook: %w", err)
	}
	return nil
}

// renderTemplate inserts the three-line sentinel header immediately after
// the shebang line of the embedded body. The template-sha256 value captures
// the bare embedded body so that classify can later detect drift between
// the installed file and the binary's current template.
func renderTemplate(body []byte) ([]byte, error) {
	nl := bytes.IndexByte(body, '\n')
	if nl < 0 || !bytes.HasPrefix(body, []byte("#!")) {
		return nil, fmt.Errorf("%w: template missing shebang line", ErrTemplateLoad)
	}
	sum := sha256.Sum256(body)
	sentinel := fmt.Sprintf("%s\n%s%s\n%s%s\n",
		sentinelManagedBy,
		sentinelVersionPrefix, version.Value,
		sentinelShaPrefix, hex.EncodeToString(sum[:]),
	)
	var out bytes.Buffer
	out.Grow(len(body) + len(sentinel))
	out.Write(body[:nl+1])
	out.WriteString(sentinel)
	out.Write(body[nl+1:])
	return out.Bytes(), nil
}
