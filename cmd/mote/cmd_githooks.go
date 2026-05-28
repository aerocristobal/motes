// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/githooks"
)

// githooksCmd is the parent for `mote githooks <subcmd>`. The verb is
// distinct from the singular `mote hook` (cmd_hook.go) which emits Claude
// Code lifecycle reminders. See STORY-HOOKINST-001 for the naming rationale.
var githooksCmd = &cobra.Command{
	Use:   "githooks",
	Short: "Manage per-project git hooks shipped with the mote binary",
	Long: `Manage per-project git hooks shipped with the mote binary.

Installs scripts into .git/hooks/ from binary-embedded templates so every
contributor to a project picks up the same lifecycle hooks automatically
(post-checkout context refresh, pre-commit derived-file guard). Hooks
update with the mote binary rather than the project, so a binary upgrade
followed by ` + "`mote githooks install`" + ` (or ` + "`mote doctor --fix`" + `) restores
drifted files to the current template.

This command is distinct from "mote hook" (singular), which emits Claude
Code lifecycle reminders for an active agent session.`,
}

var (
	githooksInstallDryRun bool
	githooksInstallForce  bool
)

var githooksInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install mote's git hooks into .git/hooks/",
	Long: `Install (or repair) mote's git hooks into the current project's
.git/hooks/ directory from binary-embedded templates.

Idempotent — re-run after a mote binary upgrade to pick up template
changes. User-authored hooks are never overwritten without --force.

Exit codes:
  0  success or all hooks already current
  1  generic I/O or template-load failure
  2  conflict with a user-authored hook (use --force to override)
  3  current directory is not inside a git working tree`,
	// Silence cobra's own error/usage banner: the documented exit-code
	// contract means an error return is not a usage mistake, and main.go
	// already renders the error message via *exitCodeError.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runGithooksInstall,
}

func init() {
	githooksInstallCmd.Flags().BoolVar(&githooksInstallDryRun, "dry-run", false,
		"Show planned actions without writing to .git/hooks/")
	githooksInstallCmd.Flags().BoolVar(&githooksInstallForce, "force", false,
		"Overwrite user-authored hooks that lack the mote sentinel")
	githooksCmd.AddCommand(githooksInstallCmd)
	rootCmd.AddCommand(githooksCmd)
}

func runGithooksInstall(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return &exitCodeError{code: 1, err: fmt.Errorf("get working directory: %w", err)}
	}

	opts := githooks.InstallOpts{
		DryRun: githooksInstallDryRun,
		Force:  githooksInstallForce,
	}
	report, installErr := githooks.Install(cwd, opts)

	if errors.Is(installErr, githooks.ErrNotGitRepo) {
		// installErr already wraps the cwd in its message; main.go renders it.
		return &exitCodeError{code: 3, err: installErr}
	}

	printGithooksReport(report, opts)

	if errors.Is(installErr, githooks.ErrConflict) {
		return &exitCodeError{code: 2, err: installErr}
	}
	if installErr != nil {
		return &exitCodeError{code: 1, err: installErr}
	}
	return nil
}

// printGithooksReport renders one line per hook event to stdout. The format
// is human-oriented; CLI consumers that need structured output should call
// githooks.Install directly from Go.
func printGithooksReport(report githooks.InstallReport, opts githooks.InstallOpts) {
	prefix := ""
	if opts.DryRun {
		prefix = "would "
	}
	for _, ev := range report.Events {
		switch ev.Action {
		case githooks.ActionConflict:
			fmt.Printf("  conflict: %s — %s (run `mote githooks install --force` to overwrite)\n",
				ev.Path, ev.Reason)
		case githooks.ActionUnchanged:
			fmt.Printf("  unchanged: %s\n", ev.Path)
		case githooks.ActionInstall:
			fmt.Printf("  %sinstall: %s\n", prefix, ev.Path)
		case githooks.ActionUpdate:
			fmt.Printf("  %supdate: %s\n", prefix, ev.Path)
		}
	}
}
