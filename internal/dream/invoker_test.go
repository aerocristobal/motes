// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"strings"
	"testing"
)

func TestFilterEnv_RemovesNamedVar(t *testing.T) {
	os.Setenv("MOTES_TEST_VAR", "hello")
	defer os.Unsetenv("MOTES_TEST_VAR")

	result := filterEnv("MOTES_TEST_VAR")
	for _, e := range result {
		if strings.HasPrefix(e, "MOTES_TEST_VAR=") {
			t.Error("MOTES_TEST_VAR should be removed")
		}
	}
}

func TestFilterEnv_RemovesMultiple(t *testing.T) {
	os.Setenv("MOTES_A", "1")
	os.Setenv("MOTES_B", "2")
	defer os.Unsetenv("MOTES_A")
	defer os.Unsetenv("MOTES_B")

	result := filterEnv("MOTES_A", "MOTES_B")
	for _, e := range result {
		k, _, _ := strings.Cut(e, "=")
		if k == "MOTES_A" || k == "MOTES_B" {
			t.Errorf("%s should be removed", k)
		}
	}
}

func TestFilterEnv_NoMatchReturnsAll(t *testing.T) {
	full := os.Environ()
	result := filterEnv("NONEXISTENT_VAR_XYZ_12345")
	if len(result) != len(full) {
		t.Errorf("expected %d env vars, got %d", len(full), len(result))
	}
}

func TestFilterEnv_EmptyNames(t *testing.T) {
	full := os.Environ()
	result := filterEnv()
	if len(result) != len(full) {
		t.Errorf("expected %d env vars, got %d", len(full), len(result))
	}
}
