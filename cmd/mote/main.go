// SPDX-License-Identifier: MIT
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"motes/internal/format"
)

var rootCmd = &cobra.Command{
	Use:     "mote",
	Short:   "AI-native context and memory system",
	Long:    "Motes is an AI-native context and memory system. Knowledge is stored as atomic units (motes) linked in two dimensions: dependency links and semantic links.",
	Version: "0.4.31",
}

// noColorFlag is bound to the persistent --no-color flag on rootCmd. Renderers
// pass it through format.ShouldColor along with TTY detection.
var noColorFlag bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false,
		"Disable ANSI color/styling in human-readable output")
}

// useColorOutput decides whether the current invocation should emit ANSI styles
// for human-readable output on stdout. Combines TTY detection (with the
// MOTE_FORCE_TTY=1 internal test override), the NO_COLOR env var, and the
// persistent --no-color flag.
func useColorOutput() bool {
	return format.ShouldColor(format.IsTTY(os.Stdout.Fd()), noColorFlag)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		var ec *exitCodeError
		if errors.As(err, &ec) {
			fmt.Fprintln(os.Stderr, ec.err)
			os.Exit(ec.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
