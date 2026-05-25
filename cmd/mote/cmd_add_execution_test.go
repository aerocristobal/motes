// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"motes/internal/core"
)

func resetAddFlags() {
	addType, addTitle, addBody, addStatus, addOrigin, addSize, addParent = "", "", "", "", "normal", "", ""
	addWeight = 0.5
	addTags, addAccept, addRefs = nil, nil, nil
	addLocal, addForce, addQuiet = false, false, false
	addExecutionAgentType = ""
	addExecutionSuggestedModel = ""
	addExecutionReasoningEffort = ""
	addExecutionMode = ""
	addExecutionParallelGroup = ""
	addDue = ""
	addDefer = ""
	addCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
}

func runAddViaCobra(args []string) error {
	resetAddFlags()
	defer resetAddFlags()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

func TestAdd_AllFiveExecutionFlags(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	if err := runAddViaCobra([]string{
		"add",
		"--type=task",
		"--title=parallel job 1",
		"--body=hello",
		"--execution-agent-type=mote-subagent",
		"--execution-suggested-model=haiku",
		"--execution-reasoning-effort=low",
		"--execution-mode=parallel",
		"--execution-parallel-group=group-A",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	mm := core.NewMoteManager(memDir)
	motes, err := mm.List(core.ListFilters{Type: "task"})
	if err != nil || len(motes) != 1 {
		t.Fatalf("expected 1 task, got %d (err=%v)", len(motes), err)
	}
	m := motes[0]
	if m.ExecutionAgentType != "mote-subagent" {
		t.Errorf("agent_type: %q", m.ExecutionAgentType)
	}
	if m.ExecutionSuggestedModel != "haiku" {
		t.Errorf("suggested_model: %q", m.ExecutionSuggestedModel)
	}
	if m.ExecutionReasoningEffort != "low" {
		t.Errorf("reasoning_effort: %q", m.ExecutionReasoningEffort)
	}
	if m.ExecutionMode != "parallel" {
		t.Errorf("mode: %q", m.ExecutionMode)
	}
	if m.ExecutionParallelGroup != "group-A" {
		t.Errorf("parallel_group: %q", m.ExecutionParallelGroup)
	}
}

func TestAdd_NoExecutionFlags_OmitsFromFrontmatter(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	if err := runAddViaCobra([]string{"add", "--type=task", "--title=ordinary", "--body=plain"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	mm := core.NewMoteManager(memDir)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	m := motes[0]
	if m.ExecutionMode != "" || m.ExecutionAgentType != "" {
		t.Errorf("unset execution fields should be empty, got mode=%q agent_type=%q",
			m.ExecutionMode, m.ExecutionAgentType)
	}
}

func TestAdd_InvalidExecutionMode_Rejected(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	err := runAddViaCobra([]string{"add", "--type=task", "--title=t", "--body=b", "--execution-mode=fire_and_forget"})
	if err == nil {
		t.Fatal("expected error for invalid execution_mode")
	}
	if !strings.Contains(err.Error(), "invalid execution_mode") {
		t.Errorf("error should mention 'invalid execution_mode': %v", err)
	}
	mm := core.NewMoteManager(memDir)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	if len(motes) != 0 {
		t.Errorf("no mote should be created on validation failure, found %d", len(motes))
	}
}

func TestAdd_AdversarialAgentType_Rejected(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	err := runAddViaCobra([]string{"add", "--type=task", "--title=t", "--body=b", "--execution-agent-type=$(rm -rf ~)"})
	if err == nil {
		t.Fatal("expected error for adversarial agent_type")
	}
	mm := core.NewMoteManager(memDir)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	if len(motes) != 0 {
		t.Errorf("no mote should be created on validation failure, found %d", len(motes))
	}
}

func TestAdd_PartialExecutionFlags(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	if err := runAddViaCobra([]string{"add", "--type=task", "--title=p", "--body=b", "--execution-mode=delegated"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	mm := core.NewMoteManager(memDir)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	m := motes[0]
	if m.ExecutionMode != "delegated" {
		t.Errorf("mode: %q", m.ExecutionMode)
	}
	if m.ExecutionAgentType != "" || m.ExecutionParallelGroup != "" {
		t.Errorf("other execution fields should be empty")
	}
}
