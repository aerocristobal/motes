package dream

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"motes/internal/core"
)

func TestVotingConfigLabel(t *testing.T) {
	tests := []struct {
		name string
		cfg  core.DreamConfig
		want string
	}{
		{
			name: "single sonnet",
			cfg: core.DreamConfig{
				Batching: core.BatchingConfig{SelfConsistencyRuns: 1},
				Provider: core.DreamProvider{Batch: core.ProviderEntry{Model: "claude-sonnet-4-20250514"}},
			},
			want: "1x-sonnet",
		},
		{
			name: "triple sonnet",
			cfg: core.DreamConfig{
				Batching: core.BatchingConfig{SelfConsistencyRuns: 3},
				Provider: core.DreamProvider{Batch: core.ProviderEntry{Model: "claude-sonnet-4-20250514"}},
			},
			want: "3x-sonnet",
		},
		{
			name: "zero defaults to 1",
			cfg: core.DreamConfig{
				Batching: core.BatchingConfig{SelfConsistencyRuns: 0},
				Provider: core.DreamProvider{Batch: core.ProviderEntry{Model: "claude-sonnet-4-20250514"}},
			},
			want: "1x-sonnet",
		},
		{
			name: "haiku model",
			cfg: core.DreamConfig{
				Batching: core.BatchingConfig{SelfConsistencyRuns: 2},
				Provider: core.DreamProvider{Batch: core.ProviderEntry{Model: "claude-haiku-4-5-20251001"}},
			},
			want: "2x-haiku",
		},
		{
			name: "opus model",
			cfg: core.DreamConfig{
				Batching: core.BatchingConfig{SelfConsistencyRuns: 1},
				Provider: core.DreamProvider{Batch: core.ProviderEntry{Model: "claude-opus-4-20250514"}},
			},
			want: "1x-opus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VotingConfigLabel(tt.cfg)
			if got != tt.want {
				t.Errorf("VotingConfigLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompareConfigs(t *testing.T) {
	entries := []QualityEntry{
		{VotingConfig: "3x-sonnet", Batches: 10, ReconVisions: 5, ReconFilterRate: 0.2, AvgConfidence: 0.7, EstimatedCost: 1.50, CostPerVision: 0.30},
		{VotingConfig: "3x-sonnet", Batches: 12, ReconVisions: 6, ReconFilterRate: 0.25, AvgConfidence: 0.72, EstimatedCost: 1.80, CostPerVision: 0.30},
		{VotingConfig: "1x-sonnet", Batches: 10, ReconVisions: 8, ReconFilterRate: 0.1, AvgConfidence: 0.68, EstimatedCost: 0.50, CostPerVision: 0.0625},
	}

	comparisons := CompareConfigs(entries)
	if len(comparisons) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(comparisons))
	}

	// Sorted alphabetically: 1x-sonnet first
	if comparisons[0].Config != "1x-sonnet" {
		t.Errorf("expected first config '1x-sonnet', got %q", comparisons[0].Config)
	}
	if comparisons[0].Cycles != 1 {
		t.Errorf("expected 1 cycle for 1x-sonnet, got %d", comparisons[0].Cycles)
	}

	if comparisons[1].Config != "3x-sonnet" {
		t.Errorf("expected second config '3x-sonnet', got %q", comparisons[1].Config)
	}
	if comparisons[1].Cycles != 2 {
		t.Errorf("expected 2 cycles for 3x-sonnet, got %d", comparisons[1].Cycles)
	}

	// Check averages for 3x-sonnet
	expectedCost := (1.50 + 1.80) / 2
	if abs(comparisons[1].AvgCostPerCycle-expectedCost) > 0.001 {
		t.Errorf("expected avg cost %.2f, got %.2f", expectedCost, comparisons[1].AvgCostPerCycle)
	}
}

func TestQualityLedgerRoundTrip(t *testing.T) {
	// Use a temp dir for global storage
	tmpDir := t.TempDir()
	nodesDir := filepath.Join(tmpDir, "nodes")
	os.MkdirAll(nodesDir, 0755)
	t.Setenv("MOTE_GLOBAL_ROOT", tmpDir)

	entry := QualityEntry{
		Timestamp:    "2026-03-28T02:00:00Z",
		Project:      "testproj",
		VotingConfig: "3x-sonnet",
		Batches:      10,
		BatchVisions: 15,
		ReconVisions: 12,
	}

	if err := AppendQualityEntry(entry); err != nil {
		t.Fatalf("AppendQualityEntry: %v", err)
	}

	entries, err := ReadQualityLedger()
	if err != nil {
		t.Fatalf("ReadQualityLedger: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Project != "testproj" {
		t.Errorf("expected project 'testproj', got %q", entries[0].Project)
	}
	if entries[0].BatchVisions != 15 {
		t.Errorf("expected 15 batch visions, got %d", entries[0].BatchVisions)
	}

	// Append another
	entry2 := QualityEntry{
		Timestamp:    "2026-03-29T02:00:00Z",
		Project:      "testproj",
		VotingConfig: "1x-sonnet",
		Batches:      10,
		ReconVisions: 8,
	}
	if err := AppendQualityEntry(entry2); err != nil {
		t.Fatalf("AppendQualityEntry: %v", err)
	}

	entries, err = ReadQualityLedger()
	if err != nil {
		t.Fatalf("ReadQualityLedger: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestUpdateLastEntryAutoApply(t *testing.T) {
	tmpDir := t.TempDir()
	nodesDir := filepath.Join(tmpDir, "nodes")
	os.MkdirAll(nodesDir, 0755)
	t.Setenv("MOTE_GLOBAL_ROOT", tmpDir)

	// Write initial entry
	entry := QualityEntry{
		Timestamp:    "2026-03-28T02:00:00Z",
		Project:      "testproj",
		VotingConfig: "3x-sonnet",
		Batches:      10,
	}
	if err := AppendQualityEntry(entry); err != nil {
		t.Fatalf("AppendQualityEntry: %v", err)
	}

	// Update with auto-apply stats
	if err := UpdateLastEntryAutoApply(8, 2, 0.72); err != nil {
		t.Fatalf("UpdateLastEntryAutoApply: %v", err)
	}

	entries, err := ReadQualityLedger()
	if err != nil {
		t.Fatalf("ReadQualityLedger: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].AutoApplied != 8 {
		t.Errorf("expected 8 applied, got %d", entries[0].AutoApplied)
	}
	if entries[0].Deferred != 2 {
		t.Errorf("expected 2 deferred, got %d", entries[0].Deferred)
	}
	if abs(entries[0].AvgConfidence-0.72) > 0.001 {
		t.Errorf("expected avg confidence 0.72, got %.3f", entries[0].AvgConfidence)
	}
}

func TestRunLogEntryQualityFields(t *testing.T) {
	entry := RunLogEntry{
		Timestamp:       "2026-03-28T02:00:00Z",
		Status:          "complete",
		Batches:         10,
		Visions:         12,
		VotingConfig:    "3x-sonnet",
		BatchVisions:    15,
		ReconVisions:    12,
		ReconFilterRate: 0.2,
		AutoApplied:     10,
		Deferred:        2,
		AvgConfidence:   0.72,
		AvgAgreement:    0.85,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded RunLogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.VotingConfig != "3x-sonnet" {
		t.Errorf("expected voting_config '3x-sonnet', got %q", decoded.VotingConfig)
	}
	if decoded.BatchVisions != 15 {
		t.Errorf("expected batch_visions 15, got %d", decoded.BatchVisions)
	}
	if decoded.ReconFilterRate != 0.2 {
		t.Errorf("expected recon_filter_rate 0.2, got %f", decoded.ReconFilterRate)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
