// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
)

var memoriesCmd = &cobra.Command{
	Use:   "memories [substring]",
	Short: "List saved memories, optionally filtered by substring",
	Long: `List every saved memory, sorted ascending by key. With an optional
substring argument, restrict output to memories whose key OR body
contains the substring (case-insensitive).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMemories,
}

var memoriesJSON bool

// memoryBodySnippet is the body width used in compact text rows. Long
// enough to fit a one-liner rule, short enough to keep prime output
// tight when this list grows.
const memoryBodySnippet = 100

func init() {
	memoriesCmd.Flags().BoolVar(&memoriesJSON, "json", false, "Emit JSON object {\"memories\":[...]}")
	rootCmd.AddCommand(memoriesCmd)
}

func runMemories(cmd *cobra.Command, args []string) error {
	mode, err := outputMode(memoriesJSON)
	if err != nil {
		return err
	}

	root, err := findMemoryRoot()
	if err != nil {
		// No .memory/ → treat as empty list, matching how `mote ls` behaves
		// in fresh repos.
		if mode == ModeJSON {
			fmt.Println(`{"memories":[]}`)
		}
		return nil
	}
	store := core.NewMemoryStore(root)
	substr := ""
	if len(args) == 1 {
		substr = args[0]
	}
	records, err := store.List(substr)
	if err != nil {
		return fmt.Errorf("list memories: %w", err)
	}

	if mode == ModeJSON {
		return emitMemoriesJSON(records)
	}

	if mode == ModePlain {
		// STORY-PLAIN-001: no padding, one record per line. Body still truncated
		// to memoryBodySnippet — the snippet is a UX contract for the listing
		// view across both pretty and plain modes; full bodies live behind
		// `mote recall <key>`.
		for _, r := range records {
			fmt.Printf("%s: %s\n", r.Key, format.Truncate(r.Body, memoryBodySnippet))
		}
		return nil
	}

	if len(records) == 0 {
		fmt.Println("(no memories)")
		return nil
	}

	keyWidth := 0
	for _, r := range records {
		if len(r.Key) > keyWidth {
			keyWidth = len(r.Key)
		}
	}
	for _, r := range records {
		fmt.Printf("  %-*s  %s\n", keyWidth, r.Key, format.Truncate(r.Body, memoryBodySnippet))
	}
	return nil
}

func emitMemoriesJSON(records []core.MemoryRecord) error {
	out := struct {
		Memories []memoryJSONRow `json:"memories"`
	}{Memories: make([]memoryJSONRow, 0, len(records))}
	for _, r := range records {
		out.Memories = append(out.Memories, memoryJSONRow{
			Key:       r.Key,
			Body:      r.Body,
			CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal memories: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

type memoryJSONRow struct {
	Key       string `json:"key"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
