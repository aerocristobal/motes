// SPDX-License-Identifier: MIT
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testBinaryPath is the path to a freshly built `mote` binary, populated by
// TestMain. Tests that need to assert real exit codes or subprocess stderr
// (e.g. non-prime commands that call os.Exit via mustFindRoot) use this.
var testBinaryPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "motes-cli-tests-")
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "nodes"), 0o755); err != nil {
		panic(err)
	}
	os.Setenv("MOTE_GLOBAL_ROOT", tmp)

	// Build a binary once per package run for subprocess-based tests.
	testBinaryPath = filepath.Join(tmp, "mote")
	build := exec.Command("go", "build", "-o", testBinaryPath, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("test setup: go build failed: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}
