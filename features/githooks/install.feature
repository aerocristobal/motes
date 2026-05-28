# STORY-HOOKINST-001 — First-class `mote githooks install`
#
# This file is living documentation. Motes does not currently bundle a
# Gherkin runner (godog/cucumber-go); the executable spec for these
# scenarios is the Go test suite — see:
#   internal/githooks/install_test.go         (Scenarios 1, 3, 4, 5, 6)
#   internal/githooks/classify_test.go        (Scenario outline)
#   cmd/mote/cmd_githooks_test.go             (CLI exit-code mapping)
#   cmd/mote/cmd_onboard_githooks_test.go     (Scenario 2)
#   cmd/mote/cmd_doctor_fix_test.go           (Scenario 3, doctor --fix)
#
# The command shipped is `mote githooks install` (not `mote hooks install`)
# to avoid colliding with the pre-existing `mote hook <event>` singular
# command, which emits Claude Code lifecycle reminders. See the resolved
# naming decision in /home/chris/.claude/plans/implement-home-chris-downloads-story-hoo-snazzy-eich.md.

@githooks @onboarding
Feature: First-class `mote githooks install`

  Background:
    Given the `mote` binary is on PATH
    And the current directory is a fresh git repository (`.git/` exists)
    And `.memory/` has been initialized (e.g., by `mote init`)
    And no files exist under `.git/hooks/` other than the default `.sample` files

  Scenario: First-time installation writes embedded hook templates to .git/hooks/
    Given the project has never had mote-managed hooks installed
    When  the developer runs `mote githooks install`
    Then  for each hook event mote ships, an executable script is written to `.git/hooks/<event>`
    And   every script begins with the mote-managed header sentinel (`# managed-by: mote githooks install`)
    And   every script is mode `0755`
    And   the command prints a one-line summary per event installed
    And   the command exits 0

  Scenario: Onboard wires the hooks transparently for new contributors
    Given the developer has just cloned a repository that uses motes
    And   `.memory/` exists in the working tree
    And   `.git/hooks/` contains only the default `.sample` files
    When  the developer runs `mote onboard`
    Then  mote performs its normal onboard steps (index rebuild, skill install, dotfile shims)
    And   mote also installs the embedded git hooks into `.git/hooks/`
    And   the onboard summary lists the hooks that were installed
    And   `mote onboard --dry-run` previews the hook writes without performing them

  Scenario: Re-running install repairs a mote-managed hook that drifted from the embedded template
    Given `.git/hooks/post-checkout` exists, was originally installed by mote, and still carries the sentinel
    But   the file body no longer matches the template embedded in the current `mote` binary
    When  the developer runs `mote githooks install`
    Then  the drifted hook is replaced with the current template
    And   the command output flags the file as "update" rather than "install" or "unchanged"
    And   no other files under `.git/hooks/` are touched
    And   `mote doctor --fix` performs the same repair when invoked without an explicit `mote githooks install`

  Scenario: A user-authored hook without the mote sentinel is left untouched
    Given `.git/hooks/post-checkout` already exists
    And   the file does NOT carry the `managed-by: mote githooks install` sentinel
    When  the developer runs `mote githooks install`
    Then  mote does NOT overwrite the existing file
    And   the command prints a clear warning naming the conflicting file
    And   the warning shows the exact command (`mote githooks install --force`) required to override
    And   the command exits 2 (conflict)
    But   all other mote-managed hooks are installed normally

  Scenario: Running `mote githooks install` outside a git checkout fails cleanly
    Given the current directory contains a `.memory/` directory
    But   the current directory is NOT inside a git working tree
    When  the developer runs `mote githooks install`
    Then  the command writes nothing to disk
    And   stderr contains a single line explaining "not a git repository" and naming the directory checked
    And   the command exits 3
    And   `mote onboard` invoked in the same directory degrades gracefully (hook install skipped with a one-line note)

  Scenario: Dry-run shows the planned hook writes without modifying disk
    Given `.git/hooks/` is in a mixed state (one mote-managed hook drifted, one user-authored hook conflicts, one event missing)
    When  the developer runs `mote githooks install --dry-run`
    Then  for each hook event the planned action is printed (install / update / unchanged / conflict)
    And   no files under `.git/hooks/` are created, modified, or deleted
    And   the command exits 0 even when a conflict would be reported

  Scenario Outline: Hook-state classification drives the install action
    Given `.git/hooks/<event>` is in the <prior_state> state
    When  the developer runs `mote githooks install`
    Then  the action taken is <action>
    And   the command exit code is <exit_code>

    Examples:
      | event          | prior_state                                  | action        | exit_code |
      | post-checkout  | absent                                       | install       | 0         |
      | post-checkout  | mote-managed, matches embedded template      | unchanged     | 0         |
      | post-checkout  | mote-managed, drifted from embedded template | update        | 0         |
      | post-checkout  | user-authored (no mote sentinel)             | conflict      | 2         |
      | post-checkout  | symlink pointing outside the repo            | conflict      | 2         |

  Scenario: Pre-commit template blocks staged edits to derived files
    Given the project is initialized with `mote githooks install`
    And   `.memory/index.jsonl` has been staged for commit
    When  the developer runs `git commit -m "..."`
    Then  the pre-commit hook prints a message explaining the file is derived
    And   the hook tells the developer to run `mote index rebuild`
    And   git commit exits non-zero (commit refused)
    And   the developer can bypass via `git commit --no-verify`
