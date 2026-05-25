// SPDX-License-Identifier: MIT
package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var forgetCmd = &cobra.Command{
	Use:   "forget <key>",
	Short: "Delete a saved memory by key",
	Long: `Delete the memory with the given key. Writes a "memory.delete"
entry to the audit log. Exits with code 2 if no such memory exists.`,
	Args: cobra.ExactArgs(1),
	RunE: runForget,
}

func init() {
	rootCmd.AddCommand(forgetCmd)
}

func runForget(cmd *cobra.Command, args []string) error {
	root, err := findMemoryRoot()
	if err != nil {
		return &exitCodeError{code: 2, err: fmt.Errorf("memory not found: %s", args[0])}
	}
	store := core.NewMemoryStore(root)
	actor := core.ResolveAgentID()
	if err := store.Delete(args[0], actor); err != nil {
		if errors.Is(err, core.ErrMemoryNotFound) {
			return &exitCodeError{code: 2, err: fmt.Errorf("memory not found: %s", args[0])}
		}
		return fmt.Errorf("forget memory: %w", err)
	}
	fmt.Printf("Forgot memory %s\n", args[0])
	return nil
}
