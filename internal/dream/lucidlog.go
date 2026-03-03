package dream

import (
	"encoding/json"
	"os"
	"time"
)

// LucidLog accumulates findings across dream batches.
type LucidLog struct {
	ObservedPatterns []Pattern       `json:"observed_patterns"`
	Tensions         []Tension       `json:"tensions"`
	VisionsSummary   []VisionSummary `json:"visions_summary"`
	Interrupts       []Interrupt     `json:"interrupts"`
	StrataHealth     []StrataFlag    `json:"strata_health"`
	Metadata         LucidMetadata   `json:"metadata"`
	maxTokens        int
}

// NewLucidLog creates an empty lucid log with the given token budget.
func NewLucidLog(maxTokens int) *LucidLog {
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	return &LucidLog{maxTokens: maxTokens}
}

// LoadLucidLog reads the lucid log from disk if it exists.
func LoadLucidLog(path string, maxTokens int) *LucidLog {
	ll := NewLucidLog(maxTokens)
	data, err := os.ReadFile(path)
	if err != nil {
		return ll
	}
	_ = json.Unmarshal(data, ll)
	return ll
}

// Initialize resets the log for a new dream run.
func (ll *LucidLog) Initialize() {
	ll.ObservedPatterns = nil
	ll.Tensions = nil
	ll.VisionsSummary = nil
	ll.Interrupts = nil
	ll.StrataHealth = nil
	ll.Metadata = LucidMetadata{}
}

// Update merges new findings from a batch into the log.
func (ll *LucidLog) Update(updates LucidLogUpdates) {
	for _, p := range updates.ObservedPatterns {
		if existing := ll.findPattern(p.PatternID); existing != nil {
			existing.Merge(p)
		} else {
			ll.ObservedPatterns = append(ll.ObservedPatterns, p)
		}
	}
	ll.Tensions = append(ll.Tensions, updates.Tensions...)
	ll.VisionsSummary = append(ll.VisionsSummary, updates.VisionsSummary...)
	ll.Interrupts = append(ll.Interrupts, updates.Interrupts...)
	ll.StrataHealth = append(ll.StrataHealth, updates.StrataHealth...)
	ll.Metadata.BatchCount++
	ll.Metadata.LastUpdatedAt = time.Now().UTC()
	ll.pruneIfOverLimit()
}

// RecordBatchFailure adds a failure note to the log.
func (ll *LucidLog) RecordBatchFailure(batchIndex int, errMsg string) {
	ll.Interrupts = append(ll.Interrupts, Interrupt{
		Severity:    "medium",
		Description: errMsg,
		MoteID:      "",
	})
	ll.Metadata.BatchCount++
}

// Serialize returns the log as a JSON string for prompt inclusion.
func (ll *LucidLog) Serialize() string {
	data, _ := json.Marshal(ll)
	return string(data)
}

// Save writes the lucid log to disk.
func (ll *LucidLog) Save(path string) error {
	data, err := json.MarshalIndent(ll, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (ll *LucidLog) findPattern(id string) *Pattern {
	for i := range ll.ObservedPatterns {
		if ll.ObservedPatterns[i].PatternID == id {
			return &ll.ObservedPatterns[i]
		}
	}
	return nil
}

func (ll *LucidLog) pruneIfOverLimit() {
	for ll.estimateTokens() > ll.maxTokens {
		// Remove oldest patterns first, then visions summaries
		if len(ll.ObservedPatterns) > 3 {
			ll.ObservedPatterns = ll.ObservedPatterns[1:]
			continue
		}
		if len(ll.VisionsSummary) > 5 {
			ll.VisionsSummary = ll.VisionsSummary[1:]
			continue
		}
		break
	}
}

func (ll *LucidLog) estimateTokens() int {
	data, _ := json.Marshal(ll)
	// Rough estimate: ~4 chars per token
	return len(data) / 4
}
