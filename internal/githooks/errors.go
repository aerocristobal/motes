// SPDX-License-Identifier: MIT

package githooks

import "errors"

// ErrConflict is returned when a hook file in .git/hooks/ exists without
// the mote-managed sentinel — i.e. it was authored by the developer or by
// another tool. The install path never overwrites such files unless
// InstallOpts.Force is set, and `mote doctor --fix` never touches them.
var ErrConflict = errors.New("user-authored hook present without mote sentinel")

// ErrNotGitRepo is returned by Install when no .git directory (or .git
// gitfile) is found in repoRoot or any parent. Callers in onboard's path
// degrade this to a warning; the CLI surface maps it to exit code 3.
var ErrNotGitRepo = errors.New("not a git repository")

// ErrTemplateLoad signals that an embedded hook template could not be read
// or is malformed (e.g. missing the shebang line the sentinel injector needs
// to split on). This is a programmer error in mote itself; callers map it
// to exit code 1.
var ErrTemplateLoad = errors.New("failed to load embedded hook template")
