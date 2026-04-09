// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var hookHookMode bool

var hookCmd = &cobra.Command{
	Use:   "hook <event>",
	Short: "Emit lifecycle reminders for Claude Code hooks",
	Long: `Emit structured reminders at workflow boundaries.

Supported events:
  plan-approved   After ExitPlanMode: remind to create a task mote`,
	Args: cobra.ExactArgs(1),
	RunE: runHook,
}

func init() {
	hookCmd.Flags().BoolVar(&hookHookMode, "hook", false, "Wrap output in {\"additionalContext\": ...} JSON for hooks")
	rootCmd.AddCommand(hookCmd)
}

func runHook(cmd *cobra.Command, args []string) error {
	if hookHookMode {
		return runHookWrapped(args[0])
	}
	return runHookInner(args[0])
}

func runHookWrapped(event string) error {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	os.Stdout = w

	runErr := runHookInner(event)

	w.Close()
	os.Stdout = old

	captured, _ := io.ReadAll(r)
	if runErr != nil {
		return runErr
	}

	text := string(captured)
	if len(text) == 0 {
		fmt.Println("{}")
		return nil
	}

	out := struct {
		AdditionalContext string `json:"additionalContext"`
	}{AdditionalContext: text}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
	return nil
}

func runHookInner(event string) error {
	switch event {
	case "plan-approved":
		return runHookPlanApproved()
	default:
		return fmt.Errorf("unknown hook event %q", event)
	}
}

func runHookPlanApproved() error {
	root := mustFindRoot()

	// If a task mote is already active, stay silent — no nagging needed.
	if s := core.ReadSessionState(root); s != nil && s.CurrentTask != "" {
		fmt.Printf("Task mote active: %s. Proceed with implementation.\n", s.CurrentTask)
		return nil
	}

	fmt.Println("MOTE REQUIRED: Create a task mote before writing any code.")
	fmt.Println("  mote add --type=task --title=\"...\" --tag=... --body \"...\"")

	// Surface the highest-weight ready task as a suggested title.
	mm := core.NewMoteManager(root)
	ready, err := mm.List(core.ListFilters{Type: "task", Ready: true})
	if err == nil && len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool {
			return ready[i].Weight > ready[j].Weight
		})
		top := ready[0]
		fmt.Printf("\nSuggested (from mote ls --ready): %s [%s]\n", top.Title, top.ID)
	}

	return nil
}
