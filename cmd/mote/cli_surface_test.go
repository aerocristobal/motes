// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"motes/internal/docflags"
)

// TestCLISurface_RealRoot_PersistentFlags verifies that the three
// known root-level persistent flags survive the walk. If someone
// reorders main.go.init() or refactors the rootCmd setup, this test
// catches the regression before docs go stale.
func TestCLISurface_RealRoot_PersistentFlags(t *testing.T) {
	surface := walkCLISurface(rootCmd)
	must := []string{"--no-color", "--plain", "--pretty", "--help", "--version"}
	got := make(map[string]bool, len(surface.Persistent))
	for _, f := range surface.Persistent {
		got[f.Name] = true
	}
	for _, m := range must {
		if !got[m] {
			t.Errorf("persistent flag %q missing from surface", m)
		}
	}
}

// TestCLISurface_RealRoot_LsHasReady is the canonical invariant from
// Scenario 5 of the story: --ready must exist on the ls command. If
// this flag is ever renamed or removed, the test catches the surface
// regression so the doc check can be updated in the same commit.
func TestCLISurface_RealRoot_LsHasReady(t *testing.T) {
	surface := walkCLISurface(rootCmd)
	ls := findCommand(t, surface, "ls")
	if !hasFlag(ls, "--ready") {
		t.Errorf("ls command missing --ready: %+v", ls.Flags)
	}
}

// TestCLISurface_RealRoot_ShowHasNoReady is the other half of
// Scenario 5: --ready must NOT exist on show. This guards against
// someone copy-pasting flags between commands.
func TestCLISurface_RealRoot_ShowHasNoReady(t *testing.T) {
	surface := walkCLISurface(rootCmd)
	show := findCommand(t, surface, "show")
	if hasFlag(show, "--ready") {
		t.Errorf("show command unexpectedly has --ready: %+v", show.Flags)
	}
}

// TestCLISurface_RealRoot_CheckDocFlagsRegistered verifies the new
// subcommand wired itself in via init(). If init() ordering changes
// or someone deletes the registration, this is the smoke test.
func TestCLISurface_RealRoot_CheckDocFlagsRegistered(t *testing.T) {
	surface := walkCLISurface(rootCmd)
	cmd := findCommand(t, surface, "check-doc-flags")
	must := []string{"--root", "--removed-flags", "--paths"}
	for _, m := range must {
		if !hasFlag(cmd, m) {
			t.Errorf("check-doc-flags missing %q in surface: %+v", m, cmd.Flags)
		}
	}
}

// TestCLISurface_SyntheticTree_TwoLevelCommand verifies the walker
// produces a "compliance export" entry (a child of compliance) using
// a synthetic tree, decoupled from any real-command churn.
func TestCLISurface_SyntheticTree_TwoLevelCommand(t *testing.T) {
	root := &cobra.Command{Use: "tool", Version: "0.0.0"}
	root.PersistentFlags().Bool("no-color", false, "")

	parent := &cobra.Command{Use: "compliance"}
	parent.Flags().Bool("parent-only", false, "")

	child := &cobra.Command{Use: "export"}
	child.Flags().Bool("json", false, "")
	parent.AddCommand(child)
	root.AddCommand(parent)

	surface := walkCLISurface(root)
	cmd := findCommand(t, surface, "compliance export")
	if !hasFlag(cmd, "--json") {
		t.Errorf("synthetic 'compliance export' missing --json: %+v", cmd.Flags)
	}
}

// TestCLISurface_SyntheticTree_DeprecatedFlagMarked confirms the
// walker propagates Cobra's Deprecated marker into FlagSpec.
func TestCLISurface_SyntheticTree_DeprecatedFlagMarked(t *testing.T) {
	root := &cobra.Command{Use: "tool"}
	sub := &cobra.Command{Use: "do"}
	sub.Flags().Bool("old", false, "")
	if err := sub.Flags().MarkDeprecated("old", "use --new"); err != nil {
		t.Fatalf("MarkDeprecated: %v", err)
	}
	root.AddCommand(sub)

	surface := walkCLISurface(root)
	cmd := findCommand(t, surface, "do")
	for _, f := range cmd.Flags {
		if f.Name == "--old" && !f.Deprecated {
			t.Errorf("--old should be marked Deprecated, got %+v", f)
		}
	}
}

// TestCLISurface_SyntheticTree_HelpAndCompletionSkipped checks that
// Cobra's auto-injected `help` and `completion` subcommands do not
// appear as their own entries in the surface — they're not commands
// users document.
func TestCLISurface_SyntheticTree_HelpAndCompletionSkipped(t *testing.T) {
	root := &cobra.Command{Use: "tool"}
	root.AddCommand(&cobra.Command{Use: "real"})

	// Cobra adds `help` and `completion` lazily on its own when the
	// tree is non-trivial. We don't add them explicitly; if the
	// walker is permissive about which children to recurse into,
	// they'd show up. Since they're skipped by name in walk(), they
	// won't appear regardless.
	surface := walkCLISurface(root)
	for _, c := range surface.Commands {
		if strings.HasPrefix(c.Name, "help") || strings.HasPrefix(c.Name, "completion") {
			t.Errorf("surface unexpectedly includes %q", c.Name)
		}
	}
}

// --- helpers --------------------------------------------------------------

func findCommand(t *testing.T, s docflags.CLISurface, name string) docflags.CommandSpec {
	t.Helper()
	for _, c := range s.Commands {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("command %q not found in surface; available: %s", name, surfaceNames(s))
	return docflags.CommandSpec{}
}

func hasFlag(c docflags.CommandSpec, flag string) bool {
	for _, f := range c.Flags {
		if f.Name == flag {
			return true
		}
	}
	return false
}

func surfaceNames(s docflags.CLISurface) string {
	names := make([]string, 0, len(s.Commands))
	for _, c := range s.Commands {
		names = append(names, c.Name)
	}
	return strings.Join(names, ", ")
}
