// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// origHomeAtStart is the value of $HOME when the process started, captured before
// any test could redirect it via t.Setenv. Used by the test-isolation guard in
// GlobalRoot to detect tests that forgot to redirect HOME or set MOTE_GLOBAL_ROOT.
var origHomeAtStart string

func init() {
	origHomeAtStart, _ = os.UserHomeDir()
}

var (
	isTestBinaryOnce sync.Once
	isTestBinaryFlag bool
)

// IsTestBinary reports whether the current process is a Go test binary.
// Detection is conservative: binary name ends in `.test`, contains `__debug_bin`
// (delve), or any argv flag begins with `-test.`.
func IsTestBinary() bool {
	isTestBinaryOnce.Do(func() {
		if len(os.Args) == 0 {
			return
		}
		base := filepath.Base(os.Args[0])
		if strings.HasSuffix(base, ".test") || strings.Contains(base, "__debug_bin") {
			isTestBinaryFlag = true
			return
		}
		for _, a := range os.Args[1:] {
			if strings.HasPrefix(a, "-test.") {
				isTestBinaryFlag = true
				return
			}
		}
	})
	return isTestBinaryFlag
}

// assertTestSafeHome panics if the process is a test binary that would resolve
// $HOME to the developer's real home directory — meaning a forgotten t.Setenv
// would write into the user's actual ~/.motes/. Callers should hit this before
// any path resolution that touches the global memory store.
func assertTestSafeHome() {
	if !IsTestBinary() {
		return
	}
	if os.Getenv("MOTE_GLOBAL_ROOT") != "" {
		return
	}
	cur, _ := os.UserHomeDir()
	if cur != "" && cur == origHomeAtStart {
		panic("motes: test binary resolving GlobalRoot against the real $HOME — set MOTE_GLOBAL_ROOT or t.Setenv(\"HOME\", t.TempDir()) before calling")
	}
}

// minPromoteBodyChars is the floor for non-whitespace body content on a global
// promotion. Picked at 30 to match the triage threshold that flagged stub motes
// like "Body one" while preserving real one-liner decisions.
const minPromoteBodyChars = 30

// syntheticTitleRE matches titles that are almost certainly test fixtures or
// stubs left behind by past test runs. Keep this conservative — `--force` exists
// for the rare false positive. Each branch is anchored with ^...$ via the outer
// group so a synthetic match is the entire title, not a substring.
var syntheticTitleRE = regexp.MustCompile(`(?i)^(` +
	`mote( \d+)?` + `|` +
	`lesson( [a-z0-9]+)?` + `|` +
	`decision( [a-z0-9]+)?` + `|` +
	`test( [a-z0-9]+)?` + `|` +
	`updated` + `|` +
	`target( mote)?` + `|` +
	`access test( [a-z]+)?` + `|` +
	`bm25( [a-z]+){1,3}` + `|` +
	`auth (mote|context)( \d+)?` + `|` +
	`auth lesson( [a-z])?` + `|` +
	`db mote` + `|` +
	`old (mote|context)` + `|` +
	`body [a-z]+` +
	`)$`)

// IsLikelyTestTitle reports whether title matches a synthetic test-fixture
// pattern. Empty titles return false (a separate validation rule covers them).
func IsLikelyTestTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return false
	}
	t = strings.Trim(t, "'\"")
	return syntheticTitleRE.MatchString(t)
}

// BodyChars returns the count of non-whitespace runes in body. Used to enforce
// minimum content on promotion.
func BodyChars(body string) int {
	n := 0
	for _, r := range body {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\v' && r != '\f' {
			n++
		}
	}
	return n
}
