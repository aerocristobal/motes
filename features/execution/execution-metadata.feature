# STORY-EXEC-001 — Execution metadata as orchestration hints
#
# This file is living documentation. Motes does not currently bundle a
# Gherkin runner (godog/cucumber-go); the executable spec for these
# scenarios is the Go test suite — see:
#   internal/core/mote_execution_test.go              (YAML round-trip, manager set/clear/audit, change_after_launch)
#   internal/security/validate_execution_test.go      (validation: enums, free-form, length, bidi, adversarial, fuzz)
#   cmd/mote/cmd_add_execution_test.go                (--execution-* flags on `mote add`)
#   cmd/mote/cmd_update_execution_test.go             (--execution-* flags on `mote update`, clear-with-empty, mutex with --claim, warning)
#   cmd/mote/cmd_show_execution_test.go               (text section order, JSON key order, omission when unset)
#   cmd/mote/cmd_promote_execution_test.go            (execution_* stripped on promote to global)
#
# The schema is intentionally flat (top-level frontmatter keys, matching
# beads' shape). The orchestrator reads these BEFORE dispatching a subagent;
# a running subagent cannot change its own model or reasoning effort, so the
# first read decides everything. STORY-EREAD-001 owns the "must read before
# launch" contract; STORY-MQRY-001 owns the query surface.

@mote @execution @schema
Feature: Execution metadata on motes

  Background:
    Given the agent ID is set via `MOTE_AGENT_ID=agent-orchestrator`

  # ---- Field declaration & round-trip ----

  Scenario: All five execution fields round-trip through YAML
    When I run:
      """
      mote add --type=task --title="parallel job 1" \
        --execution-agent-type=mote-subagent \
        --execution-suggested-model=haiku \
        --execution-reasoning-effort=low \
        --execution-mode=parallel \
        --execution-parallel-group=group-A
      """
    Then the new mote's frontmatter contains:
      | field                       | value          |
      | execution_agent_type        | mote-subagent  |
      | execution_suggested_model   | haiku          |
      | execution_reasoning_effort  | low            |
      | execution_mode              | parallel       |
      | execution_parallel_group    | group-A        |
    And running `mote show <id> --json` returns those same values under the same keys

  Scenario: A mote with no execution metadata writes none of the fields
    When I run `mote add --type=task --title="ordinary task"`
    Then the new mote's frontmatter contains no `execution_*` fields
    And `mote show <id> --json` returns no `execution_*` keys in the output

  Scenario: Update an existing mote's execution metadata
    Given mote `motes-abc` exists with no execution metadata
    When I run `mote update motes-abc --execution-mode=delegated --execution-suggested-model=sonnet`
    Then `motes-abc.execution_mode` is `delegated`
    And `motes-abc.execution_suggested_model` is `sonnet`

  Scenario: Clear a single execution field with empty-string value
    Given mote `motes-abc` has `execution_parallel_group=group-A`
    When I run `mote update motes-abc --execution-parallel-group=""`
    Then `motes-abc.execution_parallel_group` is removed from frontmatter

  # ---- Validation: enums ----

  Scenario Outline: Valid `execution_mode` values
    When I run `mote add --type=task --title=t --execution-mode=<mode>`
    Then the command exits 0
    And `execution_mode` equals `<mode>`

    Examples:
      | mode       |
      | local      |
      | delegated  |
      | parallel   |

  Scenario: Invalid `execution_mode` is rejected
    When I run `mote add --type=task --title=t --execution-mode=fire_and_forget`
    Then the command exits non-zero
    And stderr contains "invalid execution_mode"
    And lists the valid values: local, delegated, parallel
    And no mote is created

  Scenario Outline: Valid `execution_reasoning_effort` values
    When I run `mote add --type=task --title=t --execution-reasoning-effort=<effort>`
    Then the command exits 0

    Examples:
      | effort  |
      | low     |
      | medium  |
      | high    |

  Scenario: Invalid `execution_reasoning_effort` is rejected
    When I run `mote add --type=task --title=t --execution-reasoning-effort=maximum`
    Then the command exits non-zero
    And stderr contains "invalid execution_reasoning_effort"

  # ---- Validation: free-form fields ----

  Scenario: `execution_suggested_model` accepts arbitrary alphanumeric + hyphen tokens
     When I run `mote add --type=task --title=t --execution-suggested-model=claude-sonnet-5`
    Then the command exits 0

  Scenario Outline: `execution_*` free-form fields reject shell metacharacters and traversal
    When I run `mote add --type=task --title=t --execution-agent-type=<value>`
    Then the command exits non-zero
    And stderr contains "invalid execution_agent_type"
    And no mote is created

    Examples:
      | value             |
      | $(rm -rf ~)       |
      | ../../etc/passwd  |
      | foo;bar           |
      | foo\nbar          |
      | foo`bar`          |

  Scenario: `execution_parallel_group` length is bounded
    When I run `mote add --type=task --title=t --execution-parallel-group=<a 257-character string>`
    Then the command exits non-zero
    And stderr contains "execution_parallel_group too long"

  Scenario: Unicode bidi-override characters in execution fields are rejected
    When I run `mote add --type=task --title=t --execution-agent-type=<string containing U+202E>`
    Then the command exits non-zero
    And stderr contains "invalid"

  # ---- Display: surfaced ahead of body in `show` ----

  Scenario: `mote show` surfaces execution metadata before the body
    Given mote `motes-abc` has all five execution fields set
    When I run `mote show motes-abc`
    Then the rendered output contains a section titled "execution"
    And the "execution" section appears before the "body" section

  Scenario: `mote show --json` places `execution_*` keys ahead of `body` in output ordering
    Given mote `motes-abc` has all five execution fields set
    When I run `mote show motes-abc --json`
    Then the JSON output has all five `execution_*` keys
    And each `execution_*` key appears before `body` in the serialized JSON

  Scenario: `mote show` on a mote with no execution metadata has no "execution" section
    Given mote `motes-xyz` has no `execution_*` fields
    When I run `mote show motes-xyz`
    Then the rendered output contains no "execution" section header
    And `mote show motes-xyz --json` returns no `execution_*` keys

  # ---- Provenance / immutability after launch ----

  Scenario: A subagent CANNOT silently rewrite execution metadata
    Given mote `motes-abc` has `execution_suggested_model=haiku`
    And the mote is currently claimed
    When I run `mote update motes-abc --execution-suggested-model=opus`
    Then the command exits 0 but emits a warning to stderr:
      "warning: changing execution metadata after dispatch has no effect on the running subagent"
    And the audit log records the change with `change_after_launch: true`

  # ---- Promote / crystallize strip execution metadata ----

  Scenario: Promoting a mote to global strips execution metadata
    Given mote `motes-abc` is a lesson with all five execution fields set
    When I run `mote promote motes-abc`
    Then the global copy contains none of the `execution_*` fields

  # ---- Error / sad path ----

  Scenario: Updating a nonexistent mote returns the existing not-found error
    When I run `mote update motes-doesnotexist --execution-mode=parallel`
    Then the command exits non-zero
    And no new mote is created
