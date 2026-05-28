// SPDX-License-Identifier: MIT
package main

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"motes/internal/docflags"
)

// walkCLISurface walks the Cobra command tree rooted at root and
// returns a docflags.CLISurface that lists every command's accepted
// flags plus the root-level persistent flag set.
//
// Conventions:
//   - The auto-injected `--help` flag is recorded once in Persistent,
//     not on every command (it propagates to every subcommand).
//   - The auto-injected `--version` flag (only present on root if
//     root.Version != "") is also recorded in Persistent.
//   - Cobra's built-in `help` and `completion` subcommands are skipped
//     — docs do not use them as references, and including them just
//     adds noise (and brings their flags into the surface).
//   - Hidden commands ARE included: docs may legitimately reference
//     a hidden diagnostic command like `mote __dump-cli-surface`.
//   - For each non-root command, the function records its local
//     non-persistent flags AND its own persistent flags (the latter
//     because they propagate to that command's descendants and we
//     want to count them as valid references on the command that
//     defines them).
//
// The returned surface is deterministic: commands are sorted by name
// and flags within each command are sorted by name.
func walkCLISurface(root *cobra.Command) docflags.CLISurface {
	// Initialize Cobra's lazy default flags so they appear in the
	// pflag iteration below.
	root.InitDefaultHelpFlag()
	root.InitDefaultVersionFlag()

	surface := docflags.CLISurface{}
	seenPersistent := map[string]bool{}

	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if seenPersistent[f.Name] {
			return
		}
		seenPersistent[f.Name] = true
		surface.Persistent = append(surface.Persistent, docflags.FlagSpec{
			Name:       "--" + f.Name,
			Deprecated: f.Deprecated != "",
		})
	})

	// --help and --version are auto-injected by Cobra and applied to
	// every command. Record them as persistent so doc references on
	// any subcommand resolve.
	if !seenPersistent["help"] {
		surface.Persistent = append(surface.Persistent, docflags.FlagSpec{Name: "--help"})
	}
	if root.Version != "" && !seenPersistent["version"] {
		surface.Persistent = append(surface.Persistent, docflags.FlagSpec{Name: "--version"})
	}

	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		if c != root {
			c.InitDefaultHelpFlag()
			seenFlag := map[string]bool{}
			var flags []docflags.FlagSpec
			collect := func(f *pflag.Flag) {
				if f.Name == "help" {
					return // covered by Persistent
				}
				if seenFlag[f.Name] {
					return
				}
				seenFlag[f.Name] = true
				flags = append(flags, docflags.FlagSpec{
					Name:       "--" + f.Name,
					Deprecated: f.Deprecated != "",
				})
			}
			c.LocalNonPersistentFlags().VisitAll(collect)
			c.PersistentFlags().VisitAll(collect)

			sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
			surface.Commands = append(surface.Commands, docflags.CommandSpec{
				Name:  joinPath(path),
				Flags: flags,
			})
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			walk(sub, append(path, sub.Name()))
		}
	}
	walk(root, nil)

	sort.Slice(surface.Commands, func(i, j int) bool {
		return surface.Commands[i].Name < surface.Commands[j].Name
	})
	sort.Slice(surface.Persistent, func(i, j int) bool {
		return surface.Persistent[i].Name < surface.Persistent[j].Name
	})
	return surface
}

func joinPath(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += " " + p
	}
	return out
}
