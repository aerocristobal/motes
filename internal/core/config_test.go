// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig_Populated(t *testing.T) {
	cfg := DefaultConfig()

	// Scoring
	if cfg.Scoring.MaxResults != 12 {
		t.Errorf("MaxResults: got %d, want 12", cfg.Scoring.MaxResults)
	}
	if cfg.Scoring.MaxHops != 2 {
		t.Errorf("MaxHops: got %d, want 2", cfg.Scoring.MaxHops)
	}
	if cfg.Scoring.MinThreshold != 0.25 {
		t.Errorf("MinThreshold: got %f, want 0.25", cfg.Scoring.MinThreshold)
	}
	if cfg.Scoring.EdgeBonuses["builds_on"] != 0.3 {
		t.Errorf("EdgeBonuses[builds_on]: got %f, want 0.3", cfg.Scoring.EdgeBonuses["builds_on"])
	}
	if cfg.Scoring.StatusPenalties["deprecated"] != -0.5 {
		t.Errorf("StatusPenalties[deprecated]: got %f, want -0.5", cfg.Scoring.StatusPenalties["deprecated"])
	}

	// Recency tiers
	tiers := cfg.Scoring.RecencyDecay.Tiers
	if len(tiers) != 4 {
		t.Fatalf("RecencyDecay.Tiers: got %d tiers, want 4", len(tiers))
	}
	if *tiers[0].MaxDays != 7 || tiers[0].Factor != 1.0 {
		t.Errorf("Tier 0: got max=%v factor=%f", tiers[0].MaxDays, tiers[0].Factor)
	}
	if tiers[3].MaxDays != nil {
		t.Errorf("Tier 3 MaxDays should be nil, got %v", tiers[3].MaxDays)
	}
	if tiers[3].Factor != 0.4 {
		t.Errorf("Tier 3 Factor: got %f, want 0.4", tiers[3].Factor)
	}

	// Retrieval strength
	if cfg.Scoring.RetrievalStrength.PerAccess != 0.03 {
		t.Errorf("PerAccess: got %f, want 0.03", cfg.Scoring.RetrievalStrength.PerAccess)
	}
	if cfg.Scoring.RetrievalStrength.MaxBonus != 0.15 {
		t.Errorf("MaxBonus: got %f, want 0.15", cfg.Scoring.RetrievalStrength.MaxBonus)
	}

	// Salience
	if cfg.Scoring.Salience["failure"] != 0.2 {
		t.Errorf("Salience[failure]: got %f, want 0.2", cfg.Scoring.Salience["failure"])
	}
	if cfg.Scoring.ExploreTypeBonus != 0.1 {
		t.Errorf("ExploreTypeBonus: got %f, want 0.1", cfg.Scoring.ExploreTypeBonus)
	}

	// Priming signals
	if len(cfg.Priming.Signals) != 3 {
		t.Errorf("Signals: got %d, want 3", len(cfg.Priming.Signals))
	}

	// Dream
	if cfg.Dream.ScheduleHintDays != 2 {
		t.Errorf("ScheduleHintDays: got %d, want 2", cfg.Dream.ScheduleHintDays)
	}
	if cfg.Dream.Batching.MaxMotesPerBatch != 25 {
		t.Errorf("MaxMotesPerBatch: got %d, want 25", cfg.Dream.Batching.MaxMotesPerBatch)
	}

	// Strata
	if cfg.Strata.Retrieval.DefaultTopK != 5 {
		t.Errorf("DefaultTopK: got %d, want 5", cfg.Strata.Retrieval.DefaultTopK)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig with missing file should not error: %v", err)
	}
	if cfg.Scoring.MaxResults != 12 {
		t.Errorf("should fall back to defaults, MaxResults: got %d", cfg.Scoring.MaxResults)
	}
}

