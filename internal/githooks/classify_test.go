// SPDX-License-Identifier: MIT

package githooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyHookState_Table(t *testing.T) {
	embedded := TemplatePostCheckout
	rendered, err := renderTemplate(embedded)
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}

	type tc struct {
		name     string
		setup    func(t *testing.T, path string)
		wantCode ActionCode
	}
	tests := []tc{
		{
			name:     "absent",
			setup:    func(t *testing.T, path string) {},
			wantCode: ActionInstall,
		},
		{
			name: "managed-match",
			setup: func(t *testing.T, path string) {
				if err := os.WriteFile(path, rendered, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: ActionUnchanged,
		},
		{
			name: "managed-drift",
			setup: func(t *testing.T, path string) {
				drifted := append([]byte{}, rendered...)
				drifted = append(drifted, []byte("# tampered\n")...)
				if err := os.WriteFile(path, drifted, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: ActionUpdate,
		},
		{
			name: "user-authored",
			setup: func(t *testing.T, path string) {
				body := []byte("#!/bin/sh\necho hi\n")
				if err := os.WriteFile(path, body, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: ActionConflict,
		},
		{
			name: "symlink-outside",
			setup: func(t *testing.T, path string) {
				target := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlink not supported: %v", err)
				}
			},
			wantCode: ActionConflict,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "post-checkout")
			c.setup(t, path)
			got, err := ClassifyHookState(path, embedded)
			if err != nil {
				t.Fatalf("ClassifyHookState: %v", err)
			}
			if got != c.wantCode {
				t.Errorf("got action %q, want %q", got, c.wantCode)
			}
		})
	}
}

func TestStripSentinel_RoundTrip(t *testing.T) {
	embedded := TemplatePreCommit
	rendered, err := renderTemplate(embedded)
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	stripped := stripSentinel(rendered)
	if string(stripped) != string(embedded) {
		t.Errorf("stripSentinel did not round-trip embedded body\nwant:\n%s\ngot:\n%s",
			embedded, stripped)
	}
}

func TestHasMoteSentinel(t *testing.T) {
	rendered, err := renderTemplate(TemplatePostCheckout)
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !hasMoteSentinel(rendered) {
		t.Errorf("rendered template should carry sentinel")
	}
	if hasMoteSentinel([]byte("#!/bin/sh\necho hi\n")) {
		t.Errorf("plain script should not be detected as mote-managed")
	}
}
