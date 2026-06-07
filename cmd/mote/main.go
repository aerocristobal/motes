// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/format"
	"motes/internal/jsonenv"
	"motes/internal/version"
)

var rootCmd = &cobra.Command{
	Use:     "mote",
	Short:   "AI-native context and memory system",
	Long:    "Motes is an AI-native context and memory system. Knowledge is stored as atomic units (motes) linked in two dimensions: dependency links and semantic links.",
	Version: version.Value,
}

// noColorFlag is bound to the persistent --no-color flag on rootCmd. Renderers
// pass it through format.ShouldColor along with TTY detection.
var noColorFlag bool

// plainFlag and prettyFlag are the persistent layout-mode flags introduced by
// STORY-PLAIN-001. They are orthogonal to --no-color (a color axis), and they
// are mutually exclusive with each other and with --json (per-command). See
// outputMode() for the resolution logic.
var (
	plainFlag  bool
	prettyFlag bool
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false,
		"Disable ANSI color/styling in human-readable output")
	rootCmd.PersistentFlags().BoolVar(&plainFlag, "plain", false,
		"Colorless, line-oriented output for pipelines (mutually exclusive with --json and --pretty)")
	rootCmd.PersistentFlags().BoolVar(&prettyFlag, "pretty", false,
		"Force ANSI + Tufte-styled output even on non-TTY (mutually exclusive with --json and --plain)")

	// STORY-COLOR-001: feed the resolved color decision into the format
	// package once, after persistent flags are parsed, so the 1-arg
	// semantic tokens (format.Pass / Warn / Fail / Accent / Command) can
	// consult a single source of truth without each command threading
	// useColor through. The 2-arg format.Muted callers continue to pass
	// useColorOutput() explicitly and are unaffected.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		format.SetColorEnabled(useColorOutput())
		return nil
	}
}

// useColorOutput decides whether the current invocation should emit ANSI styles
// for human-readable output on stdout. Combines TTY detection (with the
// MOTE_FORCE_TTY=1 internal test override), the NO_COLOR env var, and the
// persistent --no-color and --pretty flags. --pretty forces color on even when
// stdout is not a TTY; --no-color always wins.
func useColorOutput() bool {
	if noColorFlag {
		return false
	}
	if prettyFlag {
		return true
	}
	return format.ShouldColor(format.IsTTY(os.Stdout.Fd()), noColorFlag)
}

// OutputMode is the single source of truth for the rendering mode of a read
// command. ModeAuto is the no-mode-flag default and preserves pre-STORY-PLAIN
// behaviour (TTY → styled, non-TTY → no color but layout unchanged). The other
// three modes are user-explicit.
type OutputMode int

const (
	ModeAuto OutputMode = iota
	ModeJSON
	ModePretty
	ModePlain
)

// outputMode resolves the rendering mode for a JSON-capable RunE. jsonFlag is
// the per-command --json bool (passed because not every command has one — recall,
// for instance, only accepts the persistent --plain/--pretty as no-ops). Returns
// an *exitCodeError with code 2 when more than one mode flag is set, matching
// STORY-PLAIN-001 Scenario 3.
func outputMode(jsonFlag bool) (OutputMode, error) {
	var picked []string
	if jsonFlag {
		picked = append(picked, "--json")
	}
	if prettyFlag {
		picked = append(picked, "--pretty")
	}
	if plainFlag {
		picked = append(picked, "--plain")
	}
	if len(picked) > 1 {
		return ModeAuto, &exitCodeError{
			code: 2,
			err:  fmt.Errorf("output modes are mutually exclusive: %s", strings.Join(picked, ", ")),
		}
	}
	switch {
	case jsonFlag:
		return ModeJSON, nil
	case prettyFlag:
		return ModePretty, nil
	case plainFlag:
		return ModePlain, nil
	default:
		return ModeAuto, nil
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Envelope mode (STORY-JSCHEMA-001): a JSON-emitting command in
		// envelope mode returns *jsonenv.EnvelopedError so we can serialize
		// the error envelope onto stderr instead of printing prose. Order
		// matters — check the more specific type first.
		var ee *jsonenv.EnvelopedError
		if errors.As(err, &ee) {
			data, mErr := json.Marshal(jsonenv.WrapError(ee.Code, ee.Message))
			if mErr != nil {
				fmt.Fprintln(os.Stderr, ee.Message)
				os.Exit(ee.ExitCode)
			}
			fmt.Fprintln(os.Stderr, string(data))
			os.Exit(ee.ExitCode)
		}
		var ec *exitCodeError
		if errors.As(err, &ec) {
			fmt.Fprintln(os.Stderr, ec.err)
			os.Exit(ec.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
