// SPDX-License-Identifier: MIT
package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestStore builds a MemoryStore rooted at a fresh tempdir.
func newTestStore(t *testing.T) (*MemoryStore, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return NewMemoryStore(dir), dir
}

func TestMemoryStore_Put_NewKey_Persists(t *testing.T) {
	store, _ := newTestStore(t)
	rec, err := store.Put("auth-jwt", "auth uses JWT not sessions", "claude-code", PutOpts{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if rec.Key != "auth-jwt" {
		t.Errorf("key: want auth-jwt, got %q", rec.Key)
	}
	if rec.CreatedAt.IsZero() {
		t.Error("CreatedAt must be stamped")
	}
	if !rec.CreatedAt.Equal(rec.UpdatedAt) {
		t.Errorf("first Put: CreatedAt and UpdatedAt should be equal, got %v vs %v",
			rec.CreatedAt, rec.UpdatedAt)
	}

	got, err := store.Get("auth-jwt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Body != "auth uses JWT not sessions" {
		t.Errorf("body: want JWT…, got %q", got.Body)
	}
}

func TestMemoryStore_Put_OverwriteBumpsUpdatedAt(t *testing.T) {
	store, _ := newTestStore(t)
	rec1, err := store.Put("k", "first", "agent", PutOpts{})
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	rec2, err := store.Put("k", "second", "agent", PutOpts{})
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if !rec2.UpdatedAt.After(rec1.CreatedAt) {
		t.Errorf("UpdatedAt %v should be after first CreatedAt %v", rec2.UpdatedAt, rec1.CreatedAt)
	}
	if !rec2.CreatedAt.Equal(rec1.CreatedAt) {
		t.Errorf("CreatedAt should be preserved across overwrite: first=%v second=%v",
			rec1.CreatedAt, rec2.CreatedAt)
	}
	got, _ := store.Get("k")
	if got.Body != "second" {
		t.Errorf("overwrite: want body 'second', got %q", got.Body)
	}
}

func TestMemoryStore_Put_NoClobberRejects(t *testing.T) {
	store, _ := newTestStore(t)
	_, _ = store.Put("k", "first", "agent", PutOpts{})
	_, err := store.Put("k", "second", "agent", PutOpts{NoClobber: true})
	if !errors.Is(err, ErrMemoryExists) {
		t.Errorf("want ErrMemoryExists, got %v", err)
	}
	got, _ := store.Get("k")
	if got.Body != "first" {
		t.Errorf("body should remain 'first' after rejected overwrite, got %q", got.Body)
	}
}

func TestMemoryStore_Put_EmptyBodyRejected(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Put("k", "", "agent", PutOpts{}); !errors.Is(err, ErrMemoryEmptyBody) {
		t.Errorf("empty body: want ErrMemoryEmptyBody, got %v", err)
	}
	if _, err := store.Put("k", "   \n\t  ", "agent", PutOpts{}); !errors.Is(err, ErrMemoryEmptyBody) {
		t.Errorf("whitespace-only body: want ErrMemoryEmptyBody, got %v", err)
	}
}

func TestMemoryStore_Put_BodyTooLong(t *testing.T) {
	store, _ := newTestStore(t)
	body := strings.Repeat("x", MemoryBodyMaxLen+1)
	if _, err := store.Put("k", body, "agent", PutOpts{}); !errors.Is(err, ErrMemoryBodyTooLong) {
		t.Errorf("oversize body: want ErrMemoryBodyTooLong, got %v", err)
	}
	// Boundary: exactly MemoryBodyMaxLen is allowed.
	body = strings.Repeat("x", MemoryBodyMaxLen)
	if _, err := store.Put("k", body, "agent", PutOpts{}); err != nil {
		t.Errorf("exact-max body: want success, got %v", err)
	}
}

func TestMemoryStore_Put_EmptyKeyRejected(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Put("", "body", "agent", PutOpts{}); !errors.Is(err, ErrMemoryEmptyKey) {
		t.Errorf("empty key: want ErrMemoryEmptyKey, got %v", err)
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Get("nope"); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("want ErrMemoryNotFound, got %v", err)
	}
}

func TestMemoryStore_Delete_HappyAndMissing(t *testing.T) {
	store, _ := newTestStore(t)
	_, _ = store.Put("k", "body", "agent", PutOpts{})

	if err := store.Delete("k", "agent"); err != nil {
		t.Fatalf("delete existing: %v", err)
	}
	if _, err := store.Get("k"); !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("post-delete Get: want ErrMemoryNotFound, got %v", err)
	}
	if err := store.Delete("k", "agent"); !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("delete missing: want ErrMemoryNotFound, got %v", err)
	}
}

func TestMemoryStore_List_SortedByKey(t *testing.T) {
	store, _ := newTestStore(t)
	_, _ = store.Put("zeta", "z body", "agent", PutOpts{})
	_, _ = store.Put("alpha", "a body", "agent", PutOpts{})
	_, _ = store.Put("mike", "m body", "agent", PutOpts{})

	out, err := store.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 records, got %d", len(out))
	}
	if out[0].Key != "alpha" || out[1].Key != "mike" || out[2].Key != "zeta" {
		t.Errorf("not sorted by key: %v", []string{out[0].Key, out[1].Key, out[2].Key})
	}
}

