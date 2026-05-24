// SPDX-License-Identifier: MIT

// Package claudehooks embeds the Claude Code PreToolUse safety hook scripts
// shipped by `mote onboard`. See docs/beads-recommendations.md §1 for the
// rationale.
package claudehooks

import _ "embed"

//go:embed block-interactive-cmds.sh
var BlockInteractiveCmds []byte

//go:embed block-gh-watch.sh
var BlockGhWatch []byte

//go:embed block-mote-rm.sh
var BlockMoteRm []byte
