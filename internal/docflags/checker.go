// SPDX-License-Identifier: MIT
package docflags

import (
	"fmt"
	"strings"
)

// Check resolves every Reference against the live CLI surface and
// the removed-flags manifest, and returns one Violation per bad
// reference.  Returns an empty slice when every reference is valid.
//
// Resolution order per reference:
//  1. If (resolved-cmd, flag) is in the surface → OK.
//  2. If flag is a root-level persistent flag → OK (Scenario 6).
//  3. If (resolved-cmd, flag) is in the removed manifest AND the
//     reference is allowlisted → OK (Scenarios 3, 7).
//  4. If (resolved-cmd, flag) is in the removed manifest AND NOT
//     allowlisted → violation, reason names the flag as removed
//     (Scenario 4).
//  5. If the command path cannot be resolved against the surface
//     at all → violation, reason names it an unknown command.
//  6. Otherwise → violation, reason names the flag as unknown for
//     the resolved command (Scenarios 2, 5).
func Check(
	refs []Reference,
	surface CLISurface,
	removed map[string]map[string]bool,
	fileLines map[string][]string,
	tokens []string,
) []Violation {
	cmdMap := buildCommandMap(surface)
	persistent := buildPersistentSet(surface)
	if len(tokens) == 0 {
		tokens = DefaultTokens
	}

	var violations []Violation
	for _, ref := range refs {
		resolvedCmd, knownCmd := resolveCommand(ref.Command, cmdMap)

		// Persistent flags are valid on any (known or unknown)
		// command, because Cobra accepts them globally. But when the
		// command itself is unknown, the doc is still broken — Cobra
		// would error before getting to the flag. Treat unknown
		// command as a violation regardless of the flag's persistence.
		if !knownCmd {
			violations = append(violations, Violation{
				Reference: ref,
				Reason:    fmt.Sprintf("unknown command: %q", ref.Command),
			})
			continue
		}

		if cmdMap[resolvedCmd][ref.Flag] {
			continue
		}
		if persistent[ref.Flag] {
			continue
		}

		if isRemoved(removed, resolvedCmd, ref.Flag) {
			if IsAllowlisted(ref, fileLines[ref.Path], tokens) {
				continue
			}
			violations = append(violations, Violation{
				Reference: ref,
				Reason: fmt.Sprintf(
					"removed flag %s for %q, not in allowlisted context",
					ref.Flag, resolvedCmd),
			})
			continue
		}

		violations = append(violations, Violation{
			Reference: ref,
			Reason:    fmt.Sprintf("unknown flag %s for command %q", ref.Flag, resolvedCmd),
		})
	}
	return violations
}

// buildCommandMap lowers the surface into a name → set(flag) map for
// O(1) (cmd, flag) lookup.
func buildCommandMap(s CLISurface) map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(s.Commands))
	for _, c := range s.Commands {
		set := make(map[string]bool, len(c.Flags))
		for _, f := range c.Flags {
			set[f.Name] = true
		}
		out[c.Name] = set
	}
	return out
}

func buildPersistentSet(s CLISurface) map[string]bool {
	out := make(map[string]bool, len(s.Persistent))
	for _, f := range s.Persistent {
		out[f.Name] = true
	}
	return out
}

// resolveCommand picks the longest-prefix match of candidate against
// the known command names.  This handles positional args that the
// scanner couldn't distinguish from subcommand names — e.g. the
// candidate `add foo bar` resolves to `add` because the surface has
// no `add foo` or `add foo bar` command.
func resolveCommand(candidate string, cmdMap map[string]map[string]bool) (string, bool) {
	if candidate == "" {
		return "", false
	}
	parts := strings.Fields(candidate)
	for n := len(parts); n > 0; n-- {
		try := strings.Join(parts[:n], " ")
		if _, ok := cmdMap[try]; ok {
			return try, true
		}
	}
	return "", false
}

func isRemoved(removed map[string]map[string]bool, cmd, flag string) bool {
	if removed == nil {
		return false
	}
	set, ok := removed[cmd]
	if !ok {
		return false
	}
	return set[flag]
}
