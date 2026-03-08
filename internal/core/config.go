package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Scoring ScoringConfig `yaml:"scoring"`
	Priming PrimingConfig `yaml:"priming"`
	Dream   DreamConfig   `yaml:"dream"`
	Strata  StrataConfig  `yaml:"strata"`
	Trash   TrashConfig   `yaml:"trash"`
}

type TrashConfig struct {
	RetentionDays int `yaml:"retention_days"`
}

type ScoringConfig struct {
	EdgeBonuses       map[string]float64 `yaml:"edge_bonuses"`
	StatusPenalties   map[string]float64 `yaml:"status_penalties"`
	RecencyDecay      RecencyDecayConfig `yaml:"recency_decay"`
	RetrievalStrength RetrievalConfig    `yaml:"retrieval_strength"`
	Salience          map[string]float64 `yaml:"salience"`
	ExploreTypeBonus  float64            `yaml:"explore_type_bonus"`
	TagSpecificity    TagSpecConfig      `yaml:"tag_specificity"`
	InterferencePenalty float64          `yaml:"interference_per_contradiction"`
	MaxResults        int                `yaml:"max_results"`
	MaxHops           int                `yaml:"max_hops"`
	MinThreshold      float64            `yaml:"min_relevance_threshold"`
}

type RecencyDecayConfig struct {
	Tiers []RecencyTier `yaml:"tiers"`
}

type RecencyTier struct {
	MaxDays *int    `yaml:"max_days"` // nil means unbounded (90+ or never)
	Factor  float64 `yaml:"factor"`
}

type RetrievalConfig struct {
	PerAccess float64 `yaml:"per_access_bonus"`
	MaxBonus  float64 `yaml:"max_bonus"`
}

type TagSpecConfig struct {
	Weight            float64 `yaml:"weight"`
	OverloadThreshold int     `yaml:"overload_threshold"`
}

type PrimingConfig struct {
	Signals []SignalConfig `yaml:"signals"`
}

type SignalConfig struct {
	Name        string  `yaml:"name"`
	Type        string  `yaml:"type"` // built_in | co_access
	Description string  `yaml:"description,omitempty"`
	TriggerTags []string `yaml:"trigger_tags,omitempty"`
	BoostTags   []string `yaml:"boost_tags,omitempty"`
	BoostAmount float64  `yaml:"boost_amount,omitempty"`
}

type DreamConfig struct {
	ScheduleHintDays    int              `yaml:"schedule_hint_days"`
	ReviewMode          string           `yaml:"review_mode"` // "auto" | "manual"
	ConfidenceThreshold float64          `yaml:"confidence_threshold"`
	Provider            DreamProvider    `yaml:"provider"`
	Batching            BatchingConfig   `yaml:"batching"`
	Reconciliation      ReconConfig      `yaml:"reconciliation"`
	PreScan             PreScanConfig    `yaml:"pre_scan"`
	Journal             JournalConfig    `yaml:"journal"`
	Interrupts          InterruptConfig  `yaml:"interrupts"`
}

type DreamProvider struct {
	Batch          ProviderEntry `yaml:"batch"`
	Reconciliation ProviderEntry `yaml:"reconciliation"`
}

type ProviderEntry struct {
	Backend string `yaml:"backend"`
	Auth    string `yaml:"auth"`
	Model   string `yaml:"model"`
}

type BatchingConfig struct {
	Strategy             string  `yaml:"strategy"`
	MaxMotesPerBatch     int     `yaml:"max_motes_per_batch"`
	ClusteredFraction    float64 `yaml:"clustered_fraction"`
	MaxConcurrent        int     `yaml:"max_concurrent"`
	SelfConsistencyRuns  int     `yaml:"self_consistency_runs"` // default 1 (disabled), 3 for voting
}

type ReconConfig struct {
	Enabled        bool `yaml:"enabled"`
	MaxRefetchMotes int  `yaml:"max_refetch_motes"`
}

type PreScanConfig struct {
	LinkCandidateMinSharedTags int                      `yaml:"link_candidate_min_shared_tags"`
	StalenessThresholdDays     int                      `yaml:"staleness_threshold_days"`
	TagOverloadThreshold       int                      `yaml:"tag_overload_threshold"`
	ThemeGrowthThresholdPct    int                      `yaml:"theme_growth_threshold_pct"`
	CompressionMinWords        int                      `yaml:"compression_min_words"`
	ContentSimilarity          ContentSimilarityConfig  `yaml:"content_similarity"`
	MergeSimilarityMultiplier  float64                  `yaml:"merge_similarity_multiplier"`
}

// ContentSimilarityConfig controls content-based semantic linking.
type ContentSimilarityConfig struct {
	Enabled      bool    `yaml:"enabled"`
	TopK         int     `yaml:"top_k"`
	MinScore     float64 `yaml:"min_score"`
	MaxTerms     int     `yaml:"max_terms"`
	PrimingBoost float64 `yaml:"priming_boost"`
}

type JournalConfig struct {
	MaxTokens int `yaml:"max_tokens"`
}

type InterruptConfig struct {
	HighSeverityMotePct int `yaml:"high_severity_mote_pct"`
}

type StrataConfig struct {
	Chunking         ChunkingConfig     `yaml:"chunking"`
	Retrieval        RetrievalStrata    `yaml:"retrieval"`
	ContextAugment   ContextAugConfig   `yaml:"context_augment"`
	Crystallization  CrystallizeConfig  `yaml:"crystallization"`
}

