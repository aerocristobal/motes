// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"motes/internal/core"
)

// QualityEntry is appended to the global ledger after each dream cycle.
type QualityEntry struct {
	Timestamp       string  `json:"timestamp"`
	Project         string  `json:"project"`
	VotingConfig    string  `json:"voting_config"`
	Batches         int     `json:"batches"`
	BatchVisions    int     `json:"batch_visions"`
	ReconVisions    int     `json:"recon_visions"`
	ReconFilterRate float64 `json:"recon_filter_rate"`
	AutoApplied     int     `json:"auto_applied"`
	Deferred        int     `json:"deferred"`
	AvgConfidence   float64 `json:"avg_confidence"`
	AvgAgreement    float64 `json:"avg_agreement"`
	DurationS       float64 `json:"duration_s"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	EstimatedCost   float64 `json:"estimated_cost"`
	CostPerVision   float64 `json:"cost_per_vision"`
}

// ConfigComparison holds aggregated stats for a single voting config.
type ConfigComparison struct {
	Config          string
	Cycles          int
	AvgVisionsPerBatch float64
	AvgReconFilter  float64
	AvgConfidence   float64
	AvgCostPerCycle float64
	AvgCostPerVision float64
}

// VotingConfigLabel builds a human-readable label from the dream config.
func VotingConfigLabel(cfg core.DreamConfig) string {
	scRuns := cfg.Batching.SelfConsistencyRuns
	if scRuns <= 0 {
		scRuns = 1
	}

	model := cfg.Provider.Batch.Model
	short := "sonnet"
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"):
		short = "opus"
	case strings.Contains(lower, "haiku"):
		short = "haiku"
	case strings.Contains(lower, "sonnet"):
		short = "sonnet"
	default:
		short = model
	}

	if scRuns == 1 {
		return "1x-" + short
	}
	return fmt.Sprintf("%dx-%s", scRuns, short)
}

// globalLedgerPath returns the path to ~/.claude/memory/dream_quality.jsonl.
func globalLedgerPath() (string, error) {
	globalDir, err := core.GlobalNodesDir()
	if err != nil {
		return "", err
	}
	// GlobalNodesDir returns ~/.claude/memory/nodes/, go up one level
	memDir := filepath.Dir(globalDir)
	return filepath.Join(memDir, "dream_quality.jsonl"), nil
}

// AppendQualityEntry writes a single entry to the global quality ledger.
func AppendQualityEntry(entry QualityEntry) error {
	path, err := globalLedgerPath()
	if err != nil {
		return err
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte{'\n'})
	return nil
}

// ReadQualityLedger reads all entries from the global quality ledger.
func ReadQualityLedger() ([]QualityEntry, error) {
	path, err := globalLedgerPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []QualityEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e QualityEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// UpdateLastEntryAutoApply updates the most recent global ledger entry with auto-apply counts
// and average confidence. Called after AutoApply completes.
func UpdateLastEntryAutoApply(applied, deferred int, avgConfidence float64) error {
	entries, err := ReadQualityLedger()
	if err != nil || len(entries) == 0 {
		return err
	}
	last := &entries[len(entries)-1]
	last.AutoApplied = applied
	last.Deferred = deferred
	last.AvgConfidence = avgConfidence

	// Rewrite the full ledger
	path, err := globalLedgerPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		f.Write(line)
		f.Write([]byte{'\n'})
	}
	return nil
}

// CompareConfigs groups quality entries by voting config and returns aggregated stats.
func CompareConfigs(entries []QualityEntry) []ConfigComparison {
	groups := make(map[string][]QualityEntry)
	for _, e := range entries {
		groups[e.VotingConfig] = append(groups[e.VotingConfig], e)
	}

	var result []ConfigComparison
	for config, group := range groups {
		c := ConfigComparison{
			Config: config,
			Cycles: len(group),
		}
		var totalVisionsPerBatch, totalFilter, totalConf, totalCost, totalCPV float64
		cpvCount := 0
		for _, e := range group {
			if e.Batches > 0 {
				totalVisionsPerBatch += float64(e.ReconVisions) / float64(e.Batches)
			}
			totalFilter += e.ReconFilterRate
			totalConf += e.AvgConfidence
			totalCost += e.EstimatedCost
			if e.CostPerVision > 0 && !math.IsInf(e.CostPerVision, 0) {
				totalCPV += e.CostPerVision
				cpvCount++
			}
		}
		n := float64(len(group))
		c.AvgVisionsPerBatch = totalVisionsPerBatch / n
		c.AvgReconFilter = totalFilter / n
		c.AvgConfidence = totalConf / n
		c.AvgCostPerCycle = totalCost / n
		if cpvCount > 0 {
			c.AvgCostPerVision = totalCPV / float64(cpvCount)
		}
		result = append(result, c)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Config < result[j].Config
	})
	return result
}
