# STORY-PRIMEOVR-001 — Customizable `PRIME.md` override with three-tier
# resolution + `mote prime --export`.
#
# Living-documentation Gherkin specification for the prose-preamble
# override added in v0.4.41. This file is committed parallel to the Go
# test suite — there is no Cucumber runner; the assertions are codified
# in `internal/prime/override_test.go`, `cmd/mote/cmd_prime_override_test.go`,
# and `cmd/mote/cmd_prime_export_test.go`.

Feature: Customizable `PRIME.md` override with three-tier resolution

  Background:
    Given the `mote` binary is on PATH
    And `.memory/` is initialized in the current project
    And the truncation directive (Sprint 1) is always prepended
    And `mote prime --memories-only` (Sprint 1) bypasses override resolution
    And the resolved workspace root is the parent directory of `.memory/`

  Scenario: Clone-specific PRIME.md is found at the highest-priority tier
    Given the file `./.memory/PRIME.md` exists with body content "alpha"
    And `<workspace>/PRIME.md` exists with body content "beta"
    And `~/.motes/PRIME.md` exists with body content "gamma"
    When the developer runs `mote prime`
    Then the output begins with the truncation directive (unchanged)
    And the prose section of the output equals "alpha"
    And the prose section does NOT contain any portion of "beta" or "gamma"
    And the data sections (memories, ready_tasks, active_tasks, etc.)
        are unchanged from the default render

  Scenario: Workspace-shared PRIME.md is used when clone-specific is absent
    Given the file `./.memory/PRIME.md` does NOT exist
    And `<workspace>/PRIME.md` exists with body content "shared rules for this repo"
    And `~/.motes/PRIME.md` exists with body content "global rules"
    When the developer runs `mote prime`
    Then the prose section of the output equals "shared rules for this repo"
    And the prose section does NOT contain any portion of "global rules"

  Scenario: User-global PRIME.md is used when neither project-level file exists
    Given the file `./.memory/PRIME.md` does NOT exist
    And the file `<workspace>/PRIME.md` does NOT exist
    And `~/.motes/PRIME.md` exists with body content "my own rules across all projects"
    When the developer runs `mote prime`
    Then the prose section of the output equals "my own rules across all projects"

  Scenario: No PRIME.md anywhere — default prose section unchanged from today
    Given no PRIME.md exists at any of the three tiers
    When the developer runs `mote prime`
    Then the rendered output between the truncation directive and the
         `## Persistent memories` header contains only blank lines
    And the data sections are unchanged from today's render

  Scenario: --export dumps the baked-in default template for the user to customize
    Given the developer wants a starting point to customize
    When the developer runs `mote prime --export`
    Then the command writes prime.DefaultExportTemplate() to stdout
    And the command does NOT emit data sections (no memories, no ready
        tasks, no decisions — this is a TEMPLATE, not a live render)
    And the command does NOT emit the truncation directive
        (the directive is mote-generated; the export is user-facing)
    And the command exits 0
    And the developer can pipe the output to any of the three tiers:
        `mote prime --export > .memory/PRIME.md`

  Scenario Outline: A PRIME.md that cannot be read falls through to the next tier
    Given the file at <tier_path> exists but cannot be read (e.g.,
          unreadable permissions, invalid UTF-8, broken symlink)
    And the next tier <next_tier_path> exists with body "fallback content"
    When the developer runs `mote prime`
    Then mote falls through to the next tier
    And the prose section equals "fallback content"
    And NO stderr message is emitted under default behavior
        (per Sprint 1 STORY-BR-23-4 silent-failure semantics)
    But `mote prime --debug` DOES surface a one-line warning naming
        the path that failed to read and the failure reason

    Examples:
      | tier_path              | next_tier_path              |
      | ./.memory/PRIME.md     | <workspace>/PRIME.md        |
      | <workspace>/PRIME.md   | ~/.motes/PRIME.md           |
      | ~/.motes/PRIME.md      | (baked-in default = no prose)|

  Scenario: An oversized PRIME.md is truncated to the configured limit
    Given `./.memory/PRIME.md` contains more than PRIME_MD_MAX_BYTES (16384)
          of content
    When the developer runs `mote prime`
    Then the prose section emitted is exactly the first PRIME_MD_MAX_BYTES
        of the file content
    And the emitted section ends with a clearly-marked truncation footer
        ("\n[PRIME.md truncated at 16384 bytes — see <path>]")
    And `mote prime` still exits 0

  Scenario: --memories-only ignores PRIME.md (Sprint 1 contract preserved)
    Given any tier has a PRIME.md present
    When the developer runs `mote prime --memories-only`
    Then the output is the persistent-memories block ONLY
        (Sprint 1 behavior, unchanged)
    And the PRIME.md contents do NOT appear in the output
    And the truncation directive is still prepended

  Scenario: --export safety guidance when running interactively
    Given the developer runs `mote prime --export` directly in a TTY
    When the command completes
    Then the output is the baked-in default template
    And after the output, a single-line hint is printed to STDERR:
        "Hint: pipe to a file, e.g., `mote prime --export > .memory/PRIME.md`"
    And the exit code remains 0
    And the hint is suppressed when stdout is piped

  Scenario: PRIME.md content surfaces in MCP mode too
    Given the project has `./.memory/PRIME.md` with body "mcp-prose"
    And MCP-mode is active (either auto-detected or via `--mcp`)
    When the developer runs `mote prime`
    Then the brief MCP-mode payload contains the truncation directive,
         the prose preamble "mcp-prose", the memories block, and the
         MCP notice line — in that order
    And large PRIME.md content may exceed MCP_MODE_TOKEN_BUDGET; the
        docs flag this so operators can keep their prose concise
