package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// findMemoryRoot walks cwd upward looking for a .memory/ directory.
func findMemoryRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, ".memory")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .memory/ directory found (run 'mote init' to initialize)")
}

// mustFindRoot returns the .memory/ path or exits with an error message.
func mustFindRoot() string {
	root, err := findMemoryRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	return root
}

// initMemoryDir creates .memory/ and .memory/nodes/ if absent.
func initMemoryDir(root string) error {
	return os.MkdirAll(filepath.Join(root, "nodes"), 0755)
}

// openEditor opens the given file in $EDITOR (or vi as fallback).
func openEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