func TestLoadConfig_PartialYAMLMerge(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `scoring:
  max_results: 20
  max_hops: 3
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Overridden values
	if cfg.Scoring.MaxResults != 20 {
		t.Errorf("MaxResults: got %d, want 20", cfg.Scoring.MaxResults)
	}
	if cfg.Scoring.MaxHops != 3 {
		t.Errorf("MaxHops: got %d, want 3", cfg.Scoring.MaxHops)
	}

	// Preserved defaults
	if cfg.Scoring.MinThreshold != 0.25 {
		t.Errorf("MinThreshold should keep default 0.25, got %f", cfg.Scoring.MinThreshold)
	}
	if cfg.Dream.ScheduleHintDays != 2 {
		t.Errorf("ScheduleHintDays should keep default 2, got %d", cfg.Dream.ScheduleHintDays)
	}
}

func TestSaveConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()

	// Modify some values
	cfg.Scoring.MaxResults = 42
	cfg.Dream.ScheduleHintDays = 7

	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Modified values persisted
	if loaded.Scoring.MaxResults != 42 {
		t.Errorf("MaxResults: got %d, want 42", loaded.Scoring.MaxResults)
	}
	if loaded.Dream.ScheduleHintDays != 7 {
		t.Errorf("ScheduleHintDays: got %d, want 7", loaded.Dream.ScheduleHintDays)
	}

	// Unmodified values survived round-trip
	if loaded.Scoring.MinThreshold != 0.25 {
		t.Errorf("MinThreshold: got %f, want 0.25", loaded.Scoring.MinThreshold)
	}
	if loaded.Scoring.MaxHops != 2 {
		t.Errorf("MaxHops: got %d, want 2", loaded.Scoring.MaxHops)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("{{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestDefaultConfig_DoctorDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Doctor.MaxAvgLinks != 8.0 {
		t.Errorf("MaxAvgLinks: got %f, want 8.0", cfg.Doctor.MaxAvgLinks)
	}
	if cfg.Doctor.MaxChainDepth != 10 {
		t.Errorf("MaxChainDepth: got %d, want 10", cfg.Doctor.MaxChainDepth)
	}
	if cfg.Doctor.SingletonPct != 50.0 {
		t.Errorf("SingletonPct: got %f, want 50.0", cfg.Doctor.SingletonPct)
	}
}

func TestLoadConfig_DoctorSection(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `doctor:
  max_avg_links: 5.0
  max_chain_depth: 7
  singleton_pct: 30.0
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Doctor.MaxAvgLinks != 5.0 {
		t.Errorf("MaxAvgLinks: got %f, want 5.0", cfg.Doctor.MaxAvgLinks)
	}
	if cfg.Doctor.MaxChainDepth != 7 {
		t.Errorf("MaxChainDepth: got %d, want 7", cfg.Doctor.MaxChainDepth)
	}
	if cfg.Doctor.SingletonPct != 30.0 {
		t.Errorf("SingletonPct: got %f, want 30.0", cfg.Doctor.SingletonPct)
	}
}

func TestLoadConfig_LensModeConfig(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `dream:
  batching:
    lens_mode:
      enabled: true
      lenses: ["structural", "survivorship_bias"]
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Dream.Batching.LensMode.Enabled {
		t.Error("expected lens_mode.enabled = true")
	}
	if len(cfg.Dream.Batching.LensMode.Lenses) != 2 {
		t.Errorf("expected 2 lenses, got %d", len(cfg.Dream.Batching.LensMode.Lenses))
	}
	if cfg.Dream.Batching.LensMode.Lenses[0] != "structural" {
		t.Errorf("expected first lens 'structural', got %q", cfg.Dream.Batching.LensMode.Lenses[0])
	}
}

func TestLoadConfig_LensModeDisabledByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Dream.Batching.LensMode.Enabled {
		t.Error("lens_mode should be disabled by default")
	}
	if len(cfg.Dream.Batching.LensMode.Lenses) != 0 {
		t.Errorf("expected empty lenses by default, got %v", cfg.Dream.Batching.LensMode.Lenses)
	}
}

func TestLoadConfig_DoctorMissing_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	// No config.yaml — should fall back to defaults
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Doctor.MaxAvgLinks != 8.0 {
		t.Errorf("MaxAvgLinks: got %f, want 8.0 (default)", cfg.Doctor.MaxAvgLinks)
	}
}
