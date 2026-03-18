package dream

import (
	"encoding/json"
	"io"
	"time"
)

// DreamLogger emits structured JSON log lines to a writer (typically os.Stderr)
// for machine-parseable dream cycle output in cron/systemd environments.
type DreamLogger struct {
	w       io.Writer
	enabled bool
}

// LogEntry is a single structured log line emitted during a dream cycle.
type LogEntry struct {
	Timestamp   string `json:"ts"`
	Level       string `json:"level"`
	Phase       string `json:"phase"`
	BatchIndex  int    `json:"batch_idx,omitempty"`
	Message     string `json:"msg"`
	VisionCount int    `json:"vision_count,omitempty"`
	DurationMs  int64  `json:"duration_ms,omitempty"`
	MoteCount   int    `json:"mote_count,omitempty"`
	Error       string `json:"error,omitempty"`
	PromptLen   int    `json:"prompt_len,omitempty"`
}

// NewDreamLogger creates a logger that writes to w. If enabled is false, Log is a no-op.
func NewDreamLogger(w io.Writer, enabled bool) *DreamLogger {
	return &DreamLogger{w: w, enabled: enabled}
}

// Log emits a single JSON line. Fills in Timestamp automatically.
func (dl *DreamLogger) Log(entry LogEntry) {
	if !dl.enabled || dl.w == nil {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	line = append(line, '\n')
	dl.w.Write(line)
}
