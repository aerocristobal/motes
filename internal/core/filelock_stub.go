//go:build windows

package core

import "fmt"

// FileLock is a stub on Windows where flock(2) is not available.
type FileLock struct {
	path string
}

func NewFileLock(path string) *FileLock { return &FileLock{path: path} }

func (fl *FileLock) Lock() error             { return fmt.Errorf("file locking not supported on Windows") }
func (fl *FileLock) TryLock() (bool, error)   { return false, fmt.Errorf("file locking not supported on Windows") }
func (fl *FileLock) Unlock() error            { return nil }