func TestMemoryStore_List_SubstringFilter(t *testing.T) {
	store, _ := newTestStore(t)
	_, _ = store.Put("auth-jwt", "auth uses JWT not sessions", "agent", PutOpts{})
	_, _ = store.Put("race-flag", "always run tests with -race flag", "agent", PutOpts{})
	_, _ = store.Put("dolt-required", "dolt must be installed before tests", "agent", PutOpts{})

	// Key match (story Scenario 4: "dolt" matches the dolt-required key).
	out, err := store.List("dolt")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 1 || out[0].Key != "dolt-required" {
		t.Errorf("substring 'dolt': want [dolt-required], got %+v", keys(out))
	}

	// Body match.
	out, _ = store.List("JWT")
	if len(out) != 1 || out[0].Key != "auth-jwt" {
		t.Errorf("substring 'JWT' body: want [auth-jwt], got %+v", keys(out))
	}

	// Case-insensitive.
	out, _ = store.List("jwt")
	if len(out) != 1 || out[0].Key != "auth-jwt" {
		t.Errorf("substring 'jwt' (case-insensitive): want [auth-jwt], got %+v", keys(out))
	}

	// No matches.
	out, _ = store.List("nothing-matches")
	if len(out) != 0 {
		t.Errorf("no-match: want [], got %+v", keys(out))
	}
}

func TestMemoryStore_PutAutoKey_CollisionSuffix(t *testing.T) {
	store, _ := newTestStore(t)
	rec1, err := store.PutAutoKey("auth uses JWT", "agent")
	if err != nil {
		t.Fatalf("first PutAutoKey: %v", err)
	}
	if rec1.Key != "auth-uses-jwt" {
		t.Errorf("first auto key: want auth-uses-jwt, got %q", rec1.Key)
	}

	rec2, err := store.PutAutoKey("auth uses JWT", "agent")
	if err != nil {
		t.Fatalf("collision PutAutoKey: %v", err)
	}
	if rec2.Key != "auth-uses-jwt-2" {
		t.Errorf("collision suffix: want auth-uses-jwt-2, got %q", rec2.Key)
	}

	rec3, _ := store.PutAutoKey("auth uses JWT", "agent")
	if rec3.Key != "auth-uses-jwt-3" {
		t.Errorf("second collision: want auth-uses-jwt-3, got %q", rec3.Key)
	}
}

func TestMemoryStore_PutAutoKey_EmptySlugRejected(t *testing.T) {
	store, _ := newTestStore(t)
	_, err := store.PutAutoKey("!!!", "agent")
	if err == nil {
		t.Fatal("punctuation-only body should fail PutAutoKey")
	}
	if !errors.Is(err, ErrMemoryEmptyKey) {
		t.Errorf("want ErrMemoryEmptyKey wrapped, got %v", err)
	}
}

func TestMemoryStore_AtomicWrite_FileWellFormed(t *testing.T) {
	store, root := newTestStore(t)
	_, _ = store.Put("k1", "first body", "agent", PutOpts{})
	_, _ = store.Put("k2", "second body", "agent", PutOpts{})

	raw, err := os.ReadFile(filepath.Join(root, memoryFilename))
	if err != nil {
		t.Fatalf("read store file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("store file is empty after two Puts")
	}
	var ff fileFormat
	if err := json.Unmarshal(raw, &ff); err != nil {
		t.Fatalf("store file is not valid JSON: %v\n%s", err, raw)
	}
	if len(ff.Memories) != 2 {
		t.Errorf("want 2 records on disk, got %d", len(ff.Memories))
	}
}

func TestMemoryStore_AuditLogEntries(t *testing.T) {
	store, root := newTestStore(t)
	_, _ = store.Put("k", "body", "claude-code", PutOpts{})
	_ = store.Delete("k", "claude-code")

	entries := readAuditEntries(t, root)
	var seenPut, seenDel bool
	for _, e := range entries {
		if e.Operation == "memory.put" && e.MoteID == "k" && e.AgentID == "claude-code" {
			seenPut = true
		}
		if e.Operation == "memory.delete" && e.MoteID == "k" && e.AgentID == "claude-code" {
			seenDel = true
		}
	}
	if !seenPut {
		t.Error("missing memory.put audit entry")
	}
	if !seenDel {
		t.Error("missing memory.delete audit entry")
	}
}

func TestMemoryStore_Get_EmptyStore(t *testing.T) {
	store, _ := newTestStore(t)
	// File doesn't exist yet — Get should return ErrMemoryNotFound, not an I/O error.
	if _, err := store.Get("x"); !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("Get on missing store: want ErrMemoryNotFound, got %v", err)
	}
}

func TestMemoryStore_List_EmptyStore(t *testing.T) {
	store, _ := newTestStore(t)
	out, err := store.List("")
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("want empty slice, got %v", out)
	}
}

func keys(rs []MemoryRecord) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Key
	}
	return out
}
