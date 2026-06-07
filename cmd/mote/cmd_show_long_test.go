// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"motes/internal/core"
)

// Scenario 3: --long appends an "--- internal state ---" section after default.
func TestShow_Long_HasInternalStateSection(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, err := mm.Create("task", "Forensic target", core.CreateOpts{Body: "body", Local: true, Weight: 0.5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resetShowFlags()
	defer resetShowFlags()

	defaultOut := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{m.ID}); err != nil {
			t.Fatalf("default runShow: %v", err)
		}
	})

	resetShowFlags()
	showLong = true

	longOut := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{m.ID}); err != nil {
			t.Fatalf("--long runShow: %v", err)
		}
	})

	// The internal-state section is appended; default content must still appear.
	// STORY-HDRZ-001: the old `=== <id> ===` header was replaced by the
	// two-zone header. The ID still appears on the first line as part of
	// the left zone.
	for _, want := range []string{m.ID, "--- internal state ---"} {
		if !strings.Contains(longOut, want) {
			t.Errorf("--long output should contain %q; got:\n%s", want, longOut)
		}
	}
	for _, key := range []string{"last_prime_at", "audit_log_path", "audit_log_entries", "promoted_to", "strata_corpus", "deprecated_by", "status_changed_at"} {
		if !strings.Contains(longOut, key+":") {
			t.Errorf("--long output should contain internal-state key %q; got:\n%s", key, longOut)
		}
	}
	// Default output is a prefix-superset of --long up to the internal-state
	// section: every default line should still be present somewhere in --long.
	for _, line := range strings.Split(strings.TrimRight(defaultOut, "\n"), "\n") {
		if !strings.Contains(longOut, line) {
			t.Errorf("--long missing default line %q", line)
		}
	}
}

// Scenario 7: --long --json is a strict key-superset of default --json.
func TestShow_LongJSON_IsSupersetOfDefaultJSON(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, err := mm.Create("task", "Superset target", core.CreateOpts{Body: "body", Local: true, Weight: 0.5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resetShowFlags()
	defer resetShowFlags()

	showJSON = true
	defaultOut := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{m.ID}); err != nil {
			t.Fatalf("default --json: %v", err)
		}
	})

	resetShowFlags()
	showJSON = true
	showLong = true
	longOut := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{m.ID}); err != nil {
			t.Fatalf("--long --json: %v", err)
		}
	})

	var defKeys, longKeys map[string]any
	if err := json.Unmarshal([]byte(defaultOut), &defKeys); err != nil {
		t.Fatalf("default JSON parse: %v\n%s", err, defaultOut)
	}
	if err := json.Unmarshal([]byte(longOut), &longKeys); err != nil {
		t.Fatalf("long JSON parse: %v\n%s", err, longOut)
	}
	for k := range defKeys {
		if _, ok := longKeys[k]; !ok {
			t.Errorf("long output missing default key %q", k)
		}
	}
	// Extension keys that should always be present (Q2: pragmatic mapped set).
	for _, ext := range []string{
		"audit_log_path",
		"audit_log_entries_count",
	} {
		if _, ok := longKeys[ext]; !ok {
			t.Errorf("long output missing extension key %q (keys: %v)", ext, keysOf(longKeys))
		}
	}
}

// Scenario 7: --long --json includes deprecation_chain when the mote is part of one.
func TestShow_LongJSON_DeprecationChain(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	old, _ := mm.Create("decision", "Old approach", core.CreateOpts{Body: "old", Local: true, Weight: 0.5})
	new1, _ := mm.Create("decision", "New approach", core.CreateOpts{Body: "new", Local: true, Weight: 0.5})

	// Link new1 supersedes old → old.DeprecatedBy = new1.ID and old.Status = "deprecated"
	if err := mm.Link(new1.ID, "supersedes", old.ID, im); err != nil {
		t.Fatalf("link supersedes: %v", err)
	}
	motes, _ := mm.ReadAllParallel()
	if err := im.Rebuild(motes); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	resetShowFlags()
	showJSON = true
	showLong = true
	defer resetShowFlags()

	out := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{old.ID}); err != nil {
			t.Fatalf("--long --json: %v", err)
		}
	})

	var parsed ShowLongOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if parsed.DeprecatedBy != new1.ID {
		t.Errorf("expected deprecated_by=%q, got %q", new1.ID, parsed.DeprecatedBy)
	}
	if len(parsed.DeprecationChain) != 1 || parsed.DeprecationChain[0] != new1.ID {
		t.Errorf("expected deprecation_chain=[%q], got %v", new1.ID, parsed.DeprecationChain)
	}
}
