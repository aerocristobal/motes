// SPDX-License-Identifier: AGPL-3.0-or-later
package skills

import _ "embed"

//go:embed mote-capture/SKILL.md
var MoteCapture []byte

//go:embed mote-retrieve/SKILL.md
var MoteRetrieve []byte

//go:embed mote-subagent/SKILL.md
var MoteSubagent []byte

//go:embed mote-plan/SKILL.md
var MotePlan []byte
