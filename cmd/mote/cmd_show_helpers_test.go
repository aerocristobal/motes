// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

// readAccessBatchEntriesFromDir parses .access_batch.jsonl under memDir into
// AccessBatchEntry records. Returns nil if the file is missing. Used by
// CLI-level tests that want to assert the action/agent fields written by
// runShow's access-log calls.
func readAccessBatchEntriesFromDir(t *testing.T, memDir string) []core.AccessBatchEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(memDir, ".access_batch.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read access batch: %v", err)
	}
	var out []core.AccessBatchEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e core.AccessBatchEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("parse access batch line %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

// captureBoth captures stdout and stderr around fn. Returned in (stdout, stderr) order.
// (A package-level captureStderr already exists in cmd_update_claim_test.go;
// this variant captures both streams without requiring a *testing.T.)
func captureBoth(fn func()) (string, string) {
	oldOut, oldErr := os.Stdout, os.Stderr
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	os.Stdout = wo
	os.Stderr = we

	fn()

	_ = wo.Close()
	_ = we.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr
	outBytes, _ := io.ReadAll(ro)
	errBytes, _ := io.ReadAll(re)
	return string(outBytes), string(errBytes)
}

// createDeterministicMote writes a mote YAML fixture with stable timestamps
// and identifiers directly to .memory/nodes/<id>.md, bypassing mm.Create()'s
// time.Now() / random-ID calls. After writing, the index is rebuilt so the
// fixture is visible to subsequent reads.
//
// The fixture is intentionally minimal — one acceptance criterion, one tag,
// fixed RFC3339 timestamps — so the snapshot golden file is small and easy
// to diff. Tests requiring richer state should compose additional fixtures.
func createDeterministicMote(t *testing.T, root, id, title string) {
	t.Helper()
	content := fmt.Sprintf(`---
id: %s
type: task
status: active
title: %s
tags:
    - testing
weight: 0.5
origin: normal
action: ""
created_at: 2026-01-01T00:00:00Z
last_accessed: 2026-01-02T00:00:00Z
access_count: 7
depends_on: []
blocks: []
relates_to: []
builds_on: []
contradicts: []
supersedes: []
caused_by: []
informed_by: []
acceptance:
    - First criterion
acceptance_met:
    - false
---
Body content.
`, id, title)
	path := filepath.Join(root, "nodes", id+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write deterministic mote: %v", err)
	}
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	motes, err := mm.ReadAllParallel()
	if err != nil {
		t.Fatalf("read motes after deterministic create: %v", err)
	}
	if err := im.Rebuild(motes); err != nil {
		t.Fatalf("rebuild index after deterministic create: %v", err)
	}
}

// resetShowFlags clears all show-command flag state. Call before AND after
// any test that mutates these globals, to avoid leakage into sibling tests.
func resetShowFlags() {
	showJSON = false
	showShort = false
	showLong = false
	showASCII = false
	showExecutionOnly = false
}
