# Living documentation for STORY-SHOW-001.
# Executable specs are in cmd/mote/cmd_show_short_test.go,
# cmd/mote/cmd_show_long_test.go, cmd/mote/cmd_show_flags_test.go,
# cmd/mote/cmd_show_snapshot_test.go, and internal/format/icon_test.go.

@show @density
Feature: Progressive disclosure for mote show
  In order to scan many ready motes in a loop AND debug a single mote in depth
  As a coding agent (Claude Code, Codex, Gemini CLI)
  I want one-line --short and forensic --long modes alongside the default

  Background:
    Given a mote-initialized repository
    And a task mote "proj-T1ABC" exists with title "Add login flow"
      and status "active", weight 0.5, and one acceptance criterion

  Scenario: Default mote show output is byte-stable across the release
    When the agent runs:  mote show proj-T1ABC
    Then the output equals the pre-change snapshot for "proj-T1ABC"
    And the command exits with code 0

  Scenario: One-line short form for loop consumption
    When the agent runs:  mote show proj-T1ABC --short
    Then the output is exactly one non-blank line
    And the line begins with a status icon
    And the line contains the mote ID "proj-T1ABC"
    And the line contains the weight rendered to two decimal places
    And the line contains the type "task"
    And the line ends with the (possibly truncated) title
    And the command exits with code 0

  Scenario: Long form adds every internal-state field
    Given the mote "proj-T1ABC" has access_count 7, last_accessed yesterday,
      and a non-empty audit history
    When the agent runs:  mote show proj-T1ABC --long
    Then the default output is present unchanged
    And the output additionally contains an "--- internal state ---" section
    And the section includes the audit-log path and entry count
    And the section includes the last successful mote prime timestamp (if any)
    And the section includes the global-promotion status (if global)
    And the command exits with code 0

  Scenario Outline: Status icon in short mode reflects status
    Given a mote with status "<status>" exists
    When the agent runs:  mote show <id> --short
    Then the leading icon equals "<icon>"

    Examples:
      | status      | icon | rationale                          |
      | active      | ○    | open work, not started             |
      | in_progress | ◐    | partial progress                   |
      | completed   | ✓    | done                               |
      | archived    | ●    | closed but kept for record         |
      | deprecated  | ❄    | superseded; visually receded       |

  Scenario: Combining --short and --long is a configuration error
    When the agent runs:  mote show proj-T1ABC --short --long
    Then the command exits with a non-zero code
    And stderr contains "--short and --long are mutually exclusive"
    But no access-batch entry is appended (no side effect)

  Scenario: Short JSON shape is the minimal field set
    When the agent runs:  mote show proj-T1ABC --short --json
    Then the JSON output contains exactly these top-level keys:
      | id     |
      | status |
      | type   |
      | weight |
      | title  |
    And the command exits with code 0

  Scenario: Long JSON shape strictly extends the default JSON shape
    When the agent runs:  mote show proj-T1ABC --json
    And the agent then runs:  mote show proj-T1ABC --long --json
    Then every key in the first output is present in the second
    And the second output contains additional keys for: last_prime_at,
      audit_log_entries_count, deprecation_chain (when applicable),
      strata_corpus, and promoted_to (when applicable)

  Scenario Outline: Unknown mote ID error is mode-independent
    When the agent runs:  mote show no-such-id <flag>
    Then the command exits with code 1
    And stderr contains "mote not found: no-such-id"
    And nothing is written to stdout

    Examples:
      | flag           |
      |                |
      | --short        |
      | --long         |
      | --short --json |
      | --long --json  |

  Scenario: Bulk short calls do not cause access-batch contention
    Given 20 active task motes exist
    When the agent runs (in a single shell loop):
      for id in $(mote ls --ready --compact | cut -d: -f1); do mote show $id --short; done
    Then every iteration exits with code 0
    And the access batch file is NOT updated for any mote (--short is loop-pure)
    And no filesystem lock contention error is logged

  Scenario: NO_UNICODE env var forces ASCII icons
    Given NO_UNICODE is set to "1"
    When the agent runs:  mote show proj-T1ABC --short
    Then the leading icon is the ASCII glyph "o"

  Scenario: --ascii flag forces ASCII icons
    When the agent runs:  mote show proj-T1ABC --short --ascii
    Then the leading icon is the ASCII glyph "o"
