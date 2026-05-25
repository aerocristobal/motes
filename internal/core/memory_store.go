// SPDX-License-Identifier: MIT
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MemoryBodyMaxLen is the hard cap on a memory body, in bytes.
// 1000 chars matches the ~50-token compact-mode budget for `mote prime`
// under MCP hooks. Longer durable notes belong in `mote add --type=lesson`.
const MemoryBodyMaxLen = 1000

// memoryFilename is the on-disk filename relative to .memory/.
const memoryFilename = "memory.json"

// memoryLockFilename is the file-lock path relative to .memory/.
const memoryLockFilename = ".memory.lock"

// Sentinel errors. Tests and CLI callers use errors.Is() to discriminate.
var (
	ErrMemoryNotFound    = errors.New("memory not found")
	ErrMemoryExists      = errors.New("memory already exists")
	ErrMemoryEmptyBody   = errors.New("memory body cannot be empty")
	ErrMemoryBodyTooLong = fmt.Errorf("memory body exceeds %d character limit", MemoryBodyMaxLen)
	ErrMemoryEmptyKey    = errors.New("memory key cannot be empty")
)

// MemoryRecord is one persisted memory.
type MemoryRecord struct {
	Key       string    `json:"key"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PutOpts modifies Put behavior.
type PutOpts struct {
	// NoClobber returns ErrMemoryExists instead of overwriting when a
	// memory with the given key already exists. Default false = overwrite.
	NoClobber bool
}

// MemoryStore persists key/body memory pairs at .memory/memory.json and
// writes an audit entry for every Put and Delete.
//
// Memories sit OUTSIDE the mote graph — no scoring, no edges, no concept
// index. They exist to seed `mote prime` output with short durable rules.
// For knowledge with relationships, use `mote add --type=lesson` instead.
type MemoryStore struct {
	root    string
	auditor *AuditLogger
}

// NewMemoryStore returns a store rooted at the given .memory/ path.
func NewMemoryStore(root string) *MemoryStore {
	return &MemoryStore{
		root:    root,
		auditor: NewAuditLogger(root),
	}
}

func (s *MemoryStore) path() string     { return filepath.Join(s.root, memoryFilename) }
func (s *MemoryStore) lockPath() string { return filepath.Join(s.root, memoryLockFilename) }

// fileFormat is the on-disk envelope. Keeping it explicit (rather than
// marshaling a bare slice) leaves room for future top-level metadata
// (schema version, last-rotated-at, etc.) without a migration.
type fileFormat struct {
	Memories []MemoryRecord `json:"memories"`
}

// loadAll reads the store file. Returns an empty slice if the file
// does not exist yet. Caller must hold the file lock.
func (s *MemoryStore) loadAll() ([]MemoryRecord, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read memory store: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var ff fileFormat
	if err := json.Unmarshal(data, &ff); err != nil {
		return nil, fmt.Errorf("parse memory store: %w", err)
	}
	return ff.Memories, nil
}

// saveAll writes records atomically. Caller must hold the file lock.
func (s *MemoryStore) saveAll(records []MemoryRecord) error {
	if records == nil {
		records = []MemoryRecord{}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Key < records[j].Key })
	data, err := json.MarshalIndent(fileFormat{Memories: records}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal memory store: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	return AtomicWrite(s.path(), data, 0644)
}

// validateBody enforces the body invariants Put requires.
func validateBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return ErrMemoryEmptyBody
	}
	if len(body) > MemoryBodyMaxLen {
		return ErrMemoryBodyTooLong
	}
	return nil
}

// Put inserts or overwrites the memory with the given key. The actor
// is used only for the audit entry (env-fallback if empty). Returns
// the persisted record (with stamped timestamps) on success.
//
// Errors:
//   - ErrMemoryEmptyKey if key is empty.
//   - ErrMemoryEmptyBody if body is empty or whitespace-only.
//   - ErrMemoryBodyTooLong if body exceeds MemoryBodyMaxLen bytes.
//   - ErrMemoryExists if opts.NoClobber and the key already exists.
func (s *MemoryStore) Put(key, body, actor string, opts PutOpts) (*MemoryRecord, error) {
	if key == "" {
		return nil, ErrMemoryEmptyKey
	}
	if err := validateBody(body); err != nil {
		return nil, err
	}

	lock := NewFileLock(s.lockPath())
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock memory store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	records, err := s.loadAll()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var rec MemoryRecord
	var updated bool
	for i := range records {
		if records[i].Key == key {
			if opts.NoClobber {
				return nil, ErrMemoryExists
			}
			records[i].Body = body
			records[i].UpdatedAt = now
			rec = records[i]
			updated = true
			break
		}
	}
	if !updated {
		rec = MemoryRecord{
			Key:       key,
			Body:      body,
			CreatedAt: now,
			UpdatedAt: now,
		}
		records = append(records, rec)
	}

	if err := s.saveAll(records); err != nil {
		return nil, err
	}

	// Audit AFTER successful persist. The audit logger has its own lock,
	// disjoint from the memory store lock, so this does not deadlock.
	_ = s.auditor.Log(AuditEntry{
		Operation: "memory.put",
		MoteID:    key,
		AgentID:   actor,
	})
	return &rec, nil
}

// Get returns the record for key, or ErrMemoryNotFound.
func (s *MemoryStore) Get(key string) (*MemoryRecord, error) {
	lock := NewFileLock(s.lockPath())
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock memory store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	records, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].Key == key {
			rec := records[i]
			return &rec, nil
		}
	}
	return nil, ErrMemoryNotFound
}

// Has reports whether a memory with the given key exists. Errors during
// load are treated as "not found" so callers using Has for collision
// detection degrade gracefully — they will catch real errors on the
// subsequent Put.
func (s *MemoryStore) Has(key string) bool {
	_, err := s.Get(key)
	return err == nil
}

// Delete removes the memory with the given key. Returns ErrMemoryNotFound
// if no such memory exists. Writes a "memory.delete" audit entry on success.
func (s *MemoryStore) Delete(key, actor string) error {
	lock := NewFileLock(s.lockPath())
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("lock memory store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	records, err := s.loadAll()
	if err != nil {
		return err
	}
	idx := -1
	for i := range records {
		if records[i].Key == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrMemoryNotFound
	}
	records = append(records[:idx], records[idx+1:]...)
	if err := s.saveAll(records); err != nil {
		return err
	}
	_ = s.auditor.Log(AuditEntry{
		Operation: "memory.delete",
		MoteID:    key,
		AgentID:   actor,
	})
	return nil
}

// List returns all memories matching the case-insensitive substring
// (matched against both key and body). An empty substring returns all
// memories. Results are sorted ascending by key.
func (s *MemoryStore) List(substring string) ([]MemoryRecord, error) {
	lock := NewFileLock(s.lockPath())
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock memory store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	records, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	if substring == "" {
		// loadAll preserves stored order, which saveAll already sorts.
		return records, nil
	}
	needle := strings.ToLower(substring)
	var out []MemoryRecord
	for _, r := range records {
		if strings.Contains(strings.ToLower(r.Key), needle) ||
			strings.Contains(strings.ToLower(r.Body), needle) {
			out = append(out, r)
		}
	}
	return out, nil
}

// PutAutoKey slugifies body, resolves collisions with a -2/-3/... suffix,
// and persists. Returns the persisted record. Useful for `mote remember`
// when no explicit --key is given.
func (s *MemoryStore) PutAutoKey(body, actor string) (*MemoryRecord, error) {
	if err := validateBody(body); err != nil {
		return nil, err
	}
	base := Slugify(body)
	if base == "" {
		return nil, fmt.Errorf("cannot derive key from body: %w", ErrMemoryEmptyKey)
	}
	key := base
	for n := 2; s.Has(key); n++ {
		key = fmt.Sprintf("%s-%d", base, n)
	}
	return s.Put(key, body, actor, PutOpts{})
}
