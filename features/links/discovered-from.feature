# STORY-DISC-001 — `discovered_from` link type for follow-up provenance
#
# This file is living documentation. Motes does not currently bundle a Gherkin
# runner (godog/cucumber-go); the executable spec for these scenarios is the
# Go test suite — see:
#   internal/core/link_types_discovered_from_test.go
#   internal/core/mote_discovered_from_test.go
#   internal/core/link_discovered_from_test.go
#   internal/core/mote_manager_ready_discovered_from_test.go
#   cmd/mote/cmd_link_discovered_from_test.go
#   cmd/mote/cmd_show_discovered_from_test.go
#   cmd/mote/cmd_context_discovered_from_test.go
#
# The reverse edge `discovered_ref` (instead of plain `discovered` as drafted in
# the story) is the implemented name — matches the existing `builds_on` /
# `built_by_ref` convention for index-only reverse edges.

@links @discovered-from
Feature: discovered_from link type for follow-up provenance

  Background:
    Given a motes workspace initialized with `mote init`
    And   a task mote `motes-AAA` exists with status `in_progress`

  Scenario: Agent creates a follow-up mote and links it to the source via discovered_from
    Given  a new task mote `motes-BBB` has been created with title "Follow-up: race in retry loop"
    When   the agent runs `mote link motes-BBB discovered_from motes-AAA`
    Then   the command exits with status code 0
    And    `motes-BBB` records `motes-AAA` in its `discovered_from` field
    And    `motes-AAA` is unchanged on disk (no new field added)
    And    stdout contains `"Linked motes-BBB --discovered_from--> motes-AAA"`

  Scenario: The source mote can be traversed to motes it spawned
    Given  `motes-BBB` is linked to `motes-AAA` via `discovered_from`
    When   the agent runs `mote context motes-AAA`
    Then   the output includes a "Discovered" section listing `motes-BBB`
    And    the index records the reverse edge `motes-AAA --discovered_ref--> motes-BBB`

  Scenario: A mote with an open discovered_from target stays ready
    Given  `motes-AAA` has status `in_progress` (not yet completed)
    And    `motes-BBB` has status `active` and has no `depends_on` entries
    And    `motes-BBB` is linked to `motes-AAA` via `discovered_from`
    When   the agent runs `mote ls --ready --json`
    Then   the output includes `motes-BBB`
    And    `motes-BBB` is treated as ready even though `motes-AAA` is unfinished

  Scenario: Agent previews a discovered_from link with --dry-run
    Given  `motes-BBB` exists with no `discovered_from` entries
    When   the agent runs `mote link --dry-run motes-BBB discovered_from motes-AAA`
    Then   the command exits with status code 0
    And    stdout contains `"[dry-run] motes-BBB --discovered_from--> motes-AAA"`
    And    on-disk, `motes-BBB.discovered_from` remains empty

  Scenario: Agent attempts to link to a non-existent source mote
    Given  no mote with ID `motes-ZZZ` exists
    When   the agent runs `mote link motes-BBB discovered_from motes-ZZZ`
    Then   the command exits with a non-zero status code
    And    stderr contains `"read target motes-ZZZ"`
    And    `motes-BBB.discovered_from` is unchanged

  Scenario: Agent attempts to link with a path-traversal-shaped ID
    Given  `motes-BBB` exists
    When   the agent runs `mote link motes-BBB discovered_from "../../etc/passwd"`
    Then   the command exits with a non-zero status code
    And    stderr contains `"invalid target ID"`
    And    `motes-BBB.discovered_from` is unchanged

  Scenario: Agent links discovered_from twice with the same source and target
    Given  `motes-BBB` already has `discovered_from: [motes-AAA]`
    When   the agent runs `mote link motes-BBB discovered_from motes-AAA` a second time
    Then   the command exits with status code 0
    And    `motes-BBB.discovered_from` still contains exactly one entry: `motes-AAA`

  Scenario: Agent removes a discovered_from link
    Given  `motes-BBB` has `discovered_from: [motes-AAA]`
    And    the index records the reverse edge `motes-AAA --discovered_ref--> motes-BBB`
    When   the agent runs `mote unlink motes-BBB discovered_from motes-AAA`
    Then   the command exits with status code 0
    And    `motes-BBB.discovered_from` is empty (field omitted from YAML when empty)
    And    the index no longer records the reverse edge
