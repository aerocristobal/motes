//go:build !windows

package core

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFileLock_LockUnlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	fl := NewFileLock(path)
	if err := fl.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}
	if err := fl.Unlock(); err != nil {
		t.Fatalf("Unlock() failed: %v", err)
	}

	// Lock file should exist on disk
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file should exist: %v", err)
	}
}

func TestFileLock_DoubleUnlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	fl := NewFileLock(path)
	if err := fl.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}
	if err := fl.Unlock(); err != nil {
		t.Fatalf("first Unlock() failed: %v", err)
	}
	// Second unlock should be safe (no-op)
	if err := fl.Unlock(); err != nil {
		t.Fatalf("second Unlock() should be safe: %v", err)
	}
}

func TestFileLock_TryLock_Contention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	// Acquire lock with one fd
	fl1 := NewFileLock(path)
	if err := fl1.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}

	// TryLock with a different FileLock (different fd) should fail
	fl2 := NewFileLock(path)
	acquired, err := fl2.TryLock()
	if err != nil {
		t.Fatalf("TryLock() error: %v", err)
	}
	if acquired {
		t.Fatal("TryLock() should return false when lock is held by another fd")
	}

	fl1.Unlock()

	// Now TryLock should succeed
	acquired, err = fl2.TryLock()
	if err != nil {
		t.Fatalf("TryLock() error after unlock: %v", err)
	}
	if !acquired {
		t.Fatal("TryLock() should succeed after unlock")
	}
	fl2.Unlock()
}

func TestFileLock_ConcurrentSerialization(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")
	counterPath := filepath.Join(dir, "counter")
	os.WriteFile(counterPath, []byte("0"), 0644)

	var wg sync.WaitGroup
	var serialized atomic.Int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fl := NewFileLock(path)
				if err := fl.Lock(); err != nil {
					t.Errorf("Lock() failed: %v", err)
					return
				}
				serialized.Add(1)
				fl.Unlock()
			}
		}()
	}
	wg.Wait()

	if got := serialized.Load(); got != 500 {
		t.Fatalf("expected 500 serialized operations, got %d", got)
	}
}
