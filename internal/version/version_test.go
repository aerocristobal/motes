// Package version_test asserts the canonical version constant is well-formed.
// scripts/check-versions.sh reads this constant; if it ever lost its value
// or stopped matching semver, every downstream consumer would silently break.
//
// Story: STORY-VERSIONS-001.
package version_test

import (
	"regexp"
	"testing"

	"motes/internal/version"
)

// semver 2.0.0 (MAJOR.MINOR.PATCH with optional pre-release and build).
var semverRE = regexp.MustCompile(
	`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`,
)

func TestValue_NonEmpty(t *testing.T) {
	if version.Value == "" {
		t.Fatal("version.Value must not be empty")
	}
}

func TestValue_MatchesSemver(t *testing.T) {
	if !semverRE.MatchString(version.Value) {
		t.Fatalf("version.Value %q is not valid semver", version.Value)
	}
}
