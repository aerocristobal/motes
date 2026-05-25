// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"motes/internal/core"
)

// seedMemory is a tiny convenience wrapper for tests that just need one
// or two memories present in the store.
func seedMemory(t *testing.T, root, key, body string) {
	t.Helper()
	if _, err := core.NewMemoryStore(root).Put(key, body, "test", core.PutOpts{}); err != nil {
		t.Fatalf("seed memory %q: %v", key, err)
	}
}

// Scenario 7: mote prime emits a "Persistent memories" section by default,
// positioned BEFORE "Ready to start" / "Active work".
func TestPrime_InjectsMemoriesByDefault(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Pending task", Tags: []string{"test"}, Weight: 0.5},
	})
	seedMemory(t, root, "race-flag", "always run with -race")

	output := captureStdout(func() {
		_ = primeCmd.RunE(primeCmd, nil)
	})

	if !strings.Contains(output, "Persistent memories") {
		t.Errorf("prime output should contain 'Persistent memories' section:\n%s", output)
	}
	if !strings.Contains(output, "always run with -race") {
		t.Errorf("prime output should contain the memory body:\n%s", output)
	}

	memIdx := strings.Index(output, "## Persistent memories")
	activeIdx := strings.Index(output, "## Active work")
	if memIdx < 0 {
		t.Fatal("missing '## Persistent memories' heading")
	}
	if activeIdx >= 0 && memIdx > activeIdx {
		t.Errorf("memories should appear before active work; mem=%d active=%d\noutput:\n%s",
			memIdx, activeIdx, output)
	}
}

// Empty memory store → no section emitted in text mode.
func TestPrime_NoMemoriesSection_WhenEmpty(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Only task", Tags: []string{"x"}, Weight: 0.5},
	})

	output := captureStdout(func() {
		_ = primeCmd.RunE(primeCmd, nil)
	})
	if strings.Contains(output, "Persistent memories") {
		t.Errorf("empty memory store should not emit memories section:\n%s", output)
	}
}

// Scenario 8: --memories-only suppresses every other section.
func TestPrime_MemoriesOnly_SuppressesOtherSections(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Ready task", Tags: []string{"x"}, Weight: 0.5},
		{Type: "decision", Title: "Recent decision", Tags: []string{"x"}, Body: "we chose X"},
		{Type: "lesson", Title: "Learned thing", Tags: []string{"x"}, Body: "lesson body"},
	})
	seedMemory(t, root, "rule", "the rule")

	primeMemoriesOnly = true
	defer func() { primeMemoriesOnly = false }()

	output := captureStdout(func() {
		_ = primeCmd.RunE(primeCmd, nil)
	})

	if !strings.Contains(output, "Persistent memories") {
		t.Errorf("memories-only output must contain memories section:\n%s", output)
	}
	for _, banned := range []string{"## Ready to start", "## Active work",
		"## Relevant decisions", "## Key lessons", "## Prior explorations"} {
		if strings.Contains(output, banned) {
			t.Errorf("--memories-only should suppress %q; output:\n%s", banned, output)
		}
	}
}

// Scenario 8 / JSON variant: --memories-only --json emits envelope with
// only the memories field populated; other arrays present but empty.
func TestPrime_MemoriesOnly_JSON(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Ignored task", Tags: []string{"x"}, Weight: 0.5},
	})
	seedMemory(t, root, "k", "v")

	primeJSON = true
	primeMemoriesOnly = true
	defer func() { primeJSON = false; primeMemoriesOnly = false }()

	output := captureStdout(func() {
		_ = primeCmd.RunE(primeCmd, nil)
	})
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON in output:\n%s", output)
	}
	var got PrimeOutput
	if err := json.Unmarshal([]byte(output[idx:]), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if len(got.Memories) != 1 || got.Memories[0].Key != "k" || got.Memories[0].Body != "v" {
		t.Errorf("memories field wrong: %+v", got.Memories)
	}
	if len(got.ActiveTasks) != 0 {
		t.Errorf("active_tasks should be empty with --memories-only, got %d", len(got.ActiveTasks))
	}
	if len(got.Decisions) != 0 || len(got.Lessons) != 0 || len(got.Explores) != 0 {
		t.Errorf("knowledge arrays should be empty: %+v", got)
	}
}

// JSON output includes the memories field even in normal mode.
func TestPrime_JSON_IncludesMemoriesField(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Some task", Tags: []string{"x"}, Weight: 0.5},
	})
	seedMemory(t, root, "k", "v")

	primeJSON = true
	defer func() { primeJSON = false }()

	output := captureStdout(func() {
		_ = primeCmd.RunE(primeCmd, nil)
	})
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON in output:\n%s", output)
	}
	var got PrimeOutput
	if err := json.Unmarshal([]byte(output[idx:]), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if len(got.Memories) != 1 || got.Memories[0].Key != "k" {
		t.Errorf("memories field missing or wrong: %+v", got.Memories)
	}
}
