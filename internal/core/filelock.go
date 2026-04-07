// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build !windows

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FileLock provides file-based advisory locking using flock(2).
// Lock ordering: ops > batch > mote. Never acquire ops while holding mote.
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a FileLock for the given path.
// The lock file is created if it doesn't exist.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

// Lock acquires an exclusive lock, blocking until available.
func (fl *FileLock) Lock() error {
	if err := os.MkdirAll(filepath.Dir(fl.path), 0755); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}
	f, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return fmt.Errorf("flock: %w", err)
	}
	fl.file = f
	return nil
}

// TryLock attempts to acquire an exclusive lock without blocking.
// Returns true if the lock was acquired, false if held by another process.
func (fl *FileLock) TryLock() (bool, error) {
	if err := os.MkdirAll(filepath.Dir(fl.path), 0755); err != nil {
		return false, fmt.Errorf("create lock dir: %w", err)
	}
	f, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return false, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false, nil // Lock held by another process/fd
	}
	fl.file = f
	return true, nil
}

// Unlock releases the lock and closes the file descriptor.
// Safe to call multiple times.
func (fl *FileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}
	err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN)
	fl.file.Close()
	fl.file = nil
	return err
}
