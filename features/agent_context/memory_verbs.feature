Feature: First-class memory verbs
  In order to record short durable rules and have them surface automatically
  As a coding agent operating in a mote-managed repository
  I want to save, list, recall, and forget memories via intent-named verbs

  Background:
    Given a mote-initialized repository at the current working directory
    And the agent ID environment variable is set to "claude-code"

  Scenario: Agent saves a new memory and the key is auto-derived
    Given no memory with key "always-run-tests-with-race-flag" exists
    When the agent runs:  mote remember "always run tests with -race flag"
    Then a memory is persisted with key "always-run-tests-with-race-flag"
    And the memory body equals "always run tests with -race flag"
    And the command exits with code 0

  Scenario: Agent saves a memory under an explicit key
    Given no memory with key "auth-jwt" exists
    When the agent runs:  mote remember "auth uses JWT not sessions" --key auth-jwt
    Then a memory is persisted with key "auth-jwt"
    And the memory body equals "auth uses JWT not sessions"
    And the command exits with code 0

  Scenario: JSON output for mote remember reports the saved memory
    Given no memory with key "auth-jwt" exists
    When the agent runs:  mote remember "auth uses JWT not sessions" --key auth-jwt --json
    Then the JSON output is a single object
    And the object's "key" field equals "auth-jwt"
    And the object's "created_at" field is an RFC 3339 timestamp
    And the command exits with code 0

  Scenario: Agent lists every memory in compact form
    Given the following memories exist:
      | key             | body                                |
      | auth-jwt        | auth uses JWT not sessions          |
      | race-flag       | always run tests with -race flag    |
      | dolt-required   | dolt must be installed before tests |
    When the agent runs:  mote memories
    Then the output lists exactly three rows
    And each row shows the key and a truncated body
    And the rows appear sorted by key ascending
    And the command exits with code 0

  Scenario: Agent searches memories by substring
    Given the following memories exist:
      | key           | body                                |
      | auth-jwt      | auth uses JWT not sessions          |
      | race-flag     | always run tests with -race flag    |
      | dolt-required | dolt must be installed before tests |
    When the agent runs:  mote memories dolt
    Then the output lists exactly one row with key "dolt-required"
    And the command exits with code 0

  Scenario: Agent fetches a single memory by exact key
    Given a memory with key "auth-jwt" and body "auth uses JWT not sessions" exists
    When the agent runs:  mote recall auth-jwt
    Then the output equals "auth uses JWT not sessions" (one trailing newline)
    And the command exits with code 0

  Scenario: Agent removes a memory by exact key
    Given a memory with key "stale-rule" exists
    When the agent runs:  mote forget stale-rule
    Then no memory with key "stale-rule" remains in the store
    And the command exits with code 0
    And the deletion is recorded in the audit log

  Scenario: Memories appear in mote prime output without an explicit flag
    Given the following memories exist:
      | key       | body                             |
      | race-flag | always run tests with -race flag |
      | auth-jwt  | auth uses JWT not sessions       |
    When the agent runs:  mote prime
    Then the output contains a "## Persistent memories" section
    And the section lists both memory bodies in compact form
    And the section is positioned before "## Ready to start"

  Scenario: Compact hook contexts can request memories alone
    Given at least one memory exists
    And at least one ready task exists
    And at least one recent decision exists
    When the agent runs:  mote prime --memories-only
    Then the output contains the "## Persistent memories" section
    And the output contains no other sections (no ready tasks, no decisions, no lessons)
    And the command exits with code 0

  Scenario: Empty memory body is rejected before any write
    When the agent runs:  mote remember ""
    Then the command exits with a non-zero code
    And stderr contains "memory body cannot be empty"
    And no new memory is persisted

  Scenario Outline: Unknown-key operations return a distinct exit code
    Given no memory with key "<key>" exists
    When the agent runs:  mote <verb> <key>
    Then the command exits with code <exit>
    And stderr contains "memory not found"
    And no memory is created or modified

    Examples:
      | verb   | key             | exit |
      | recall | does-not-exist  | 2    |
      | forget | already-gone    | 2    |

  Scenario: Memory body containing a security-scanned token is rejected
    Given the security scanner flags content containing "BEGIN PRIVATE KEY"
    When the agent runs:  mote remember "BEGIN PRIVATE KEY ..."
    Then the command exits with a non-zero code
    And stderr contains a security warning identifying the matched rule
    And no memory is persisted
    But the same command with --force succeeds

  Scenario: Export emits memories alongside motes for portability
    Given at least one memory and at least one mote exist
    When the agent runs:  mote export --json
    Then the JSON output contains a top-level "memories" array
    And the array contains every memory's key, body, created_at, and updated_at
    And the existing "motes" array contains every mote with its full schema
