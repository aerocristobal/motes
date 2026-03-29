package dream

import (
	"encoding/json"
	"time"
)

// Vision represents a single insight or recommendation from a dream cycle batch.
type Vision struct {
	Type        string   `json:"type"`         // link_suggestion, contradiction, tag_refinement, staleness, compression, signal
	Action      string   `json:"action"`       // add_link, remove, split_tag, deprecate, compress, add_signal
	SourceMotes []string `json:"source_motes"` // mote IDs involved
	TargetMotes []string `json:"target_motes,omitempty"`
	LinkType    string   `json:"link_type,omitempty"`
	Rationale   string   `json:"rationale"`
	Severity    string   `json:"severity"` // low, medium, high
	Tags        []string `json:"tags,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"` // 0.0-1.0, deterministic
	Agreement   float64  `json:"agreement,omitempty"`  // 0.0-1.0, fraction of runs that agreed
}

// DreamResult is the summary returned after a dream run.
type DreamResult struct {
	Status        string  `json:"status"` // clean, dry-run, complete, error
	Batches       int     `json:"batches"`
	Visions       int     `json:"visions"`
	InputTokens   int     `json:"input_tokens,omitempty"`
	OutputTokens  int     `json:"output_tokens,omitempty"`
	EstimatedCost float64 `json:"estimated_cost,omitempty"`
	BatchVisions  int     `json:"batch_visions,omitempty"` // Visions before reconciliation
}

// MotePair identifies two motes for link/contradiction analysis.
type MotePair struct {
	A          string
	B          string
	SharedTags []string
	Similarity float64 // BM25 score (0 if tag-based)
	Source     string  // "tag_overlap" | "content_similarity"
}

// TagOverload flags a tag with too many associated motes.
type TagOverload struct {
	Tag   string
	Count int
}

// SignalCandidate identifies a potential priming signal pattern.
type SignalCandidate struct {
	MoteID  string
	Pattern string
}

// MergeCluster identifies a group of highly similar motes that may be redundant.
type MergeCluster struct {
	MoteIDs       []string
	AvgSimilarity float64
}

// SummarizationCluster identifies a group of completed motes sharing tags that can be summarized.
type SummarizationCluster struct {
	SharedTags []string `json:"shared_tags"`
	MoteIDs    []string `json:"mote_ids"`
}

// StrataCrystallizationCandidate identifies frequently-queried strata topics.
type StrataCrystallizationCandidate struct {
	Corpus     string
	Query      string
	QueryCount int
}

// ConstellationEvolution tracks growth of a constellation's theme.
type ConstellationEvolution struct {
	ConstellationID string `json:"constellation_id"`
	Tag             string `json:"tag"`
	OldCount        int    `json:"old_count"`
	NewCount        int    `json:"new_count"`
}

// ActionCandidate identifies a mote that would benefit from action extraction.
type ActionCandidate struct {
	MoteID      string
	AccessCount int
}

// DominantMoteCandidate identifies a mote that disproportionately occupies prime output.
type DominantMoteCandidate struct {
	MoteID      string
	PrimeFreq   int // sessions primed in (out of last 10)
	AccessCount int
}

// DecayRiskCandidate identifies a high-value mote approaching the relevance threshold.
type DecayRiskCandidate struct {
	MoteID        string
	Weight        float64
	RecencyFactor float64
	Score         float64
	ScoreGap      float64 // score - min_relevance_threshold (positive = still above)
}

// ScanResult holds all dream pre-scan findings.
type ScanResult struct {
	LinkCandidates            []MotePair
	ContentLinkCandidates     []MotePair
	ContradictionCandidates   []MotePair
	OverloadedTags            []TagOverload
	StaleMotes                []string
	ConstellationEvolution    []ConstellationEvolution
	CompressionCandidates     []string
	UncrystallizedIssues      []string
	StrataCrystallization     []StrataCrystallizationCandidate
	SignalCandidates          []SignalCandidate
	MergeCandidates           []MergeCluster
	SummarizationCandidates   []SummarizationCluster
	ActionCandidates          []ActionCandidate
	DominantMotes             []DominantMoteCandidate
	DecayRiskMotes            []DecayRiskCandidate
}

// HasWork returns true if any scan category found candidates.
func (sr *ScanResult) HasWork() bool {
	return len(sr.LinkCandidates) > 0 ||
		len(sr.ContentLinkCandidates) > 0 ||
		len(sr.ContradictionCandidates) > 0 ||
		len(sr.OverloadedTags) > 0 ||
		len(sr.StaleMotes) > 0 ||
		len(sr.ConstellationEvolution) > 0 ||
		len(sr.CompressionCandidates) > 0 ||
		len(sr.UncrystallizedIssues) > 0 ||
		len(sr.StrataCrystallization) > 0 ||
		len(sr.SignalCandidates) > 0 ||
		len(sr.MergeCandidates) > 0 ||
		len(sr.SummarizationCandidates) > 0 ||
		len(sr.ActionCandidates) > 0 ||
		len(sr.DominantMotes) > 0 ||
		len(sr.DecayRiskMotes) > 0
}

// Batch groups motes for a single Claude invocation.
type Batch struct {
	Phase          string   // "clustered" | "interleaved"
	PrimaryCluster string   // tag cluster name (clustered only)
	MoteIDs        []string
	Tasks          []string
}

