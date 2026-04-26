// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyGlobal_MovesMotesOwnedFiles(t *testing.T) {
	legacy, newRoot := setupMigrationDirs(t)

	// Seed motes-owned content in legacy root.
	mustWriteFile(t, filepath.Join(legacy, "nodes", "global-Aabc.md"), "mote body")
	mustWriteFile(t, filepath.Join(legacy, "index.jsonl"), `{"id":"global-Aabc"}`+"\n")
	mustWriteFile(t, filepath.Join(legacy, "config.yaml"), "scoring: {}\n")
	mustMkdir(t, filepath.Join(legacy, "dream"))
	mustWriteFile(t, filepath.Join(legacy, "dream", "log.jsonl"), "")

	// Seed auto-memory content that must NOT migrate.
	mustWriteFile(t, filepath.Join(legacy, "MEMORY.md"), "# auto-memory")
	mustWriteFile(t, filepath.Join(legacy, "project_x.md"), "claude project note")

	moved, err := MigrateLegacyGlobal(legacy, newRoot)
	if err != nil {
		t.Fatalf("MigrateLegacyGlobal: %v", err)
	}
	if !moved {
		t.Fatal("expected moved=true")
	}

	// motes-owned files should be at newRoot.
	mustExist(t, filepath.Join(newRoot, "nodes", "global-Aabc.md"))
	mustExist(t, filepath.Join(newRoot, "index.jsonl"))
	mustExist(t, filepath.Join(newRoot, "config.yaml"))
	mustExist(t, filepath.Join(newRoot, "dream", "log.jsonl"))

	// Auto-memory files should remain in legacy.
	mustExist(t, filepath.Join(legacy, "MEMORY.md"))
	mustExist(t, filepath.Join(legacy, "project_x.md"))

	// motes-owned files should be gone from legacy.
	mustNotExist(t, filepath.Join(legacy, "nodes"))
	mustNotExist(t, filepath.Join(legacy, "index.jsonl"))
	mustNotExist(t, filepath.Join(legacy, "config.yaml"))

	// Marker file should exist.
	mustExist(t, filepath.Join(newRoot, migrationMarkerName))
}

func TestMigrateLegacyGlobal_IsIdempotent(t *testing.T) {
	legacy, newRoot := setupMigrationDirs(t)
	mustWriteFile(t, filepath.Join(legacy, "nodes", "global-Aabc.md"), "x")

	if _, err := MigrateLegacyGlobal(legacy, newRoot); err != nil {
		t.Fatalf("first run: %v", err)
	}
	moved, err := MigrateLegacyGlobal(legacy, newRoot)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if moved {
		t.Fatal("expected moved=false on idempotent re-run")
	}
}

