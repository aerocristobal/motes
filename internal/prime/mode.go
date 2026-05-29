// SPDX-License-Identifier: MIT
// Package prime carries the shared building blocks for the `mote prime`
// command: mode/source enums, the MCP-server detection routine that drives
// auto-mode selection, and the brief MCP-mode renderer.
//
// The full CLI-mode renderer lives in cmd/mote/cmd_prime.go for now because
// it is intimately bound to the scoring/traversal pipeline and to package
// globals owned by the cobra layer. Extracting only the new pieces here
// (detection + brief renderer) keeps STORY-ADAPRIME-001 surgical and
// preserves the byte-for-byte CLI-mode parity guarantee from Scenario 2.
package prime

// Mode is the resolved output mode of a `mote prime` invocation.
//
// ModeMCP emits the brief workflow reminder (~50 tokens) on the assumption
// that the agent host is running an MCP server for motes and can call tools
// for detail on demand. ModeCLI emits the full payload (~1–2k tokens) used
// when no MCP integration is present.
type Mode string

const (
	ModeCLI Mode = "cli"
	ModeMCP Mode = "mcp"
)

// ModeSource records whether the resolved Mode came from auto-detection or
// from an explicit --mcp/--full flag. Surfaced in the JSON envelope so
// downstream consumers can tell "the user told me to be brief" from
// "I inferred I should be brief".
type ModeSource string

const (
	SourceAuto ModeSource = "auto"
	SourceFlag ModeSource = "flag"
)

// ResolveMode applies the STORY-ADAPRIME-001 precedence rule:
//
//	explicit flag > auto-detection > default (cli)
//
// --mcp and --full are mutually exclusive — the caller MUST reject that
// combination before calling here (Scenario 6 wants the rejection to
// happen before any prime body is emitted). When both are passed, this
// returns the --full result so the function is total; callers that depend
// on the mutex are expected to gate first.
func ResolveMode(mcpFlag, fullFlag, detected bool) (Mode, ModeSource) {
	switch {
	case fullFlag:
		return ModeCLI, SourceFlag
	case mcpFlag:
		return ModeMCP, SourceFlag
	case detected:
		return ModeMCP, SourceAuto
	default:
		return ModeCLI, SourceAuto
	}
}
