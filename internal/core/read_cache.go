// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"sync"
	"time"
)

// ReadCache is a process-lifetime in-memory cache for parsed motes.
// It uses file mtime for invalidation.
type ReadCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	mote  *Mote
	mtime time.Time
}

// NewReadCache creates an empty read cache.
func NewReadCache() *ReadCache {
	return &ReadCache{
		entries: make(map[string]*cacheEntry),
	}
}

// Get returns a cached mote if the file hasn't been modified since caching.
// Returns (nil, false) on cache miss or stale entry.
func (rc *ReadCache) Get(moteID, path string) (*Mote, bool) {
	rc.mu.RLock()
	e, ok := rc.entries[moteID]
	rc.mu.RUnlock()
	if !ok {
		return nil, false
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if !info.ModTime().Equal(e.mtime) {
		return nil, false
	}
	return e.mote, true
}

// Put stores a parsed mote in the cache with the file's current mtime.
func (rc *ReadCache) Put(moteID, path string, m *Mote) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	rc.mu.Lock()
	rc.entries[moteID] = &cacheEntry{
		mote:  m,
		mtime: info.ModTime(),
	}
	rc.mu.Unlock()
}
