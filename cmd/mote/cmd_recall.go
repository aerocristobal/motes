// SPDX-License-Identifier: MIT
package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var recallCmd = &cobra.Command{
	Use:   "recall <key>",
	Short: "Print the body of a saved memory",
	Long: `Print the body of the memory with the given key, followed by a
newline. Exits with code 2 if no such memory exists, to distinguish
"not found" from real I/O errors (exit code 1).`,
	Args: cobra.ExactArgs(1),
	RunE: runRecall,
}

func init() {
	rootCmd.AddCommand(recallCmd)
}

func runRecall(cmd *cobra.Command, args []string) error {
	root, err := findMemoryRoot()
	if err != nil {
		// No .memory/ at all → treat as not-found per the recall contract,
		// not as a hard error. Agents can recall in fresh repos safely.
		return &exitCodeError{code: 2, err: fmt.Errorf("memory not found: %s", args[0])}
	}
	store := core.NewMemoryStore(root)
	rec, err := store.Get(args[0])
	if err != nil {
		if errors.Is(err, core.ErrMemoryNotFound) {
			return &exitCodeError{code: 2, err: fmt.Errorf("memory not found: %s", args[0])}
		}
		return fmt.Errorf("recall memory: %w", err)
	}
	fmt.Println(rec.Body)
	return nil
}
