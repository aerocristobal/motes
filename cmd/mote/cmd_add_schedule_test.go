// SPDX-License-Identifier: MIT
//
// STORY-TIME-001 §6.3 — `mote add --due / --defer` CLI integration.
//
// Each test seeds an isolated .memory/ via setupIntegrationTest, drives the
// `add` cobra command through runAddViaCobra, and asserts on the persisted
// mote file by re-parsing it from disk.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

func TestRunAdd_DueFlag_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	if err := runAddViaCobra([]string{
		"add", "--type=task", "--title=ship report", "--body=x", "--due=+2d",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	m := mustFindFirstMote(t, root)
	if m.DueAt == nil {
		t.Fatal("DueAt should be set")
	}
	// Tolerance: the CLI uses real time.Now(), so we can only assert the
	// distance from now is roughly 2 days. Use a generous window.
	delta := time.Until(m.DueAt)
	if delta < 47*time.Hour || delta > 49*time.Hour {
		t.Errorf("DueAt distance from now: got %v, want ~48h", delta)
	}
}

func TestRunAdd_DeferFlag_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	if err := runAddViaCobra([]string{
		"add", "--type=task", "--title=follow up", "--body=x", "--defer=+6h",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	m := mustFindFirstMote(t, root)
	if m.DeferUntil == nil {
		t.Fatal("DeferUntil should be set")
	}
	delta := time.Until(m.DeferUntil)
	if delta < 5*time.Hour+30*time.Minute || delta > 6*time.Hour+30*time.Minute {
		t.Errorf("DeferUntil distance from now: got %v, want ~6h", delta)
	}
}

func TestRunAdd_BothFlags_BothFieldsSet(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	if err := runAddViaCobra([]string{
		"add", "--type=task", "--title=bridge", "--body=x", "--due=+1w", "--defer=+2d",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	m := mustFindFirstMote(t, root)
	if m.DueAt == nil || m.DeferUntil == nil {
		t.Fatalf("both fields should be set: due=%v defer=%v", m.DueAt, m.DeferUntil)
	}
	if !m.DeferUntil.Before(*m.DueAt) {
		t.Errorf("defer (%v) should precede due (%v)", m.DeferUntil, m.DueAt)
	}
}

func TestRunAdd_DueInPast_Accepted(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	if err := runAddViaCobra([]string{
		"add", "--type=task", "--title=missed", "--body=x", "--due=2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	m := mustFindFirstMote(t, root)
	if m.DueAt == nil || m.DueAt.After(time.Now()) {
		t.Errorf("expected past DueAt, got %v", m.DueAt)
	}
}

func TestRunAdd_DeferInPast_Rejected(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := runAddViaCobra([]string{
		"add", "--type=task", "--title=t", "--body=x", "--defer=-1h",
	})
	if err == nil {
		t.Fatal("expected error for past defer")
	}
	// Either the parser rejects -1h (negative relative) OR (if a user got past
	// the parser somehow) the create-time validation rejects "defer must be
	// in the future". Both shapes are acceptable per the story.
	msg := err.Error()
	if !strings.Contains(msg, "future") && !strings.Contains(msg, "invalid time") {
		t.Errorf("error should mention future or invalid time; got: %v", err)
	}
}

func TestRunAdd_InvalidTimeString_ExitNonZero(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cases := []string{
		"yesterday",
		"$(rm -rf ~)",
		"../../etc/passwd",
		"tomrrow",
		"not a date",
	}
	for _, v := range cases {
		err := runAddViaCobra([]string{
			"add", "--type=task", "--title=t", "--body=x", "--due=" + v,
		})
		if err == nil {
			t.Errorf("--due=%q should error", v)
			continue
		}
		if !strings.Contains(err.Error(), "invalid time") {
			t.Errorf("--due=%q: expected 'invalid time' in error, got %v", v, err)
		}
	}
}

// mustFindFirstMote scans .memory/nodes/ and returns the first parsed mote.
// Test helper for "I just created exactly one mote, give it back to me".
func mustFindFirstMote(t *testing.T, root string) *core.Mote {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("read nodes/: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		m, err := core.ParseMote(filepath.Join(root, "nodes", e.Name()))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return m
	}
	t.Fatal("no mote found in nodes/")
	return nil
}
