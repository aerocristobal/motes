// SPDX-License-Identifier: MIT

package githooks

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/version"
)

// mkRepo creates a temp dir with a bare `.git/hooks/` layout, mimicking what
// `git init` would produce, without shelling out.
func mkRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInstall_WritesAllTemplates_OnFreshHooksDir(t *testing.T) {
	root := mkRepo(t)

	report, err := Install(root, InstallOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(report.Events) != len(Templates()) {
		t.Fatalf("report should describe every template; got %d, want %d",
			len(report.Events), len(Templates()))
	}
	for _, ev := range report.Events {
		if ev.Action != ActionInstall {
			t.Errorf("event %s: action %q, want %q", ev.Event, ev.Action, ActionInstall)
		}
		info, err := os.Stat(ev.Path)
		if err != nil {
			t.Fatalf("event %s: stat hook: %v", ev.Event, err)
		}
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Errorf("event %s: mode %#o, want 0755", ev.Event, perm)
		}
		body, err := os.ReadFile(ev.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("# managed-by: mote githooks install")) {
			t.Errorf("event %s: missing managed-by sentinel", ev.Event)
		}
		if !bytes.Contains(body, []byte("# mote-binary-version: "+version.Value)) {
			t.Errorf("event %s: missing version sentinel", ev.Event)
		}
		if !bytes.Contains(body, []byte("# template-sha256: ")) {
			t.Errorf("event %s: missing sha256 sentinel", ev.Event)
		}
	}
}

func TestInstall_Idempotent_Unchanged(t *testing.T) {
	root := mkRepo(t)
	if _, err := Install(root, InstallOpts{}); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	report, err := Install(root, InstallOpts{})
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	for _, ev := range report.Events {
		if ev.Action != ActionUnchanged {
			t.Errorf("event %s: want unchanged on re-run, got %q", ev.Event, ev.Action)
		}
	}
}

func TestInstall_RewritesDrifted_MoteManagedHook(t *testing.T) {
	root := mkRepo(t)
	if _, err := Install(root, InstallOpts{}); err != nil {
		t.Fatalf("seed Install: %v", err)
	}
	// Snapshot every non-drifted hook so we can prove they were untouched
	// (STORY-HOOKINST-001 Scenario 3: "no other files under .git/hooks/
	// are touched").
	snapshots := map[string][]byte{}
	hooksDir := filepath.Join(root, ".git", "hooks")
	for _, tpl := range Templates() {
		if tpl.Event == "post-checkout" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(hooksDir, tpl.Event))
		if err != nil {
			t.Fatal(err)
		}
		snapshots[tpl.Event] = b
	}

	// Append a stray line so sha256(stripped) differs from sha256(embedded).
	path := filepath.Join(hooksDir, "post-checkout")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# tampered\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := Install(root, InstallOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	var found bool
	for _, ev := range report.Events {
		if ev.Event != "post-checkout" {
			continue
		}
		found = true
		if ev.Action != ActionUpdate {
			t.Errorf("post-checkout: want update, got %q", ev.Action)
		}
	}
	if !found {
		t.Fatal("post-checkout missing from report")
	}
	// Body should now match the freshly rendered template.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("# tampered")) {
		t.Errorf("drifted line was not removed")
	}
	// No other hook file should have been rewritten.
	for event, before := range snapshots {
		after, err := os.ReadFile(filepath.Join(hooksDir, event))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("hook %s was rewritten despite being unchanged", event)
		}
	}
}

func TestInstall_DryRun_DoesNotWrite(t *testing.T) {
	root := mkRepo(t)
	report, err := Install(root, InstallOpts{DryRun: true})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(report.Events) != len(Templates()) {
		t.Fatalf("dry-run should still classify every event")
	}
	for _, ev := range report.Events {
		if ev.Action != ActionInstall {
			t.Errorf("event %s: want install action, got %q", ev.Event, ev.Action)
		}
		if _, err := os.Stat(ev.Path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("event %s: dry-run wrote %s", ev.Event, ev.Path)
		}
	}
}

func TestInstall_ReturnsErrConflict_OnUserAuthoredHook(t *testing.T) {
	root := mkRepo(t)
	hookPath := filepath.Join(root, ".git", "hooks", "post-checkout")
	userBody := []byte("#!/bin/sh\n# user-authored\nexit 0\n")
	if err := os.WriteFile(hookPath, userBody, 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := Install(root, InstallOpts{})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	// User file untouched.
	gotBody, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBody, userBody) {
		t.Errorf("user hook was modified despite conflict")
	}
	// Other hooks should still install normally.
	var sawConflict, sawInstall bool
	for _, ev := range report.Events {
		if ev.Event == "post-checkout" && ev.Action == ActionConflict {
			sawConflict = true
		}
		if ev.Event != "post-checkout" && ev.Action == ActionInstall {
			sawInstall = true
		}
	}
	if !sawConflict {
		t.Errorf("report missing conflict for post-checkout")
	}
	if !sawInstall {
		t.Errorf("non-conflicting hooks should still install")
	}
}

func TestInstall_DryRun_SuppressesErrConflict(t *testing.T) {
	root := mkRepo(t)
	hookPath := filepath.Join(root, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := Install(root, InstallOpts{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run should not surface ErrConflict, got %v", err)
	}
	var sawConflict bool
	for _, ev := range report.Events {
		if ev.Event == "post-checkout" && ev.Action == ActionConflict {
			sawConflict = true
		}
	}
	if !sawConflict {
		t.Errorf("report should still flag conflict in dry-run")
	}
}

func TestInstall_Force_OverwritesUserAuthored(t *testing.T) {
	root := mkRepo(t)
	hookPath := filepath.Join(root, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n# user\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := Install(root, InstallOpts{Force: true})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, ev := range report.Events {
		if ev.Event == "post-checkout" && ev.Action != ActionUpdate {
			t.Errorf("post-checkout: want update under --force, got %q", ev.Action)
		}
	}
	body, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("# managed-by: mote githooks install")) {
		t.Errorf("--force did not install the embedded template")
	}
}

func TestInstall_ReturnsErrNotGitRepo_WhenNoDotGit(t *testing.T) {
	root := t.TempDir()
	_, err := Install(root, InstallOpts{})
	if !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("want ErrNotGitRepo, got %v", err)
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("error message should name the directory checked; got %q", err.Error())
	}
}

func TestInstall_WalksUp_FromSubdir(t *testing.T) {
	root := mkRepo(t)
	sub := filepath.Join(root, "deep", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(sub, InstallOpts{}); err != nil {
		t.Fatalf("Install from subdir: %v", err)
	}
	// Hooks land under root/.git/hooks/, not sub/.git/hooks/.
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "post-checkout")); err != nil {
		t.Errorf("hook not found at repo root: %v", err)
	}
}

func TestRenderTemplate_RejectsMissingShebang(t *testing.T) {
	_, err := renderTemplate([]byte("echo no shebang\n"))
	if !errors.Is(err, ErrTemplateLoad) {
		t.Errorf("want ErrTemplateLoad, got %v", err)
	}
}
