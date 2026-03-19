package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry records a single mutating operation on the mote store.
type AuditEntry struct {
	Operation string   `json:"operation"`            // create|update|delete|link|unlink
	MoteID    string   `json:"mote_id"`
	AgentID   string   `json:"agent_id"`
	Timestamp string   `json:"timestamp"`            // RFC3339
	FieldsSet []string `json:"fields_set,omitempty"` // fields changed (update only)
}

// AuditLogger appends structured entries to .memory/audit.jsonl.
type AuditLogger struct {
	root string
}

// NewAuditLogger creates an AuditLogger rooted at the given .memory directory.
func NewAuditLogger(root string) *AuditLogger {
	return &AuditLogger{root: root}
}

// Log writes an audit entry to audit.jsonl under file lock.
// Best-effort: errors are printed to stderr but do not fail the caller.
func (al *AuditLogger) Log(entry AuditEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.AgentID == "" {
		entry.AgentID = ResolveAgentID()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit: marshal error: %v\n", err)
		return err
	}
	data = append(data, '\n')

	lockPath := filepath.Join(al.root, ".audit.lock")
	lock := NewFileLock(lockPath)
	if err := lock.Lock(); err != nil {
		fmt.Fprintf(os.Stderr, "audit: lock error: %v\n", err)
		return err
	}
	defer lock.Unlock()

	auditPath := filepath.Join(al.root, "audit.jsonl")
	f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit: open error: %v\n", err)
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "audit: write error: %v\n", err)
		return err
	}
	return nil
}
