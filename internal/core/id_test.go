// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"regexp"
	"sync"
	"testing"
)

func TestBase36Encode(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{35, "z"},
		{36, "10"},
		{1296, "100"},
	}
	for _, tt := range tests {
		got := base36Encode(tt.input)
		if got != tt.want {
			t.Errorf("base36Encode(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerateID_Format(t *testing.T) {
	types := []struct {
		moteType string
		prefix   string
	}{
		{"task", "T"},
		{"decision", "D"},
		{"lesson", "L"},
		{"context", "C"},
		{"question", "Q"},
		{"constellation", "C"},
		{"anchor", "A"},
		{"explore", "E"},
	}

	pattern := regexp.MustCompile(`^[a-z]+-[A-Z][a-z0-9]+$`)

	for _, tt := range types {
		id := GenerateID("proj", tt.moteType)

		if !pattern.MatchString(id) {
			t.Errorf("GenerateID(proj, %s) = %q, doesn't match pattern", tt.moteType, id)
		}

		// Check scope prefix
		if id[:5] != "proj-" {
			t.Errorf("GenerateID(proj, %s) = %q, missing scope prefix", tt.moteType, id)
		}

		// Check type char
		if string(id[5]) != tt.prefix {
			t.Errorf("GenerateID(proj, %s) = %q, type char got %c, want %s", tt.moteType, id, id[5], tt.prefix)
		}
	}
}

func TestGenerateID_NoConcurrentCollisions(t *testing.T) {
	const n = 1000
	ids := make([]string, n)
	var wg sync.WaitGroup

	wg.Add(n)
	for i := range n {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = GenerateID("proj", "task")
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("collision detected: %s", id)
		}
		seen[id] = true
	}
}
