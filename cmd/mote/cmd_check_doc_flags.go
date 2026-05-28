// SPDX-License-Identifier: MIT
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/docflags"
)

// STORY-DOCFLAGS-001: `mote check-doc-flags` is the runnable surface
// of the doc-flag-freshness gate. It walks its own Cobra tree, scans
// the doc tree for `mote <cmd> --<flag>` references, and exits
// non-zero when a reference does not resolve to a live flag (or to
// an allowlisted historical-context reference).
//
// The subcommand is wired into rootCmd for two reasons: (1) the
// Cobra tree is package-private, so introspection has to happen
// from inside cmd/mote, and (2) shipping the check as a real
// subcommand makes it agent-native — any agent or contributor can
// run `mote check-doc-flags` locally without consulting CI.

var (
	docFlagsRoot        string
	docFlagsRemovedFile string
	docFlagsPaths       []string
)

var checkDocFlagsCmd = &cobra.Command{
	Use:           "check-doc-flags",
	Short:         "Verify every mote <command> --<flag> reference in docs resolves to a live flag",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Scans the documentation tree for references of the form
"mote <command> --<flag>" and fails when any reference does not match
the live Cobra command tree.

Default doc scope: README.md, AGENTS.md, CLAUDE.md, CODEX.md, GEMINI.md,
and every docs/*.md except docs/example-*-config.md (those are config
samples, not flag examples). Use --paths to override.

A reference to a known-removed flag (listed in scripts/removed-flags.txt)
is allowlisted when its nearest H2 heading OR its enclosing paragraph
case-insensitively contains one of: changelog, removed, deprecated,
historical. The HTML comment <!-- doc-flags: ignore-next --> suppresses
the next non-blank line (or, when that line opens a fenced code block,
the whole block).

Exit codes:
  0  every reference is valid
  1  one or more violations
  2  operational error (missing file, unreadable manifest)`,
	RunE: runCheckDocFlags,
}

func init() {
	checkDocFlagsCmd.Flags().StringVar(&docFlagsRoot, "root", ".",
		"Repository root to scan")
	checkDocFlagsCmd.Flags().StringVar(&docFlagsRemovedFile, "removed-flags",
		"scripts/removed-flags.txt",
		"Path (relative to --root) to the known-removed-flags manifest")
	checkDocFlagsCmd.Flags().StringSliceVar(&docFlagsPaths, "paths", nil,
		"Comma-separated override of the default doc scope (paths relative to --root)")
	rootCmd.AddCommand(checkDocFlagsCmd)
}

func runCheckDocFlags(cmd *cobra.Command, args []string) error {
	repoRoot, err := filepath.Abs(docFlagsRoot)
	if err != nil {
		return &exitCodeError{code: 2, err: fmt.Errorf("resolve --root: %w", err)}
	}

	paths := docFlagsPaths
	if len(paths) == 0 {
		paths, err = defaultDocPaths(repoRoot)
		if err != nil {
			return &exitCodeError{code: 2, err: err}
		}
	}

	surface := walkCLISurface(rootCmd)

	removed, err := docflags.LoadRemoved(filepath.Join(repoRoot, docFlagsRemovedFile))
	if err != nil {
		return &exitCodeError{code: 2, err: err}
	}

	refs, fileLines, err := docflags.Scan(repoRoot, paths)
	if err != nil {
		return &exitCodeError{code: 2, err: err}
	}

	violations := docflags.Check(refs, surface, removed, fileLines, docflags.DefaultTokens)

	if len(violations) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"doc-flags: %d reference(s) checked across %d file(s); no violations\n",
			len(refs), len(paths))
		return nil
	}

	for _, v := range violations {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s:%d: mote %s %s (%s)\n",
			v.Reference.Path, v.Reference.Line,
			v.Reference.Command, v.Reference.Flag, v.Reason)
	}
	return &exitCodeError{
		code: 1,
		err: fmt.Errorf("doc-flags: %d violation(s) across %d reference(s) in %d file(s)",
			len(violations), len(refs), len(paths)),
	}
}

// defaultDocPaths assembles the standard scope: every top-level
// agent-instruction file that exists, plus docs/*.md except the
// example-*-config samples. Paths are returned relative to repoRoot.
func defaultDocPaths(repoRoot string) ([]string, error) {
	var paths []string
	for _, p := range []string{"README.md", "AGENTS.md", "CLAUDE.md", "CODEX.md", "GEMINI.md"} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); err == nil {
			paths = append(paths, p)
		}
	}
	matches, err := filepath.Glob(filepath.Join(repoRoot, "docs", "*.md"))
	if err != nil {
		return nil, fmt.Errorf("glob docs: %w", err)
	}
	sort.Strings(matches)
	for _, m := range matches {
		base := filepath.Base(m)
		if strings.HasPrefix(base, "example-") && strings.HasSuffix(base, "-config.md") {
			continue
		}
		rel, err := filepath.Rel(repoRoot, m)
		if err != nil {
			return nil, fmt.Errorf("relativize %s: %w", m, err)
		}
		paths = append(paths, rel)
	}
	return paths, nil
}
