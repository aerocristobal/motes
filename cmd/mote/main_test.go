// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "motes-cli-tests-")
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "nodes"), 0o755); err != nil {
		panic(err)
	}
	os.Setenv("MOTE_GLOBAL_ROOT", tmp)
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}
