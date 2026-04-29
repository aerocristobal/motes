// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain sandboxes the dream package's tests so the safety guard in
// core.GlobalRoot doesn't fire when a test indirectly calls LoadConfig.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "motes-dream-tests-")
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