type ChunkingConfig struct {
	Strategy      string `yaml:"strategy"`
	MaxChunkTokens int   `yaml:"max_chunk_tokens"`
	OverlapTokens  int   `yaml:"overlap_tokens"`
}

type RetrievalStrata struct {
	DefaultTopK       int     `yaml:"default_top_k"`
	MinRelevanceScore float64 `yaml:"min_relevance_score"`
}

type ContextAugConfig struct {
	Enabled          bool `yaml:"enabled"`
	MaxAugmentCorpora int  `yaml:"max_augment_corpora"`
	ChunksPerCorpus  int  `yaml:"chunks_per_corpus"`
}

type CrystallizeConfig struct {
	MinQueries             int `yaml:"min_queries"`
	MinSessions            int `yaml:"min_sessions"`
	StalenessThresholdDays int `yaml:"staleness_threshold_days"`
}

func intPtr(v int) *int { return &v }

// DefaultConfig returns the PRD-specified defaults for all configuration.
func DefaultConfig() *Config {
	return &Config{
		Scoring: ScoringConfig{
			EdgeBonuses: map[string]float64{
				"builds_on":   0.3,
				"supersedes":  0.3,
				"caused_by":   0.2,
				"informed_by": 0.2,
				"relates_to":  0.1,
			},
			StatusPenalties: map[string]float64{
				"deprecated": -0.5,
			},
			RecencyDecay: RecencyDecayConfig{
				Tiers: []RecencyTier{
					{MaxDays: intPtr(7), Factor: 1.0},
					{MaxDays: intPtr(30), Factor: 0.85},
					{MaxDays: intPtr(90), Factor: 0.65},
					{MaxDays: nil, Factor: 0.4},
				},
			},
			RetrievalStrength: RetrievalConfig{
				PerAccess: 0.03,
				MaxBonus:  0.15,
			},
			Salience: map[string]float64{
				"failure":   0.2,
				"revert":    0.2,
				"hotfix":    0.2,
				"discovery": 0.1,
				"normal":    0.0,
			},
			ExploreTypeBonus:    0.1,
			TagSpecificity:      TagSpecConfig{Weight: 0.2, OverloadThreshold: 15},
			InterferencePenalty: -0.1,
			MaxResults:          12,
			MaxHops:             2,
			MinThreshold:        0.25,
		},
		Priming: PrimingConfig{
			Signals: []SignalConfig{
				{Name: "git_branch", Type: "built_in", Description: "Current git branch name used as tag-match signal"},
				{Name: "recent_files", Type: "built_in", Description: "Files modified in last 30 min; paths parsed for keyword signals"},
				{Name: "prompt_keywords", Type: "built_in", Description: "Keywords extracted from user's initial prompt"},
			},
		},
		Dream: DreamConfig{
			ScheduleHintDays:    2,
			ReviewMode:          "auto",
			ConfidenceThreshold: 0.6,
			Provider: DreamProvider{
				Batch:          ProviderEntry{Backend: "claude-cli", Auth: "oauth", Model: "claude-sonnet-4-20250514"},
				Reconciliation: ProviderEntry{Backend: "claude-cli", Auth: "oauth", Model: "claude-opus-4-20250514"},
			},
			Batching: BatchingConfig{
				Strategy:          "hybrid",
				MaxMotesPerBatch:  10,
				ClusteredFraction: 0.6,
			},
			Reconciliation: ReconConfig{
				Enabled:        true,
				MaxRefetchMotes: 15,
			},
			PreScan: PreScanConfig{
				LinkCandidateMinSharedTags: 3,
				StalenessThresholdDays:     180,
				TagOverloadThreshold:       15,
				ThemeGrowthThresholdPct:    30,
				CompressionMinWords:        300,
				ContentSimilarity: ContentSimilarityConfig{
					Enabled:      true,
					TopK:         3,
					MinScore:     1.0,
					MaxTerms:     8,
					PrimingBoost: 0.15,
				},
				MergeSimilarityMultiplier: 2.0,
			},
			Journal: JournalConfig{MaxTokens: 2000},
			Interrupts: InterruptConfig{HighSeverityMotePct: 20},
		},
		Trash: TrashConfig{
			RetentionDays: 30,
		},
		Strata: StrataConfig{
			Chunking: ChunkingConfig{
				Strategy:       "heading-aware",
				MaxChunkTokens: 512,
				OverlapTokens:  50,
			},
			Retrieval: RetrievalStrata{
				DefaultTopK:       5,
				MinRelevanceScore: 0.3,
			},
			ContextAugment: ContextAugConfig{
				Enabled:          true,
				MaxAugmentCorpora: 2,
				ChunksPerCorpus:  3,
			},
			Crystallization: CrystallizeConfig{
				MinQueries:             5,
				MinSessions:            3,
				StalenessThresholdDays: 180,
			},
		},
	}
}

// SaveConfig writes config to .memory/config.yaml using atomic write.
func SaveConfig(root string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return AtomicWrite(filepath.Join(root, "config.yaml"), data, 0644)
}

// LoadConfig reads config from .memory/config.yaml, falling back to defaults.
func LoadConfig(root string) (*Config, error) {
	data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		return DefaultConfig(), nil
	}
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("bad config: %w", err)
	}
	return cfg, nil
}
