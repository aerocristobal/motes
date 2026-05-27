// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 Scenario 3 — `--json`, `--pretty`, and `--plain` are
// mutually exclusive. Verified end-to-end through cobra so the exit code
// surfaces and the error message names the conflicting flags.
package main

import (
	"errors"
	"strings"
	"testing"
)

func TestMutex_JSONAndPlain_Exit2(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)
	plainFlag = true
	err := runLsViaCobra([]string{"ls", "--plain", "--json"})
	if err == nil {
		t.Fatal("expected mutex error, got nil")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("exit code = %d, want 2", ec.code)
	}
	if !strings.Contains(err.Error(), "--json") || !strings.Contains(err.Error(), "--plain") {
		t.Errorf("error must name conflicting flags; got %q", err.Error())
	}
}

func TestMutex_JSONAndPretty_Exit2(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)
	prettyFlag = true
	err := runLsViaCobra([]string{"ls", "--pretty", "--json"})
	if err == nil {
		t.Fatal("expected mutex error, got nil")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("exit code = %d, want 2", ec.code)
	}
}

func TestMutex_PrettyAndPlain_Exit2(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)
	plainFlag = true
	prettyFlag = true
	err := runLsViaCobra([]string{"ls", "--plain", "--pretty"})
	if err == nil {
		t.Fatal("expected mutex error, got nil")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("exit code = %d, want 2", ec.code)
	}
}

func TestMutex_AllThree_Exit2(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	resetModeFlags(t)
	plainFlag = true
	prettyFlag = true
	err := runLsViaCobra([]string{"ls", "--plain", "--pretty", "--json"})
	if err == nil {
		t.Fatal("expected mutex error, got nil")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("exit code = %d, want 2", ec.code)
	}
	msg := err.Error()
	for _, flag := range []string{"--json", "--pretty", "--plain"} {
		if !strings.Contains(msg, flag) {
			t.Errorf("error must name %q; got %q", flag, msg)
		}
	}
}
