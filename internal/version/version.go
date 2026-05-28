// Package version is the single source of truth for the mote binary's
// version string. STORY-VERSIONS-001 introduced this package so that
// scripts/check-versions.sh and scripts/bump-version.sh have a single,
// grep-able location to read and rewrite.
//
// Any future code that needs the running version (e.g. self-checks,
// telemetry headers, doctor advisories) should import this package
// rather than embed its own string literal.
package version

// Value is the canonical version string for the mote binary. It MUST be
// kept in semver MAJOR.MINOR.PATCH form (with optional pre-release and
// build-metadata suffixes per semver 2.0.0). scripts/bump-version.sh is
// the only tool that should rewrite this literal.
const Value = "0.4.38"
