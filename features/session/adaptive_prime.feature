# STORY-ADAPRIME-001 — Adaptive `mote prime` context size
#
# Living-documentation Gherkin specification for the MCP-mode auto-detection,
# explicit --mcp / --full overrides, and the documented size budget added in
# v0.4.40. This file is committed parallel to the Go test suite — there is no
# Cucumber runner; the assertions are codified in
# `internal/prime/*_test.go` and `cmd/mote/cmd_prime_adaptive_test.go`.

Feature: Adaptive `mote prime` context size

  Background:
    Given the `mote` binary is on PATH
    And `.memory/` is initialized in the current project
    And the truncation directive (Sprint 1) is always prepended
    And the silent-failure semantics (Sprint 1, --debug) remain unchanged

  Scenario: ~/.claude/settings.json declares a "mote" MCP server — brief prime emitted
    Given `~/.claude/settings.json` contains an `mcpServers` object with a
          key named `mote` (case-sensitive match)
    And no `--mcp`, `--full`, or `--memories-only` flag is passed
    When the agent runs `mote prime`
    Then the output contains the truncation directive (unchanged from
         today's behavior)
    And the output contains the brief workflow reminder block
         (persistent memories + a single line referencing
         "use MCP tools to fetch detail")
    And the output does NOT contain the full command reference block
         (no listing of `mote ls --ready` / `mote search` / etc.)
    And the output token count is ≤ MCP_MODE_TOKEN_BUDGET (75)
    And the JSON form (`mote prime --json`) sets `mode: "mcp"` in the envelope

  Scenario: No `mote` MCP server in any detected settings file — full payload
    Given `~/.claude/settings.json` either does not exist, or exists but
          has no `mcpServers.mote` entry
    And `~/.codex/settings.json` and `~/.gemini/settings.json` likewise
          do not declare a `mote` MCP server
    And no `--mcp`, `--full`, or `--memories-only` flag is passed
    When the agent runs `mote prime`
    Then the output is byte-for-byte unchanged from the pre-story baseline
         (same content as today's `mote prime` — Sprint 1 directive
         prepended, full memories, full reference block)
    And the JSON form sets `mode: "cli"` in the envelope

  Scenario: Explicit `--mcp` flag bypasses detection
    Given NO settings file declares a `mote` MCP server
    When the agent runs `mote prime --mcp`
    Then the brief MCP-mode payload is emitted as if detection had succeeded
    And the output token count is ≤ MCP_MODE_TOKEN_BUDGET (75)
    And the JSON form sets `mode: "mcp"` and `mode_source: "flag"`

  Scenario: Explicit `--full` flag bypasses detection
    Given `~/.claude/settings.json` declares a `mote` MCP server
    When the agent runs `mote prime --full`
    Then the full CLI-mode payload is emitted as if detection had failed
    And the JSON form sets `mode: "cli"` and `mode_source: "flag"`

  Scenario Outline: --memories-only collapses both modes to just the memories block
    Given the MCP-server detection state is <detected>
    And the explicit flag is <flag>
    And `--memories-only` is also passed
    When the agent runs `mote prime --memories-only <flag>`
    Then the output is the persistent-memories block ONLY (today's
         --memories-only behavior, unchanged from Sprint 1)
    And the truncation directive is still prepended

    Examples:
      | detected | flag    |
      | yes      |         |
      | no       |         |
      | yes      | --mcp   |
      | no       | --mcp   |
      | yes      | --full  |
      | no       | --full  |

  Scenario: Conflicting mode overrides are rejected before any prime output
    Given any project state
    When the agent runs `mote prime --mcp --full`
    Then the command exits with the same non-zero code used for other CLI
         flag-validation errors
    And stderr contains a single line naming the conflicting flags
    But stdout is empty — no partial prime is emitted, no truncation
         directive is leaked
    And the exit is NOT silenced by the §23.4 / `--debug` policy
         (flag-misuse is a developer error, not a "this is not a mote
         project" environmental condition)

  Scenario: Detection survives a broken settings file
    Given `~/.claude/settings.json` exists but is not valid JSON (or is
          empty, or is a JSON array instead of an object)
    And neither `--mcp` nor `--full` is passed
    When the agent runs `mote prime`
    Then detection treats the broken file as "no mote MCP server
         declared" (falls through to CLI mode)
    And the rest of `mote prime` proceeds normally
    And no stderr message is emitted under default behavior
         (per the existing §23.4 silent-failure policy)
    And `mote prime --debug` DOES surface a one-line warning naming the
         path that failed to parse

  Scenario: docs/agents-guide.md documents the size budget for both modes
    Given the repository docs are part of the deliverable
    When a reader opens `docs/agents-guide.md`
    Then they find a "## `mote prime` size budget" section
    And the section names MCP_MODE_TOKEN_BUDGET (~75 tokens) and
        CLI_MODE_TOKEN_BUDGET (~2500 tokens)
    And the section explains the detection mechanism and flag precedence
    And the section is linked from the README's "Session Lifecycle" subsection