// RunLogEntry records a single dream execution in the log.
type RunLogEntry struct {
	Timestamp       string  `json:"timestamp"`
	Status          string  `json:"status"`
	Batches         int     `json:"batches"`
	Visions         int     `json:"visions"`
	DurationS       float64 `json:"duration_s"`
	InputTokens     int     `json:"input_tokens,omitempty"`
	OutputTokens    int     `json:"output_tokens,omitempty"`
	EstimatedCost   float64 `json:"estimated_cost,omitempty"`
	VotingConfig    string  `json:"voting_config,omitempty"`
	BatchVisions    int     `json:"batch_visions,omitempty"`
	ReconVisions    int     `json:"recon_visions,omitempty"`
	ReconFilterRate float64 `json:"recon_filter_rate,omitempty"`
	AutoApplied     int     `json:"auto_applied,omitempty"`
	Deferred        int     `json:"deferred,omitempty"`
	AvgConfidence   float64 `json:"avg_confidence,omitempty"`
	AvgAgreement    float64 `json:"avg_agreement,omitempty"`
}

// ReviewResult summarizes interactive vision review.
type ReviewResult struct {
	Accepted int
	Rejected int
	Deferred int
}

// Pattern represents an observed pattern in the lucid log.
type Pattern struct {
	PatternID   string `json:"pattern_id"`
	Description string `json:"description"`
	MoteIDs     []string `json:"mote_ids"`
	Strength    float64 `json:"strength"`
}

// UnmarshalJSON handles both string and object forms of Pattern.
// Claude sometimes returns patterns as plain strings instead of objects.
func (p *Pattern) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		p.Description = s
		p.Strength = 1.0
		return nil
	}
	type PatternAlias Pattern
	var alias PatternAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*p = Pattern(alias)
	return nil
}

// Merge combines an incoming pattern observation into this one.
func (p *Pattern) Merge(other Pattern) {
	p.Strength += other.Strength
	seen := make(map[string]bool, len(p.MoteIDs))
	for _, id := range p.MoteIDs {
		seen[id] = true
	}
	for _, id := range other.MoteIDs {
		if !seen[id] {
			p.MoteIDs = append(p.MoteIDs, id)
		}
	}
}

// Tension represents conflicting assertions found across batches.
type Tension struct {
	TensionID   string   `json:"tension_id"`
	Description string   `json:"description"`
	MoteIDs     []string `json:"mote_ids"`
}

// UnmarshalJSON handles both string and object forms of Tension.
func (t *Tension) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		t.Description = s
		return nil
	}
	type TensionAlias Tension
	var alias TensionAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*t = Tension(alias)
	return nil
}

// VisionSummary is a compressed reference to a vision in the lucid log.
type VisionSummary struct {
	Type    string   `json:"type"`
	MoteIDs []string `json:"mote_ids"`
	Batch   int      `json:"batch"`
}

// UnmarshalJSON handles both string and object forms of VisionSummary.
func (vs *VisionSummary) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		vs.Type = s
		return nil
	}
	type VisionSummaryAlias VisionSummary
	var alias VisionSummaryAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*vs = VisionSummary(alias)
	return nil
}

// Interrupt flags something requiring immediate attention.
type Interrupt struct {
	Severity    string `json:"severity"`
	Description string `json:"description"`
	MoteID      string `json:"mote_id"`
}

// StrataFlag records strata health observations.
type StrataFlag struct {
	Corpus      string `json:"corpus"`
	Issue       string `json:"issue"`
	Description string `json:"description"`
}

// LucidMetadata tracks lucid log state.
type LucidMetadata struct {
	BatchCount    int       `json:"batch_count"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

// LucidLogUpdates is the structure returned from Claude with updates to the log.
type LucidLogUpdates struct {
	ObservedPatterns []Pattern       `json:"observed_patterns,omitempty"`
	Tensions         []Tension       `json:"tensions,omitempty"`
	VisionsSummary   []VisionSummary `json:"visions_summary,omitempty"`
	Interrupts       []Interrupt     `json:"interrupts,omitempty"`
	StrataHealth     []StrataFlag    `json:"strata_health,omitempty"`
}

// UnmarshalJSON handles observed_patterns being a bare string, single object, or array.
func (lu *LucidLogUpdates) UnmarshalJSON(data []byte) error {
	type Alias struct {
		ObservedPatterns json.RawMessage `json:"observed_patterns,omitempty"`
		Tensions         []Tension       `json:"tensions,omitempty"`
		VisionsSummary   []VisionSummary `json:"visions_summary,omitempty"`
		Interrupts       []Interrupt     `json:"interrupts,omitempty"`
		StrataHealth     []StrataFlag    `json:"strata_health,omitempty"`
	}
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	lu.Tensions = alias.Tensions
	lu.VisionsSummary = alias.VisionsSummary
	lu.Interrupts = alias.Interrupts
	lu.StrataHealth = alias.StrataHealth

	if len(alias.ObservedPatterns) > 0 {
		switch alias.ObservedPatterns[0] {
		case '[':
			if err := json.Unmarshal(alias.ObservedPatterns, &lu.ObservedPatterns); err != nil {
				return err
			}
		case '"':
			var s string
			if err := json.Unmarshal(alias.ObservedPatterns, &s); err != nil {
				return err
			}
			lu.ObservedPatterns = []Pattern{{Description: s, Strength: 1.0}}
		case '{':
			var p Pattern
			if err := json.Unmarshal(alias.ObservedPatterns, &p); err != nil {
				return err
			}
			lu.ObservedPatterns = []Pattern{p}
		}
	}
	return nil
}
