// SPDX-License-Identifier: MIT

// Package githooks owns the per-project git-hook scripts that the mote binary
// installs into consuming projects' .git/hooks/ directory. The templates are
// embedded with //go:embed so hooks travel with the binary and are repaired
// by `mote githooks install` (or `mote doctor --fix`) whenever the binary is
// upgraded.
//
// This package is distinct from internal/githook (singular), which holds
// tests for the contributor-only .githooks/pre-commit script that mote's own
// repository uses via `git config core.hooksPath .githooks`. That contributor
// hook is Go-specific (gofmt + golangci-lint) and is never shipped into
// end-user projects.
//
// See STORY-HOOKINST-001 for the design rationale.
package githooks
