package dream

import "time"

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
}

// DreamResult is the summary returned after a dream run.
type DreamResult struct {
	Status  string `json:"status"` // clean, dry-run, complete, error
	Batches int    `json:"batches"`
	Visions int    `json:"visions"`
}

// MotePair identifies two motes for link/contradiction analysis.
type MotePair struct {
	A          string
	B          string
	SharedTags []string
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

// ScanResult holds all dream pre-scan findings.
type ScanResult struct {
	LinkCandidates          []MotePair
	ContradictionCandidates []MotePair
	OverloadedTags          []TagOverload
	StaleMotes              []string
	ConstellationEvolution  []ConstellationEvolution
	CompressionCandidates   []string
	UncrystallizedIssues    []string
	StrataCrystallization   []StrataCrystallizationCandidate
	SignalCandidates        []SignalCandidate
}

// HasWork returns true if any scan category found candidates.
func (sr *ScanResult) HasWork() bool {
	return len(sr.LinkCandidates) > 0 ||
		len(sr.ContradictionCandidates) > 0 ||
		len(sr.OverloadedTags) > 0 ||
		len(sr.StaleMotes) > 0 ||
		len(sr.ConstellationEvolution) > 0 ||
		len(sr.CompressionCandidates) > 0 ||
		len(sr.UncrystallizedIssues) > 0 ||
		len(sr.StrataCrystallization) > 0 ||
		len(sr.SignalCandidates) > 0
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
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
	Batches   int    `json:"batches"`
	Visions   int    `json:"visions"`
	DurationS float64 `json:"duration_s"`
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
	Strength    int    `json:"strength"`
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
	TensionID   string `json:"tension_id"`
	Description string `json:"description"`
	MoteIDs     []string `json:"mote_ids"`
}

// VisionSummary is a compressed reference to a vision in the lucid log.
type VisionSummary struct {
	Type    string `json:"type"`
	MoteIDs []string `json:"mote_ids"`
	Batch   int    `json:"batch"`
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
