// SPDX-License-Identifier: MIT
package main

import (
	"testing"

	"motes/internal/jsonenv"
)

// resetJsonenvForTest clears the jsonenv mode + deprecation-once caches both
// immediately AND on test cleanup. Callers must thread `*testing.T` so the
// deferred reset survives across the whole `go test` run — otherwise the mode
// chosen by one envelope test (cached via sync.Once) leaks into unrelated
// tests that assume legacy mode.
func resetJsonenvForTest(t *testing.T) {
	t.Helper()
	jsonenv.ResetForTest()
	t.Cleanup(jsonenv.ResetForTest)
}
