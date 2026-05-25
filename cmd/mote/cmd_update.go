// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var updateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a mote's fields",
	Args:  cobra.ExactArgs(1),
	RunE:  runUpdate,
}

var (
	updateStatus string
	updateTitle  string
	updateWeight float64
	updateAddTag []string
	updateBody   string
	updateAccept []string
	updateSize   string
	updateParent string
	updateForce  bool
	updateQuiet  bool
	updateClaim  bool
	updateJSON   bool

	updateExecutionAgentType       string
	updateExecutionSuggestedModel  string
	updateExecutionReasoningEffort string
	updateExecutionMode            string
	updateExecutionParallelGroup   string
)

// fieldMutationFlags lists the flags that are mutually exclusive with --claim.
var fieldMutationFlags = []string{
	"status", "title", "weight", "add-tag", "body", "accept", "size", "parent",
	"execution-agent-type", "execution-suggested-model",
	"execution-reasoning-effort", "execution-mode", "execution-parallel-group",
}

func init() {
	updateCmd.Flags().StringVar(&updateStatus, "status", "", "New status (active|in_progress|completed|archived|deprecated)")
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "New title")
	updateCmd.Flags().Float64Var(&updateWeight, "weight", 0, "New weight (0.0-1.0)")
	updateCmd.Flags().StringSliceVar(&updateAddTag, "add-tag", nil, "Tag to append (repeatable)")
	updateCmd.Flags().StringVar(&updateBody, "body", "", "New body content")
	updateCmd.Flags().StringSliceVar(&updateAccept, "accept", nil, "Acceptance criterion to append (repeatable)")
	updateCmd.Flags().StringVar(&updateSize, "size", "", "Effort size (xs|s|m|l|xl)")
	updateCmd.Flags().StringVar(&updateParent, "parent", "", "Parent mote ID")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Bypass security scan blocks (for false positives)")
	updateCmd.Flags().BoolVar(&updateQuiet, "quiet", false, "Suppress security scan warnings on stderr")
	updateCmd.Flags().BoolVar(&updateClaim, "claim", false, "Atomically claim a ready task: status=active → in_progress, stamp claimed_by from MOTE_AGENT_ID")
	updateCmd.Flags().BoolVar(&updateJSON, "json", false, "Emit result as JSON (currently only meaningful with --claim)")

	updateCmd.Flags().StringVar(&updateExecutionAgentType, "execution-agent-type", "", "Orchestration hint: subagent type (empty = clear)")
	updateCmd.Flags().StringVar(&updateExecutionSuggestedModel, "execution-suggested-model", "", "Orchestration hint: suggested model (empty = clear)")
	updateCmd.Flags().StringVar(&updateExecutionReasoningEffort, "execution-reasoning-effort", "", "Orchestration hint: reasoning effort low|medium|high (empty = clear)")
	updateCmd.Flags().StringVar(&updateExecutionMode, "execution-mode", "", "Orchestration hint: local|delegated|parallel (empty = clear)")
	updateCmd.Flags().StringVar(&updateExecutionParallelGroup, "execution-parallel-group", "", "Orchestration hint: parallel-group identifier (empty = clear)")

	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	moteID := args[0]

	if cmd.Flags().Changed("claim") {
		for _, f := range fieldMutationFlags {
			if cmd.Flags().Changed(f) {
				return &exitCodeError{code: 1,
					err: fmt.Errorf("--claim is mutually exclusive with --%s", f)}
			}
		}
		return runUpdateClaim(moteID)
	}

	anyFlagChanged := false
	for _, f := range fieldMutationFlags {
		if cmd.Flags().Changed(f) {
			anyFlagChanged = true
			break
		}
	}
	if !anyFlagChanged {
		return fmt.Errorf("at least one flag required: --status, --title, --weight, --add-tag, --body, --accept, --size, --parent, --execution-agent-type, --execution-suggested-model, --execution-reasoning-effort, --execution-mode, --execution-parallel-group, --claim")
	}

	// Validate mote ID
	if err := security.ValidateMoteID(moteID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	// Validate input parameters
	if cmd.Flags().Changed("status") {
		if err := security.ValidateEnum(updateStatus, core.ValidStatuses, "status"); err != nil {
			return fmt.Errorf("invalid status: %w", err)
		}
	}

	if cmd.Flags().Changed("title") {
		if updateTitle == "" {
			return fmt.Errorf("title cannot be empty")
		}
		if len(updateTitle) > 200 {
			return fmt.Errorf("title too long (max 200 characters)")
		}
	}

	if cmd.Flags().Changed("weight") {
		if err := security.ValidateWeight(updateWeight); err != nil {
			return fmt.Errorf("invalid weight: %w", err)
		}
	}

	if cmd.Flags().Changed("add-tag") {
		for _, tag := range updateAddTag {
			if err := security.ValidateTag(tag); err != nil {
				return fmt.Errorf("invalid tag %q: %w", tag, err)
			}
		}
	}

	if cmd.Flags().Changed("body") {
		if len(updateBody) > 10000 {
			return fmt.Errorf("body too long (max 10000 characters)")
		}
	}

	if cmd.Flags().Changed("size") {
		if err := security.ValidateEnum(updateSize, core.ValidSizes, "size"); err != nil {
			return fmt.Errorf("invalid size: %w", err)
		}
	}

	if cmd.Flags().Changed("parent") {
		if updateParent != "" {
			if err := security.ValidateMoteID(updateParent); err != nil {
				return fmt.Errorf("invalid parent ID: %w", err)
			}
		}
	}

	// Validate execution metadata flags (STORY-EXEC-001). Empty values are
	// permitted: they signal "clear this field".
	executionFlagsByName := []struct {
		flag, field string
		value       string
	}{
		{"execution-agent-type", "execution_agent_type", updateExecutionAgentType},
		{"execution-suggested-model", "execution_suggested_model", updateExecutionSuggestedModel},
		{"execution-reasoning-effort", "execution_reasoning_effort", updateExecutionReasoningEffort},
		{"execution-mode", "execution_mode", updateExecutionMode},
		{"execution-parallel-group", "execution_parallel_group", updateExecutionParallelGroup},
	}
	for _, ef := range executionFlagsByName {
		if !cmd.Flags().Changed(ef.flag) {
			continue
		}
		if err := security.ValidateExecutionField(ef.field, ef.value); err != nil {
			return err
		}
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)

	var opts core.UpdateOpts
	var parts []string

	if cmd.Flags().Changed("status") {
		opts.Status = &updateStatus
		parts = append(parts, fmt.Sprintf("status=%s", updateStatus))
	}
	if cmd.Flags().Changed("title") {
		opts.Title = &updateTitle
		parts = append(parts, fmt.Sprintf("title=%s", updateTitle))
	}
	if cmd.Flags().Changed("weight") {
		opts.Weight = &updateWeight
		parts = append(parts, fmt.Sprintf("weight=%v", updateWeight))
	}
	if cmd.Flags().Changed("add-tag") {
		m, err := mm.Read(moteID)
		if err != nil {
			return fmt.Errorf("read mote: %w", err)
		}
		tags := m.Tags
		for _, t := range updateAddTag {
			tags = append(tags, t)
		}
		opts.Tags = tags
		parts = append(parts, fmt.Sprintf("tags=%v", tags))
	}
	if cmd.Flags().Changed("body") {
		opts.Body = &updateBody
		parts = append(parts, fmt.Sprintf("body=%s", updateBody))
	}
	if cmd.Flags().Changed("accept") {
		m, err := mm.Read(moteID)
		if err != nil {
			return fmt.Errorf("read mote: %w", err)
		}
		acceptance := m.Acceptance
		acceptanceMet := m.AcceptanceMet
		for _, a := range updateAccept {
			acceptance = append(acceptance, a)
			acceptanceMet = append(acceptanceMet, false)
		}
		opts.Acceptance = acceptance
		opts.AcceptanceMet = acceptanceMet
		parts = append(parts, fmt.Sprintf("acceptance=%v", acceptance))
	}
	if cmd.Flags().Changed("size") {
		opts.Size = &updateSize
		parts = append(parts, fmt.Sprintf("size=%s", updateSize))
	}
	if cmd.Flags().Changed("parent") {
		opts.Parent = &updateParent
		parts = append(parts, fmt.Sprintf("parent=%s", updateParent))
	}
	if cmd.Flags().Changed("execution-agent-type") {
		opts.ExecutionAgentType = &updateExecutionAgentType
		parts = append(parts, fmt.Sprintf("execution_agent_type=%s", updateExecutionAgentType))
	}
	if cmd.Flags().Changed("execution-suggested-model") {
		opts.ExecutionSuggestedModel = &updateExecutionSuggestedModel
		parts = append(parts, fmt.Sprintf("execution_suggested_model=%s", updateExecutionSuggestedModel))
	}
	if cmd.Flags().Changed("execution-reasoning-effort") {
		opts.ExecutionReasoningEffort = &updateExecutionReasoningEffort
		parts = append(parts, fmt.Sprintf("execution_reasoning_effort=%s", updateExecutionReasoningEffort))
	}
	if cmd.Flags().Changed("execution-mode") {
		opts.ExecutionMode = &updateExecutionMode
		parts = append(parts, fmt.Sprintf("execution_mode=%s", updateExecutionMode))
	}
	if cmd.Flags().Changed("execution-parallel-group") {
		opts.ExecutionParallelGroup = &updateExecutionParallelGroup
		parts = append(parts, fmt.Sprintf("execution_parallel_group=%s", updateExecutionParallelGroup))
	}

	opts.Force = updateForce
	opts.Quiet = updateQuiet

	if err := mm.Update(moteID, opts); err != nil {
		return fmt.Errorf("update mote: %w", err)
	}

	// Print confirmation
	fmt.Fprintf(os.Stdout, "Updated %s:", moteID)
	for _, p := range parts {
		fmt.Fprintf(os.Stdout, " %s", p)
	}
	fmt.Fprintln(os.Stdout)

	// Post-completion feedback (R2, R5, R6)
	if cmd.Flags().Changed("status") && updateStatus == "completed" {
		completedMote, readErr := mm.Read(moteID)
		if readErr == nil {
			// R2: print tasks unblocked by this completion
			readyTasks, _ := mm.List(core.ListFilters{Ready: true, Type: "task"})
			var unblocked []*core.Mote
			for _, t := range readyTasks {
				for _, dep := range t.DependsOn {
					if dep == moteID {
						unblocked = append(unblocked, t)
						break
					}
				}
			}
			if len(unblocked) > 0 {
				fmt.Fprintf(os.Stdout, "  Unblocked (%d):", len(unblocked))
				for _, t := range unblocked {
					fmt.Fprintf(os.Stdout, " %s", t.Title)
				}
				fmt.Fprintln(os.Stdout)
			}

			// R5: tag-overlap link suggestions
			if len(completedMote.Tags) > 0 {
				liveTasks, _ := mm.List(core.ListFilters{Type: "task"})
				var suggestions []*core.Mote
				for _, t := range liveTasks {
					if t.ID == moteID || !core.IsLive(t.Status) {
						continue
					}
					if tagOverlapCount(completedMote.Tags, t.Tags) > 0 {
						suggestions = append(suggestions, t)
						if len(suggestions) >= 3 {
							break
						}
					}
				}
				if len(suggestions) > 0 {
					fmt.Fprintln(os.Stdout, "  Related active tasks (tag overlap):")
					for _, t := range suggestions {
						fmt.Fprintf(os.Stdout, "    → %s — %s\n", t.ID, t.Title)
					}
				}
			}

			// R6: epic wrap-up prompt when completing a task with children
			children, _ := mm.List(core.ListFilters{Parent: moteID, Type: "task"})
			if len(children) > 0 {
				doneCount := 0
				for _, c := range children {
					if c.Status == "completed" || c.Status == "archived" {
						doneCount++
					}
				}
				fmt.Fprintf(os.Stdout, "  Epic complete: %d/%d children done\n", doneCount, len(children))
				fmt.Fprintf(os.Stdout, "  Tip: mote crystallize %s --type=decision\n", moteID)
			}
		}
	}

	return nil
}