func TestMigrateLegacyGlobal_SkipsExistingDestinationFiles(t *testing.T) {
	legacy, newRoot := setupMigrationDirs(t)
	mustWriteFile(t, filepath.Join(legacy, "config.yaml"), "from legacy")
	mustWriteFile(t, filepath.Join(newRoot, "config.yaml"), "from new")

	if _, err := MigrateLegacyGlobal(legacy, newRoot); err != nil {
		t.Fatalf("MigrateLegacyGlobal: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(newRoot, "config.yaml"))
	if string(got) != "from new" {
		t.Fatalf("destination clobbered; got %q", string(got))
	}
	// Source still present (we didn't move it because dest existed).
	mustExist(t, filepath.Join(legacy, "config.yaml"))
}

func TestMigrateLegacyGlobal_NoLegacyContent(t *testing.T) {
	legacy, newRoot := setupMigrationDirs(t)
	// Legacy dir exists but has no motes-owned files.
	mustWriteFile(t, filepath.Join(legacy, "MEMORY.md"), "auto-memory only")

	moved, err := MigrateLegacyGlobal(legacy, newRoot)
	if err != nil {
		t.Fatalf("MigrateLegacyGlobal: %v", err)
	}
	if moved {
		t.Fatal("expected moved=false when nothing to move")
	}
	// Marker still written (signals "we checked").
	mustExist(t, filepath.Join(newRoot, migrationMarkerName))
	// MEMORY.md untouched.
	mustExist(t, filepath.Join(legacy, "MEMORY.md"))
}

func TestGlobalRoot_PrefersNewPathWhenPopulated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MOTE_GLOBAL_ROOT", "")

	// Populate ~/.motes/nodes/
	mustWriteFile(t, filepath.Join(home, ".motes", "nodes", "global-X.md"), "x")

	got, err := GlobalRoot()
	if err != nil {
		t.Fatalf("GlobalRoot: %v", err)
	}
	want := filepath.Join(home, ".motes")
	if got != want {
		t.Fatalf("GlobalRoot = %q, want %q", got, want)
	}
}

func TestGlobalRoot_TriggersMigrationFromLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MOTE_GLOBAL_ROOT", "")

	legacy := filepath.Join(home, ".claude", "memory")
	mustWriteFile(t, filepath.Join(legacy, "nodes", "global-X.md"), "legacy mote")
	mustWriteFile(t, filepath.Join(legacy, "MEMORY.md"), "auto-memory")

	got, err := GlobalRoot()
	if err != nil {
		t.Fatalf("GlobalRoot: %v", err)
	}
	want := filepath.Join(home, ".motes")
	if got != want {
		t.Fatalf("GlobalRoot = %q, want %q", got, want)
	}
	mustExist(t, filepath.Join(want, "nodes", "global-X.md"))
	mustExist(t, filepath.Join(legacy, "MEMORY.md")) // auto-memory untouched
}

func TestGlobalRoot_FreshInstallUsesNewPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MOTE_GLOBAL_ROOT", "")

	got, err := GlobalRoot()
	if err != nil {
		t.Fatalf("GlobalRoot: %v", err)
	}
	want := filepath.Join(home, ".motes")
	if got != want {
		t.Fatalf("GlobalRoot = %q, want %q", got, want)
	}
}

func TestGlobalRoot_EnvOverrideWins(t *testing.T) {
	home := t.TempDir()
	override := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MOTE_GLOBAL_ROOT", override)

	// Even with content at both legacy and ~/.motes, env override wins.
	mustWriteFile(t, filepath.Join(home, ".motes", "nodes", "x.md"), "x")
	mustWriteFile(t, filepath.Join(home, ".claude", "memory", "nodes", "y.md"), "y")

	got, err := GlobalRoot()
	if err != nil {
		t.Fatalf("GlobalRoot: %v", err)
	}
	if got != override {
		t.Fatalf("GlobalRoot = %q, want override %q", got, override)
	}
}

func TestHasMotesContent(t *testing.T) {
	dir := t.TempDir()
	if hasMotesContent(dir) {
		t.Fatal("empty dir reported content")
	}
	mustWriteFile(t, filepath.Join(dir, "nodes", "x.md"), "x")
	if !hasMotesContent(dir) {
		t.Fatal("populated nodes/ not detected")
	}

	dir2 := t.TempDir()
	mustWriteFile(t, filepath.Join(dir2, "index.jsonl"), "{}")
	if !hasMotesContent(dir2) {
		t.Fatal("index.jsonl not detected")
	}

	dir3 := t.TempDir()
	mustWriteFile(t, filepath.Join(dir3, "MEMORY.md"), "auto-memory")
	if hasMotesContent(dir3) {
		t.Fatal("MEMORY.md should not count as motes content")
	}
}

// --- helpers ---

func setupMigrationDirs(t *testing.T) (legacy, newRoot string) {
	t.Helper()
	tmp := t.TempDir()
	legacy = filepath.Join(tmp, "legacy")
	newRoot = filepath.Join(tmp, "new")
	mustMkdir(t, legacy)
	return legacy, newRoot
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected to exist: %s (%v)", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected NOT to exist: %s", path)
	}
}
