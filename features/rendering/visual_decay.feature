# STORY-DECAY-001 — Visual decay: closed items mute the entire row
#
# This file is living documentation. Motes does not bundle a Gherkin runner;
# the executable spec for these scenarios is the Go test suite — see:
#   cmd/mote/cmd_ls_decay_test.go         (Scenarios 1–7, 9–10)
#   cmd/mote/cmd_context_decay_test.go    (Scenario 8)
#   internal/format/style_test.go         (unit tests behind the renderer)
#
# Closed status set: {completed, archived, deprecated} → muted.
# Live status set:   {active, in_progress} → normal.
# JSON output and non-TTY output are byte-stable; the `[deprecated]` text
# marker is preserved alongside the new muting so existing log scrapers
# continue to work.

@rendering @ux @decay
Feature: Visual decay for closed motes

  Background:
    Given a mote-initialized repository
    And stdout is connected to a TTY
    And the NO_COLOR environment variable is unset

  Scenario: Mixed active and completed motes render with different styles
    Given the following motes exist:
      | id          | status     | title              |
      | proj-T1ABC  | active     | Add login flow     |
      | proj-T2DEF  | completed  | Refactor router    |
      | proj-T3GHI  | active     | Wire OAuth         |
    When the agent runs:  mote ls
    Then the row for "proj-T1ABC" is rendered in the active style
    And the row for "proj-T2DEF" is rendered in the muted style
    And the row for "proj-T3GHI" is rendered in the active style
    And the column alignment is preserved across all rows

  Scenario Outline: Every closed status produces a muted row
    Given a mote "<id>" exists with status "<status>"
    When the agent runs:  mote ls
    Then the row for "<id>" is rendered in the muted style

    Examples:
      | id         | status     |
      | proj-X1AAA | completed  |
      | proj-X1BBB | archived   |
      | proj-X1CCC | deprecated |

  Scenario: Piping mote ls to a file emits no ANSI escapes
    Given the following motes exist:
      | id         | status     | title          |
      | proj-T1ABC | active     | Add login flow |
      | proj-T2DEF | completed  | Refactor router|
    When the agent runs:  mote ls > /tmp/mote-ls.out
    Then the contents of /tmp/mote-ls.out contain no ANSI escape sequences
    And the output is byte-identical to the pre-feature output for the same fixture

  Scenario: NO_COLOR environment variable disables muting even on a TTY
    Given stdout is connected to a TTY
    And the NO_COLOR environment variable is set to "1"
    And mixed active and completed motes exist
    When the agent runs:  mote ls
    Then the output contains no ANSI escape sequences
    And the closed motes still show the "[deprecated]" textual marker (backward-compatible)

  Scenario: Explicit --no-color flag suppresses muting on a TTY
    Given stdout is connected to a TTY
    And the NO_COLOR env var is unset
    When the agent runs:  mote ls --no-color
    Then the output contains no ANSI escape sequences

  Scenario: JSON output never contains ANSI escapes or status decoration
    Given mixed active and completed motes exist
    And stdout is connected to a TTY
    When the agent runs:  mote ls --json
    Then the output is valid JSON
    And the output contains no ANSI escape sequences
    And the output is byte-identical to the pre-feature output for the same fixture

  Scenario: mote pulse default filter excludes closed motes; muting has no visible effect
    Given the following motes exist:
      | id         | status     | type | title          |
      | proj-T1ABC | active     | task | Add login flow |
      | proj-T2DEF | completed  | task | Refactor router|
    When the agent runs:  mote pulse
    Then the output contains the row for "proj-T1ABC"
    And the output does NOT contain the row for "proj-T2DEF"
    And the visible row uses the active style

  Scenario: Planning view recedes resolved prerequisites
    Given a task mote "proj-T1ABC" has these dependencies:
      | id         | status     | title           |
      | proj-T0AAA | completed  | Database schema |
      | proj-T0BBB | active     | Auth middleware |
    When the agent runs:  mote context --planning proj-T1ABC
    Then the dependency entry for "proj-T0AAA" is rendered in the muted style
    And the dependency entry for "proj-T0BBB" is rendered in the active style

  Scenario: Deprecated motes keep the [deprecated] text prefix AND get muted
    Given a deprecated mote "proj-D1XYZ" exists
    When the agent runs:  mote ls (on a TTY, with color enabled)
    Then the row contains the substring "[deprecated]"
    And the entire row is rendered in the muted style

  Scenario: ANSI escapes do not break tab/column alignment
    Given five motes exist with titles of varying lengths and mixed statuses
    When the agent runs:  mote ls
    Then visually inspecting the output (or comparing column positions after
      stripping ANSI escapes), all column edges align across rows