// ClaimOutput is the JSON shape printed by `mote update <id> --claim --json`.
// On success Claimed=true and Status/ClaimedBy are populated. On lost-race
// contention Claimed=false and CurrentStatus/CurrentClaimedBy carry the
// existing claim metadata so callers can surface it without a re-read.
type ClaimOutput struct {
	ID               string `json:"id"`
	Claimed          bool   `json:"claimed"`
	Status           string `json:"status,omitempty"`
	ClaimedBy        string `json:"claimed_by,omitempty"`
	CurrentStatus    string `json:"current_status,omitempty"`
	CurrentClaimedBy string `json:"current_claimed_by,omitempty"`
}

// runUpdateClaim handles `mote update <id> --claim [--json]`. MOTE_AGENT_ID
// must be set and validated — the claim identity has audit consequences, so
// we deliberately do NOT use core.ResolveAgentID (which falls back to
// hostname-PID).
func runUpdateClaim(moteID string) error {
	agentID := os.Getenv("MOTE_AGENT_ID")
	if agentID == "" {
		return &exitCodeError{code: 1, err: fmt.Errorf("MOTE_AGENT_ID is required for --claim")}
	}
	if err := security.ValidateAgentID(agentID); err != nil {
		return &exitCodeError{code: 1, err: fmt.Errorf("invalid MOTE_AGENT_ID: %w", err)}
	}
	if err := security.ValidateMoteID(moteID); err != nil {
		return &exitCodeError{code: 1, err: fmt.Errorf("invalid mote ID: %w", err)}
	}

	root, err := findMemoryRoot()
	if err != nil {
		return &exitCodeError{code: 1, err: err}
	}
	mm := core.NewMoteManager(root)

	result, claimErr := mm.Claim(moteID, agentID)

	if updateJSON && result != nil {
		out := ClaimOutput{
			ID:               moteID,
			Claimed:          result.Claimed,
			ClaimedBy:        result.ClaimedBy,
			CurrentStatus:    result.CurrentStatus,
			CurrentClaimedBy: result.CurrentClaimedBy,
		}
		if result.Claimed {
			out.Status = "in_progress"
		}
		data, mErr := json.MarshalIndent(out, "", "  ")
		if mErr != nil {
			return &exitCodeError{code: 1, err: fmt.Errorf("marshal json: %w", mErr)}
		}
		fmt.Println(string(data))
	}

	if claimErr != nil {
		if errors.Is(claimErr, core.ErrAlreadyClaimed) {
			return &exitCodeError{code: 2, err: claimErr}
		}
		return &exitCodeError{code: 1, err: claimErr}
	}

	if !updateJSON {
		_, _ = fmt.Fprintf(os.Stdout, "Claimed %s as %s\n", moteID, agentID)
	}
	return nil
}

// tagOverlapCount returns the number of tags in a that also appear in b (case-insensitive).
func tagOverlapCount(a, b []string) int {
	count := 0
	for _, ta := range a {
		for _, tb := range b {
			if strings.EqualFold(ta, tb) {
				count++
				break
			}
		}
	}
	return count
}
