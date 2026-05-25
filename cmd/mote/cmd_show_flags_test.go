// SPDX-License-Identifier: MIT
package main

import (
	"errors"
	"strings"
	"testing"

	"motes/internal/core"
)

// Scenario 5: --short and --long together is a configuration error with no side effects.
func TestShow_ShortAndLong_MutuallyExclusive(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, err := mm.Create("task", "Mutex target", core.CreateOpts{Body: "body", Local: true, Weight: 0.5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resetShowFlags()
	showShort = true
	showLong = true
	defer resetShowFlags()

	var runErr error
	stdout, _ := captureBoth(func() {
		runErr = showCmd.RunE(showCmd, []string{m.ID})
	})
	if runErr == nil {
		t.Fatal("--short --long should fail")
	}
	var ec *exitCodeError
	if !errors.As(runErr, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", runErr, runErr)
	}
	if ec.code != 1 {
		t.Errorf("expected exit code 1, got %d", ec.code)
	}
	if !strings.Contains(ec.Error(), "mutually exclusive") {
		t.Errorf("error message should mention mutual exclusivity: %q", ec.Error())
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on mutex error, got: %q", stdout)
	}

	// Side-effect check: mutex check runs before mm.Read, so no access-batch
	// entry should be appended.
	after, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("read after mutex error: %v", err)
	}
	if after.AccessCount != m.AccessCount {
		t.Errorf("mutex error should not append access batch; before=%d after=%d", m.AccessCount, after.AccessCount)
	}
}

// Scenario 8: unknown mote ID returns the same error in any mode.
func TestShow_NotFound_AnyFlagCombination(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	combos := []struct {
		name             string
		short, long, jsn bool
	}{
		{"default", false, false, false},
		{"--short", true, false, false},
		{"--long", false, true, false},
		{"--short --json", true, false, true},
		{"--long --json", false, true, true},
	}

	for _, c := range combos {
		t.Run(c.name, func(t *testing.T) {
			resetShowFlags()
			showShort = c.short
			showLong = c.long
			showJSON = c.jsn
			defer resetShowFlags()

			var runErr error
			stdout, _ := captureBoth(func() {
				runErr = showCmd.RunE(showCmd, []string{"no-such-id"})
			})
			if runErr == nil {
				t.Fatalf("%s: expected error", c.name)
			}
			var ec *exitCodeError
			if !errors.As(runErr, &ec) {
				t.Fatalf("%s: expected *exitCodeError, got %T: %v", c.name, runErr, runErr)
			}
			if ec.code != 1 {
				t.Errorf("%s: expected exit code 1, got %d", c.name, ec.code)
			}
			if !strings.Contains(ec.Error(), "mote not found: no-such-id") {
				t.Errorf("%s: error should identify missing ID: %q", c.name, ec.Error())
			}
			if stdout != "" {
				t.Errorf("%s: stdout should be empty on not-found, got: %q", c.name, stdout)
			}
		})
	}
}
