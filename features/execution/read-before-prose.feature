# STORY-EREAD-001 — "Read execution metadata before prose" agent contract
#
# This file is living documentation. Motes does not currently bundle a
# Gherkin runner; the executable spec for these scenarios is the Go test
# suite — see:
#   cmd/mote/cmd_show_execution_only_test.go     (--execution-only flag, exit codes, output shape, mutex with --json)
#   internal/core/access_log_execution_test.go   (read_execution access-log event, not conflated with read, not de-duped)
#   cmd/mote/cmd_show_contract_docs_test.go      (documentation drift checks — agents-guide.md, AGENTS.md, vendor MDs, skill)
#
# The schema lives in STORY-EXEC-001; this story owns the orchestrator
# contract. The contract is enforced by *documentation*, not by code — we
# just make the easy path the right path (JSON places execution first; the
# dedicated --execution-only flag makes the contract-honoring read a single
# keystroke; the access-log action distinguishes inspect-only from a full
# body read).

@mote @execution @contract @docs
Feature: Read execution metadata before prose

  Background:
    Given STORY-EXEC-001 has shipped (the five execution_* frontmatter fields)
    And mote `motes-abc` exists with all five execution fields set

  # ---- The CLI affordance: --execution-only ----

  Scenario: `mote show <id> --execution-only` returns just the execution block as JSON
    When I run `mote show motes-abc --execution-only`
    Then stdout is a JSON object with exactly these keys:
      | id                        |
      | execution_agent_type      |
      | execution_suggested_model |
      | execution_reasoning_effort|
      | execution_mode            |
      | execution_parallel_group  |
    And stdout contains no `body` field
    And stdout contains no `title`, `tags`, `weight`, or other non-execution mote fields
    And the command exits 0

  Scenario: `--execution-only` on a mote with no execution metadata returns an empty object + exit 0
    Given mote `motes-xyz` has no `execution_*` fields
    When I run `mote show motes-xyz --execution-only`
    Then stdout is `{"id":"motes-xyz"}`
    And the command exits 0

  Scenario: `--execution-only` is mutually exclusive with `--json`
    When I run `mote show motes-abc --execution-only --json`
    Then the command exits non-zero
    And stderr contains "--execution-only is mutually exclusive with --json"

  Scenario: `--execution-only` on a nonexistent mote returns the existing not-found error
    When I run `mote show motes-doesnotexist --execution-only`
    Then the command exits non-zero
    And stderr matches the existing "mote not found" pattern

  # ---- JSON output ordering reaffirms the contract ----

  Scenario: `mote show <id> --json` places execution keys before body in the serialized output
    When I run `mote show motes-abc --json`
    Then every `execution_*` key appears before `body` in the serialized JSON
    # Also asserted in STORY-EXEC-001; restated here as the contract's machine-checkable shape

  # ---- Documentation acceptance criteria ----

  Scenario: `docs/agents-guide.md` contains the exact contract block
    When I read `docs/agents-guide.md`
    Then it contains a section titled "Read execution metadata before prose"
    And the section contains the example invocation `mote show <id> --execution-only | jq .`
    And the section explicitly states "a running subagent cannot change its model or reasoning effort after launch"
    And the section explicitly states that execution metadata is "authoritative when present"

  Scenario: `AGENTS.md` references the contract in one line
    When I read `AGENTS.md`
    Then it contains a reference to `docs/agents-guide.md#read-execution-metadata-before-prose`

  Scenario: The mote-subagent skill mirrors the contract
    When I read `skills/mote-subagent/SKILL.md`
    Then it instructs the subagent that "your model and reasoning effort were chosen from execution metadata at dispatch time and cannot be changed"
    And it references `docs/agents-guide.md#read-execution-metadata-before-prose`

  Scenario: `CLAUDE.md`, `CODEX.md`, `GEMINI.md` each reference the contract
    When I read each of `CLAUDE.md`, `CODEX.md`, `GEMINI.md`
    Then each file contains a one-line reference to `docs/agents-guide.md#read-execution-metadata-before-prose`

  # ---- The audit dimension ----

  Scenario: An orchestrator that calls `--execution-only` leaves an inspect-only trace
    Given the agent ID is set via `MOTE_AGENT_ID=agent-orchestrator`
    When I run `mote show motes-abc --execution-only`
    Then the access log records {action: "read_execution", mote_id: "motes-abc", agent_id: "agent-orchestrator"}
    And the access log does NOT record a `read_body` event

  Scenario: An orchestrator that calls plain `mote show` leaves a normal read trace
    When I run `mote show motes-abc`
    Then the access log records {action: "read", mote_id: "motes-abc"}

  Scenario: Multiple `--execution-only` calls in the same session are NOT de-duped
    When I run `mote show motes-abc --execution-only` three times
    Then the access log contains three distinct `read_execution` entries
