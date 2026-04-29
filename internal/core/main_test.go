// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain sandboxes every test in this package so they cannot accidentally
// touch the developer's real ~/.motes/. Tests that need to exercise the
// unset-MOTE_GLOBAL_ROOT path (migrate/global routing) override HOME themselves
// via t.Setenv and explicitly clear MOTE_GLOBAL_ROOT inside the test body.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "motes-core-tests-")
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
