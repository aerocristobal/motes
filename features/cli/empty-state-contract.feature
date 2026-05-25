# STORY-EMPTY-001 — Empty-state semantics for the claim / ready workflow
#
# This file is living documentation. Motes does not currently bundle a
# Gherkin runner (godog/cucumber-go); the executable spec for these
# scenarios is the Go test suite — see:
#   cmd/mote/cmd_ls_empty_state_test.go       (Scenarios 1–6)
#   cmd/mote/cmd_ls_polling_test.go           (Scenario 8)
#   cmd/mote/cmd_update_claim_contract_test.go (Scenario 7 — partner to
#                                               cmd_update_claim_test.go)
#
# Scenario 6 is reinterpreted to match real behavior: `mote ls --ready`
# does NOT consult `.memory/index.jsonl`. It scans `nodes/` directly via
# ReadAllParallel; malformed node files are silently skipped with a stderr
# warning. The realistic error path is graceful degradation, not a
# non-zero exit.

@cli @empty-state @claim
Feature: Empty-state semantics for the claim / ready workflow

  Background:
    Given a motes workspace initialized with `mote init`

  Scenario: Agent polls an empty workspace for ready work
    Given  the workspace contains no task motes
    When   the agent runs `mote ls --ready --json`
    Then   the command exits with status code 0
    And    stdout is exactly the JSON document `{"motes":[]}`

  Scenario: Agent polls a workspace where all tasks are already in_progress
    Given  the workspace contains three task motes, all with status `in_progress`
    When   the agent runs `mote ls --ready --json`
    Then   the command exits with status code 0
    And    stdout is exactly the JSON document `{"motes":[]}`

  Scenario: Agent polls a workspace where every active task has unfinished blockers
    Given  task `motes-AAA` exists with status `active`, depends_on `motes-BBB`
    And    `motes-BBB` exists with status `in_progress`
    When   the agent runs `mote ls --ready --json`
    Then   the command exits with status code 0
    And    stdout is exactly the JSON document `{"motes":[]}`

  Scenario: Agent polls a workspace with one ready task
    Given  exactly one task mote exists with status `active` and no unfinished blockers
    When   the agent runs `mote ls --ready --json`
    Then   the command exits with status code 0
    And    stdout is a valid JSON document
    And    `motes` is an array of length 1
    And    `motes[0].id` is non-empty

  Scenario: Agent supplies an unknown flag to `mote ls`
    When   the agent runs `mote ls --ready --json --not-a-real-flag`
    Then   the command exits with a non-zero status code
    And    the returned error names the unknown flag
    And    stdout does NOT contain `{"motes":`

  Scenario: Agent polls a workspace containing a malformed node file
    Given  the workspace contains a `.md` file in `nodes/` with malformed frontmatter
    When   the agent runs `mote ls --ready --json`
    Then   the command exits with status code 0
    And    stdout is a valid JSON document
    And    stderr contains a warning identifying the skipped file
    # Rationale: ls does not consult .memory/index.jsonl, and malformed
    # nodes are skipped non-fatally. Run `mote doctor` to surface
    # workspace inconsistencies, or `mote dream` to rebuild the index.

  Scenario: Agent attempts to claim a specific mote that is already in_progress
    Given  task `motes-AAA` has status `in_progress` and `claimed_by` `codex-beta`
    And    the environment variable `MOTE_AGENT_ID` is set to `claude-alpha`
    When   the agent runs `mote update motes-AAA --claim --json`
    Then   the command exits with status code 2 (contention)
    And    stdout contains a JSON document with `"claimed": false`
    And    a subsequent `mote ls --ready --json` returns `{"motes":[]}` with exit 0

  Scenario: Agent's polling loop terminates exactly when work becomes available
    Given  the workspace contains no ready motes at time T0
    And    the agent runs the polling idiom (100ms interval)
    When   a new task mote with status `active` and no blockers is created at T0+500ms
    Then   the loop observes the new mote within 1 second of creation
    And    every poll during the empty interval returned `{"motes":[]}` with exit 0
    And    no spurious non-zero exits occurred during the empty interval
