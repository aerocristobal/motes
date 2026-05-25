# STORY-TIME-001 — Time-based scheduling of motes (`--due` / `--defer`)
#
# This file is living documentation. Motes does not bundle a Gherkin runner
# (godog/cucumber-go); the executable spec for these scenarios is the Go
# test suite — see:
#
#   internal/core/time_spec_test.go               (ParseTimeSpec scaffold, §6.1)
#   internal/core/mote_manager_schedule_test.go   (Core layer, §6.2)
#   cmd/mote/cmd_add_schedule_test.go             (`mote add` CLI integration, §6.3)
#   cmd/mote/cmd_update_schedule_test.go          (`mote update` CLI integration, §6.3)
#   cmd/mote/cmd_ls_schedule_test.go              (`mote ls` CLI integration, §6.4)
#
# Audit note: the story scenarios narrate audit events like
# `{action: "update", prior_due_at: null, new_due_at: <value>}`. The current
# `AuditEntry` schema records field NAMES via `fields_set []string`, not
# before/after values; tests assert against the FieldsSet shape rather than
# the literal prior_*/new_* wording. A follow-up captures the enrichment as
# an out-of-scope task.

@mote @scheduling @time
Feature: Time-based scheduling of motes

  Background:
    Given mote `motes-abc` exists with type=task and status=active
    And the agent ID is set via `MOTE_AGENT_ID=agent-A`

  # ---- Setting due / defer ----

  Scenario: Set a due date on create (relative)
    When I run `mote add --type=task --title="ship report" --due=+2d`
    Then the new mote's frontmatter contains a `due_at` field
    And the value is an RFC3339 UTC timestamp approximately 2 days from now
    And the audit log records {action: "create", due_at: <value>}

  Scenario: Set a defer-until on create (natural language)
    When I run `mote add --type=task --title="follow up" --defer="next monday"`
    Then the new mote's frontmatter contains a `defer_until` field
    And the value is an RFC3339 UTC timestamp at 00:00 local time of the next Monday
    And the audit log records {action: "create", defer_until: <value>}

  Scenario: Set both `--due` and `--defer` on the same mote
    When I run `mote add --type=task --title="bridge work" --due=+1w --defer=+2d`
    Then both `due_at` and `defer_until` are written
    And the mote is not visible in `mote ls --ready` until the defer expires
    And the mote becomes visible in `mote ls --overdue` once `due_at` passes

  Scenario: Update an existing mote's due date
    Given mote `motes-abc` has no `due_at`
    When I run `mote update motes-abc --due=+6h`
    Then `motes-abc.due_at` is set to ≈6 hours from now
    And the audit log records {action: "update", prior_due_at: null, new_due_at: <value>}

  Scenario: Clear a defer with empty-string value
    Given mote `motes-abc` has `defer_until` set to tomorrow at 09:00
    When I run `mote update motes-abc --defer=""`
    Then `motes-abc.defer_until` is removed from frontmatter
    And the audit log records {action: "update", prior_defer_until: <prev>, new_defer_until: null}

  # ---- Ready queue interaction (composes with sprint-2 §7 / §23.16) ----

  Scenario: Deferred motes are hidden from `--ready` by default
    Given mote `motes-abc` has `defer_until` set to +1d in the future
    And `motes-abc` would otherwise satisfy the ready predicate
    When I run `mote ls --ready --json`
    Then the JSON `motes` array does NOT contain `motes-abc`
    And the command exits 0
    And the JSON is `{"motes":[]}` when no other motes are ready

  Scenario: `--include-deferred` reveals deferred-but-otherwise-ready motes
    Given mote `motes-abc` has `defer_until` set to +1d in the future
    And `motes-abc` would otherwise satisfy the ready predicate
    When I run `mote ls --ready --include-deferred --json`
    Then the JSON `motes` array contains `motes-abc`

  Scenario: An expired `defer_until` is treated as not-deferred
    Given mote `motes-abc` has `defer_until` set to a timestamp 1 hour in the past
    And `motes-abc` would otherwise satisfy the ready predicate
    When I run `mote ls --ready`
    Then `motes-abc` appears in the output
    And no mutation is made to `motes-abc` (the field is not auto-cleared)

  # ---- Overdue surfacing ----

  Scenario: `mote ls --overdue` lists motes past their due date regardless of status
    Given mote `motes-abc` has `due_at` set to 1 day in the past and status=active
    And mote `motes-xyz` has `due_at` set to 2 hours in the past and status=in_progress
    When I run `mote ls --overdue --json`
    Then the JSON `motes` array contains both `motes-abc` and `motes-xyz`
    And the array is sorted by `due_at` ascending (most overdue first)

  Scenario: Completed motes are excluded from `--overdue`
    Given mote `motes-abc` has `due_at` set to 1 day in the past and status=completed
    When I run `mote ls --overdue --json`
    Then the JSON `motes` array does NOT contain `motes-abc`

  Scenario Outline: `--due-before` and `--due-after` filter the listing
    Given the motes below exist:
      | id      | due_at_offset |
      | motes-a | -2h           |
      | motes-b | +6h           |
      | motes-c | +3d           |
    When I run `<command>`
    Then the resulting `motes` array contains exactly `<expected>`

    Examples:
      | command                                    | expected             |
      | mote ls --due-before=now                   | motes-a              |
      | mote ls --due-before=+1d                   | motes-a, motes-b     |
      | mote ls --due-after=+1d                    | motes-c              |
      | mote ls --due-after=now                    | motes-b, motes-c     |

  # ---- Time-string parsing ----

  Scenario Outline: Accepted time-string formats
    When I run `mote add --type=task --title=t --due=<value>`
    Then the command exits 0
    And `due_at` is set to an RFC3339 UTC timestamp

    Examples:
      | value           |
      | +6h             |
      | +1d             |
      | +1w             |
      | +30m            |
      | tomorrow        |
      | next monday     |
      | 2026-12-01      |
      | 2026-12-01T10:00:00Z |

  Scenario Outline: Rejected time-string formats
    When I run `mote add --type=task --title=t --due=<value>`
    Then the command exits non-zero
    And stderr contains "invalid time"
    And no mote is created

    Examples:
      | value           |
      | yesterday       |
      | -1d             |
      | tomrrow         |
      | +999999d        |
      | not a date      |
      | $(rm -rf ~)     |
      | ../../etc/passwd|

  # ---- Boundary: defer/due in the past at create time ----

  Scenario: Defer-in-the-past is rejected at create
    When I run `mote add --type=task --title=t --defer=-1h`
    Then the command exits non-zero
    And stderr contains "defer must be in the future"

  Scenario: Due-in-the-past is allowed (back-dating a missed deadline)
    When I run `mote add --type=task --title=t --due=2026-01-01T00:00:00Z`
    Then the command exits 0
    And the mote immediately appears in `mote ls --overdue`

  # ---- Error / sad path ----

  Scenario: Updating a non-existent mote with --due returns the existing not-found error
    When I run `mote update motes-doesnotexist --due=+1d`
    Then the command exits non-zero
    And stderr matches the existing "mote not found" error pattern
    And no new mote is created

  Scenario: --due-before with no matches returns empty list, exit 0
    Given no mote has `due_at` set
    When I run `mote ls --due-before=now --json`
    Then stdout is `{"motes":[]}`
    And the command exits 0
