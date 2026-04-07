// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDreamLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDreamLogger(&buf, true)

	logger.Log(LogEntry{
		Level:      "info",
		Phase:      "batch",
		BatchIndex: 1,
		Message:    "batch start",
		MoteCount:  5,
	})

	output := buf.String()
	if !strings.HasSuffix(output, "\n") {
		t.Fatal("expected trailing newline")
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry.Level != "info" {
		t.Errorf("level = %q, want %q", entry.Level, "info")
	}
	if entry.Phase != "batch" {
		t.Errorf("phase = %q, want %q", entry.Phase, "batch")
	}
	if entry.BatchIndex != 1 {
		t.Errorf("batch_idx = %d, want 1", entry.BatchIndex)
	}
	if entry.MoteCount != 5 {
		t.Errorf("mote_count = %d, want 5", entry.MoteCount)
	}
	if entry.Timestamp == "" {
		t.Error("expected auto-filled timestamp")
	}
}

func TestDreamLogger_Disabled(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDreamLogger(&buf, false)

	logger.Log(LogEntry{Level: "info", Message: "should not appear"})

	if buf.Len() != 0 {
		t.Errorf("disabled logger wrote %d bytes", buf.Len())
	}
}

func TestDreamLogger_ErrorEntry(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDreamLogger(&buf, true)

	logger.Log(LogEntry{
		Level:     "error",
		Phase:     "batch",
		BatchIndex: 2,
		Message:   "batch invoke failed",
		Error:     "connection timeout",
		PromptLen: 4096,
	})

	var entry LogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry.Error != "connection timeout" {
		t.Errorf("error = %q, want %q", entry.Error, "connection timeout")
	}
	if entry.PromptLen != 4096 {
		t.Errorf("prompt_len = %d, want 4096", entry.PromptLen)
	}
}

func TestDreamLogger_OmitsZeroFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDreamLogger(&buf, true)

	logger.Log(LogEntry{
		Level:   "info",
		Phase:   "reconcile",
		Message: "reconciliation start",
	})

	output := strings.TrimSpace(buf.String())
	// batch_idx, vision_count, etc. should be omitted when zero
	if strings.Contains(output, "batch_idx") {
		t.Error("expected batch_idx to be omitted when zero")
	}
	if strings.Contains(output, "vision_count") {
		t.Error("expected vision_count to be omitted when zero")
	}
	if strings.Contains(output, "error") {
		t.Error("expected error to be omitted when empty")
	}
}
