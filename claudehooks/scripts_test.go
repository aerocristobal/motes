// SPDX-License-Identifier: MIT

package claudehooks

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// invokeHook runs the named script in this package directory with a Bash tool_input payload.
// Returns the script's stdout. Tests should pass {"tool_input":{"command":"..."}}.
func invokeHook(t *testing.T, script, command string) string {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed; safety hooks require jq")
	}
	payload, err := json.Marshal(map[string]any{
		"tool_input": map[string]any{"command": command},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	cmd := exec.Command("bash", script)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("script %s exited %v\nstderr: %s\nstdout: %s", script, err, stderr.String(), stdout.String())
	}
	return stdout.String()
}

func assertDeny(t *testing.T, out, label string, reasonContains ...string) {
	t.Helper()
	if out == "" {
		t.Fatalf("[%s] expected deny envelope, got empty stdout (allow)", label)
	}
	var env struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("[%s] stdout is not valid JSON: %v\nstdout: %s", label, err, out)
	}
	if env.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("[%s] expected permissionDecision=deny, got %q", label, env.HookSpecificOutput.PermissionDecision)
	}
	for _, sub := range reasonContains {
		if !strings.Contains(env.HookSpecificOutput.PermissionDecisionReason, sub) {
			t.Errorf("[%s] reason missing %q\nreason: %s", label, sub, env.HookSpecificOutput.PermissionDecisionReason)
		}
	}
}

func assertAllow(t *testing.T, out, label string) {
	t.Helper()
	if out != "" {
		t.Fatalf("[%s] expected allow (empty stdout), got: %s", label, out)
	}
}

// --- block-interactive-cmds.sh ------------------------------------------------

func TestBlockInteractiveCmds_Denies(t *testing.T) {
	cases := []string{
		"rm somefile.txt",
		"rm somefile.txt other.txt",
		"cp a.txt b.txt",
		"mv a.txt b.txt",
		"rm -i foo",            // -i but no -f
		"rm --interactive foo", // long form, still no -f / --force
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			out := invokeHook(t, "block-interactive-cmds.sh", cmd)
			assertDeny(t, out, cmd)
		})
	}
}

func TestBlockInteractiveCmds_Allows(t *testing.T) {
	cases := []string{
		"rm -f somefile.txt",
		"rm -rf foo",
		"rm -fr foo",
		"rm --force foo",
		"cp -f a.txt b.txt",
		"mv -f a.txt b.txt",
		"/bin/rm somefile.txt",        // absolute path bypass
		"/usr/bin/cp a b",             // absolute path bypass
		"command rm somefile.txt",     // command-builtin bypass
		"go test ./...",               // unrelated
		"git status",                  // unrelated
		"mote ls",                     // unrelated
		"ls .memory/nodes",            // unrelated
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			out := invokeHook(t, "block-interactive-cmds.sh", cmd)
			assertAllow(t, out, cmd)
		})
	}
}

func TestBlockInteractiveCmds_ReasonMentionsAllBypasses(t *testing.T) {
	out := invokeHook(t, "block-interactive-cmds.sh", "rm somefile.txt")
	// Reason must teach all three escape hatches: -f, absolute path, `command`.
	assertDeny(t, out, "rm somefile.txt", "-f", "/bin/", "command ")
}

// --- block-gh-watch.sh --------------------------------------------------------

func TestBlockGhWatch_Denies(t *testing.T) {
	cases := []string{
		"gh run watch",
		"gh run watch 1234567890",
		"gh run watch --interval=10",
		"gh run watch 1234 --exit-status",
		"  gh run watch", // leading whitespace
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			out := invokeHook(t, "block-gh-watch.sh", cmd)
			assertDeny(t, out, cmd, "quota")
		})
	}
}

func TestBlockGhWatch_Allows(t *testing.T) {
	cases := []string{
		"gh run view --log 12345",
		"gh run list",
		"gh run list --limit=5",
		"gh pr create",
		"gh pr view",
		"gh issue list",
		"go test ./...",
		"git status",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			out := invokeHook(t, "block-gh-watch.sh", cmd)
			assertAllow(t, out, cmd)
		})
	}
}

func TestBlockGhWatch_ReasonSuggestsAlternatives(t *testing.T) {
	out := invokeHook(t, "block-gh-watch.sh", "gh run watch")
	// Reason should redirect to gh run view / gh run list.
	assertDeny(t, out, "gh run watch", "gh run view", "gh run list")
}

// --- block-mote-rm.sh ---------------------------------------------------------

func TestBlockMoteRm_DeniesNodeDeletes(t *testing.T) {
	cases := []struct {
		name, cmd string
	}{
		{"rm-f-single-node", "rm -f .memory/nodes/foo.md"},
		{"rm-rf-nodes-dir", "rm -rf .memory/nodes/"},
		{"rm-bare-node", "rm .memory/nodes/foo.md"},
		{"trash-node", "trash .memory/nodes/foo.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := invokeHook(t, "block-mote-rm.sh", tc.cmd)
			assertDeny(t, out, tc.cmd, "mote delete")
		})
	}
}

func TestBlockMoteRm_DeniesDerivedIndexWrites(t *testing.T) {
	cases := []struct {
		name, cmd, mustMention string
	}{
		{"redirect-index", "echo {} > .memory/index.jsonl", "mote index rebuild"},
		{"tee-index", "tee .memory/index.jsonl", "mote index rebuild"},
		{"tee-append-index", "tee -a .memory/index.jsonl", "mote index rebuild"},
		{"redirect-bm25", "echo x > .memory/mote_bm25.json", "mote index rebuild"},
		{"redirect-strata-bm25", "echo y > .memory/strata/refs/bm25.json", "strata"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := invokeHook(t, "block-mote-rm.sh", tc.cmd)
			assertDeny(t, out, tc.cmd, tc.mustMention)
		})
	}
}

func TestBlockMoteRm_AllowsLegitimateOps(t *testing.T) {
	cases := []string{
		"mote delete proj-t1abc",
		"mote index rebuild",
		"mote trash restore proj-t1abc",
		"rm -f /tmp/scratch.txt",     // outside .memory
		"rm -rf node_modules",        // outside .memory
		"ls .memory/nodes/",          // read-only
		"cat .memory/index.jsonl",    // read-only
		"go test ./...",
		"git status",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			out := invokeHook(t, "block-mote-rm.sh", cmd)
			assertAllow(t, out, cmd)
		})
	}
}

// --- embedded asset sanity ----------------------------------------------------

func TestEmbeddedAssetsNonEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"BlockInteractiveCmds", BlockInteractiveCmds},
		{"BlockGhWatch", BlockGhWatch},
		{"BlockMoteRm", BlockMoteRm},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.data) == 0 {
				t.Fatal("embedded script is empty — //go:embed failed")
			}
			if !bytes.HasPrefix(tc.data, []byte("#!/usr/bin/env bash")) {
				t.Errorf("missing bash shebang in first line: %s", firstLine(tc.data))
			}
		})
	}
}

func firstLine(b []byte) string {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
