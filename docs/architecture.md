# Technical Architecture: The Nebula

## Design Decision: Language and Runtime

**Go as the sole implementation language.**

The nebula is a CLI tool invoked dozens of times per session — on every context load, every prime, every show, every link operation. Startup latency matters. Go compiles to a single native binary that starts in < 1ms, requires no runtime installation, and distributes as `cp mote ~/.local/bin/`. No Python version management, no pip, no venv, no "which python3."

Go's design strengths align precisely with this system's profile: file I/O, YAML/JSON parsing, arithmetic scoring, BFS graph traversal, subprocess management, and text formatting. The language's deliberate simplicity means faster implementation across 46 stories, clearer code for Claude to read and modify during sessions, and fewer compilation surprises than Rust's borrow checker would produce for a system with no complex concurrent state.

**Total external dependencies: two.**

| Dependency | What | Why |
|---|---|---|
| `github.com/spf13/cobra` | CLI framework | Subcommand dispatch, shell completion, help generation |
| `gopkg.in/yaml.v3` | YAML parser | Mote frontmatter and config.yaml parsing |

Everything else — JSON, file I/O, regex, HTTP, subprocess, time, text templates, testing — is Go stdlib.

**Runtime requirement: one.**

| Requirement | What | When needed |
|---|---|---|
| Claude CLI | Anthropic's `claude` binary with OAuth | Dream cycle only. All other operations work without it. |

A user who never runs `mote dream` needs nothing beyond the mote binary itself. Core operations (add, show, ls, pulse, link, context, prime, crystallize, constellation, strata, doctor, stats) and strata operations (add, query, update, rebuild, rm, ls, stats) work fully offline with zero external dependencies.

---

## System Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       CLI Entry Point                         │
│                     ~/.local/bin/mote                          │
│                                                                │
│  cobra dispatch: add, show, ls, pulse, link, unlink, context, │
│  prime, crystallize, constellation, strata, dream, doctor,     │
│  stats, index, promote, session-end, init, onboard, migrate,  │
│  update, tags                                                   │
└───────┬──────────────────────┬──────────────────────┬─────────┘
        │                      │                      │
┌───────▼────────┐    ┌────────▼────────┐    ┌────────▼────────┐
│  Core Engine   │    │  Strata Engine  │    │ Dream Orchestra │
│                │    │                 │    │                 │
│ Mote Manager   │    │ Corpus Manager  │    │ Pre-scanner     │
│ Index Manager  │    │ Chunker         │    │ Batch Builder   │
│ Score Engine   │    │ BM25 Index      │    │ Prompt Builder  │
│ Graph Traversal│    │ Query Log       │    │ Claude Invoker  │
│ Seed Selector  │    │                 │    │ Response Parser │
│ Signal Registry│    │                 │    │ Lucid Log       │
│ Config Manager │    │                 │    │ Vision Writer   │
└───────┬────────┘    └────────┬────────┘    └────────┬────────┘
        │                      │                      │
┌───────▼──────────────────────▼──────────────────────▼────────┐
│                        Storage Layer                          │
│                                                                │
│  .memory/nodes/*.md    (motes)                                │
│  .memory/index.jsonl   (edge index + tag stats)               │
│  .memory/config.yaml   (all configuration)                    │
│  .memory/constellations.jsonl                                 │
│  .memory/.access_batch.jsonl  (batched access updates)        │
│  .memory/dream/        (visions, lucid log, run log)          │
│  .memory/strata/       (corpora, BM25 indices, query log)     │
└──────────────────────────────────────────────────────────────┘
```

---

## Layer 1: Storage

All state lives in `.memory/` as plain files. No database, no daemon, no lock server. This is the foundation every other layer reads from and writes to.

### Mote File Format

Each mote is a markdown file with YAML frontmatter. The `---` boundaries are detected with simple string scanning. YAML frontmatter is parsed into a Go struct using `yaml.v3` with struct tags.

```go
type Mote struct {
    // Identity
    ID     string `yaml:"id"`
    Type   string `yaml:"type"`    // task|decision|lesson|context|question|constellation|anchor|explore
    Status string `yaml:"status"`  // active|in_progress|deprecated|archived|completed
    Title  string `yaml:"title"`
    Tags   []string `yaml:"tags"`
    Weight float64  `yaml:"weight"` // 0.0-1.0
    Origin string   `yaml:"origin"` // normal|failure|revert|hotfix|discovery

    // Retrieval metadata (auto-managed)
    CreatedAt    time.Time  `yaml:"created_at"`
    LastAccessed *time.Time `yaml:"last_accessed"`
    AccessCount  int        `yaml:"access_count"`

    // Planning links
    DependsOn []string `yaml:"depends_on"`
    Blocks    []string `yaml:"blocks"`

    // Memory links
    RelatesTo    []string `yaml:"relates_to"`
    BuildsOn     []string `yaml:"builds_on"`
    Contradicts  []string `yaml:"contradicts"`
    Supersedes   []string `yaml:"supersedes"`
    CausedBy     []string `yaml:"caused_by"`
    InformedBy   []string `yaml:"informed_by"`

    // External references (v0.3.0)
    ExternalRefs []ExternalRef `yaml:"external_refs,omitempty"`

    // Issue integration
    SourceIssue   string     `yaml:"source_issue,omitempty"`
    CrystallizedAt *time.Time `yaml:"crystallized_at,omitempty"`

    // Global promotion
    PromotedTo string `yaml:"promoted_to,omitempty"`

    // Deprecation tracking
    DeprecatedBy string `yaml:"deprecated_by,omitempty"`

    // Hierarchy (v0.2.0)
    Parent string `yaml:"parent,omitempty"`

    // Acceptance criteria (v0.2.0)
    Acceptance    []string `yaml:"acceptance,omitempty"`
    AcceptanceMet []bool   `yaml:"acceptance_met,omitempty"`

    // Effort sizing (v0.2.0)
    Size string `yaml:"size,omitempty"` // xs|s|m|l|xl

    // Strata integration (anchor motes only)
    StrataCorpus      string     `yaml:"strata_corpus,omitempty"`
    StrataQueryHint   string     `yaml:"strata_query_hint,omitempty"`
    StrataQueryCount  int        `yaml:"strata_query_count,omitempty"`
    StrataLastQueried *time.Time `yaml:"strata_last_queried,omitempty"`

    // Soft-delete tracking
    DeletedAt *time.Time `yaml:"deleted_at,omitempty"`

    // Non-YAML (populated after parse)
    Body     string `yaml:"-"` // markdown content below frontmatter
    FilePath string `yaml:"-"` // absolute path to .md file
}
```

### External References

External references link motes to external systems (GitHub issues, Jira tickets, etc.) via structured triples:

```go
type ExternalRef struct {
    Provider string `yaml:"provider" json:"provider"`
    ID       string `yaml:"id" json:"id"`
    URL      string `yaml:"url,omitempty" json:"url,omitempty"`
}
```

Ref IDs are included in BM25 search indexing, so searching for an issue number will surface the linked mote.

Parsing splits the file at `---` boundaries, unmarshals the YAML block into the struct, and stores everything below the second `---` as `Body`. Serialization reverses this: marshal the struct to YAML, wrap in `---` fences, append the body.

```go
func ParseMote(path string) (*Mote, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    parts := splitFrontmatter(string(data))
    if parts == nil {
        return nil, fmt.Errorf("no frontmatter in %s", path)
    }
    var m Mote
    if err := yaml.Unmarshal([]byte(parts.frontmatter), &m); err != nil {
        return nil, fmt.Errorf("bad frontmatter in %s: %w", path, err)
    }
    m.Body = parts.body
    m.FilePath = path
    return &m, nil
}
```

### Edge Index

`index.jsonl` stores one JSON line per directed edge plus a tag stats footer. Loaded entirely into memory at the start of each CLI invocation.

```go
type Edge struct {
    Source   string `json:"source"`
    Target   string `json:"target"`
    EdgeType string `json:"edge_type"`
}

type EdgeIndex struct {
    Edges    []Edge
    TagStats map[string]int
    MoteIDs  map[string]bool // fast existence check

    // Derived for fast lookup
    outgoing map[string][]Edge // source -> edges
    incoming map[string][]Edge // target -> edges
}

func (idx *EdgeIndex) Neighbors(moteID string, edgeTypes map[string]bool) []Edge {
    var result []Edge
    for _, e := range idx.outgoing[moteID] {
        if edgeTypes == nil || edgeTypes[e.EdgeType] {
            result = append(result, e)
        }
    }
    return result
}
```

For 500 motes with ~2000 edges, the in-memory footprint is under 500 KB. The adjacency maps are built on load, giving O(1) neighbor lookup.

### Atomicity

All file writes use write-to-temp-then-rename:

```go
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, perm); err != nil {
        return err
    }
    return os.Rename(tmp, path) // atomic on POSIX
}
```

Access count updates are batched in `.memory/.access_batch.jsonl` during the session and flushed to mote files at session end (`mote session-end`). This avoids rewriting mote files on every read operation.

### Directory Layout

```
.memory/
├── nodes/                          # mote files (one .md per mote)
│   ├── proj-L1a2b3c4.md
│   ├── proj-C4d5e6f7.md
│   └── ...
├── index.jsonl                     # edge index + tag stats
├── config.yaml                     # scoring, dream, strata, priming config
├── constellations.jsonl            # constellation cluster records
├── .access_batch.jsonl             # batched access updates (flushed at session end)
├── trash/                          # soft-deleted motes (restorable)
├── dream/
│   ├── visions.jsonl               # final reconciled visions (pending review)
│   ├── visions_draft.jsonl         # pre-reconciliation visions
│   ├── lucid.json                  # last lucid log
│   ├── log.jsonl                   # append-only dream run history
│   ├── scan_state.json             # content-hash cache for incremental prescanning
│   └── auto_applied.jsonl          # auto-applied dream visions log
└── strata/
    ├── query_log.jsonl             # append-only strata query log
    └── <corpus-name>/
        ├── manifest.json           # corpus metadata
        ├── chunks.jsonl            # chunked content with IDs and text
        └── bm25.json              # precomputed term frequencies + IDF values
```

---

## Layer 2: Core Engine

The core engine handles all hot-path and warm-path operations. Zero external dependencies. Must meet the < 2 second latency budget for `mote prime` and `mote context` on a 500-mote nebula (in practice, Go achieves < 100ms).

### Mote Manager

CRUD operations on mote files. Each operation reads/writes directly to `.memory/nodes/`.

```go
type MoteManager struct {
    root string // path to .memory/
}

func (mm *MoteManager) Create(moteType, title string, opts CreateOpts) (*Mote, error)
func (mm *MoteManager) Read(moteID string) (*Mote, error)
func (mm *MoteManager) Update(moteID string, fields map[string]interface{}) error
func (mm *MoteManager) List(filters ListFilters) ([]*Mote, error)
func (mm *MoteManager) Deprecate(moteID, supersededBy string) error
func (mm *MoteManager) ReadAll() ([]*Mote, error)
func (mm *MoteManager) FlushAccessBatch() error
```

**ID generation:** `<scope>-<typechar><base36-timestamp><random-suffix>`

```go
func GenerateID(scope, moteType string) string {
    typeChar := strings.ToUpper(moteType[:1])
    timestamp := base36Encode(time.Now().UnixMilli())
    suffix := base36Encode(rand.IntN(1296)) // 2 chars
    return fmt.Sprintf("%s-%s%s%s", scope, typeChar, timestamp, suffix)
}
```

### Index Manager

Builds and maintains the edge index from mote files. Loaded once per CLI invocation.

```go
type IndexManager struct {
    path  string
    index *EdgeIndex
}

func (im *IndexManager) Load() (*EdgeIndex, error)
func (im *IndexManager) Rebuild(motes []*Mote) error
func (im *IndexManager) AddEdge(edge Edge) error
func (im *IndexManager) RemoveEdge(source, target, edgeType string) error
```

The index is a cache, not a source of truth. All edges are derived from mote frontmatter link fields. `mote index rebuild` reconstructs it from scratch. Any corruption is self-healing.

### Score Engine

Implements the full scoring formula from Story 4.4. All parameters are read from config at initialization.

```go
type ScoreEngine struct {
    config   ScoringConfig
    tagStats map[string]int
}

type ScoringContext struct {
    MatchedTags          []string
    EdgeType             string
    ActiveContradictions int
}

func (se *ScoreEngine) Score(mote *Mote, ctx ScoringContext) float64 {
    // Base score
    base := mote.Weight

    // Edge bonus (spreading activation)
    edgeBonus := se.config.EdgeBonuses[ctx.EdgeType]

    // Status penalty
    statusPenalty := se.config.StatusPenalties[mote.Status]

    // Recency decay (forgetting curve)
    recencyFactor := se.recencyFactor(mote.LastAccessed)

    // Retrieval strength (spaced retrieval)
    retrievalBonus := math.Min(
        float64(mote.AccessCount)*se.config.RetrievalStrength.PerAccess,
        se.config.RetrievalStrength.MaxBonus,
    )

    // Salience boost (emotional tagging)
    salienceBonus := se.config.Salience[mote.Origin]
    if mote.Type == "explore" {
        salienceBonus += se.config.ExploreTypeBonus
    }

    // Tag specificity (cue overload prevention)
    tagBonus := se.tagSpecificity(ctx.MatchedTags)

    // Interference penalty (contradiction awareness)
    interference := float64(ctx.ActiveContradictions) * se.config.InterferencePenalty

    // Final score
    raw := base + edgeBonus + statusPenalty + retrievalBonus +
           salienceBonus + tagBonus + interference
    return raw * recencyFactor
}

func (se *ScoreEngine) recencyFactor(lastAccessed *time.Time) float64 {
    if lastAccessed == nil {
        return se.config.RecencyDecay.Tiers[len(se.config.RecencyDecay.Tiers)-1].Factor
    }
    days := int(time.Since(*lastAccessed).Hours() / 24)
    for _, tier := range se.config.RecencyDecay.Tiers {
        if tier.MaxDays == 0 || days < tier.MaxDays {
            return tier.Factor
        }
    }
    return se.config.RecencyDecay.Tiers[len(se.config.RecencyDecay.Tiers)-1].Factor
}

func (se *ScoreEngine) tagSpecificity(tags []string) float64 {
    if len(tags) == 0 {
        return 0.0
    }
    var total float64
    for _, tag := range tags {
        count := se.tagStats[tag]
        total += 1.0 / math.Log2(float64(count)+2)
    }
    return (total / float64(len(tags))) * se.config.TagSpecificityWeight
}
```

### Graph Traversal

BFS traversal from seed motes with hop-limited, edge-weighted scoring.

```go
type GraphTraverser struct {
    index        *EdgeIndex
    scorer       *ScoreEngine
    maxHops      int
    maxResults   int
    minThreshold float64
}

type ScoredMote struct {
    Mote  *Mote
    Score float64
}

func (gt *GraphTraverser) Traverse(seeds []string, reader func(string) *Mote) []ScoredMote {
    visited := make(map[string]*ScoredMote)
    type frontier struct {
        id       string
        hop      int
        edgeType string
    }
    queue := []frontier{}

    // Score seeds directly
    for _, seedID := range seeds {
        mote := reader(seedID)
        if mote == nil {
            continue
        }
        ctx := ScoringContext{EdgeType: "seed"}
        score := gt.scorer.Score(mote, ctx)
        visited[seedID] = &ScoredMote{Mote: mote, Score: score}
        queue = append(queue, frontier{id: seedID, hop: 0})
    }

    // BFS expansion
    for len(queue) > 0 {
        current := queue[0]
        queue = queue[1:]

        if current.hop >= gt.maxHops {
            continue
        }

        for _, edge := range gt.index.Neighbors(current.id, nil) {
            if _, seen := visited[edge.Target]; seen {
                continue
            }
            targetMote := reader(edge.Target)
            if targetMote == nil {
                continue
            }
            ctx := ScoringContext{
                EdgeType:             edge.EdgeType,
                ActiveContradictions: gt.countContradictions(edge.Target),
            }
            score := gt.scorer.Score(targetMote, ctx)
            if score >= gt.minThreshold {
                visited[edge.Target] = &ScoredMote{Mote: targetMote, Score: score}
                queue = append(queue, frontier{
                    id: edge.Target, hop: current.hop + 1, edgeType: edge.EdgeType,
                })
            }
        }
    }

    // Rank and cap
    result := make([]ScoredMote, 0, len(visited))
    for _, sm := range visited {
        result = append(result, *sm)
    }
    sort.Slice(result, func(i, j int) bool {
        return result[i].Score > result[j].Score
    })
    if len(result) > gt.maxResults {
        result = result[:gt.maxResults]
    }
    return result
}
```

### Seed Selector

Determines initial seed motes for traversal from explicit topics and ambient signals. Reads the extensible signal registry from config.

```go
type SeedSelector struct {
    signals  []SignalConfig
    motes    []*Mote
    tagStats map[string]int
}

type AmbientContext struct {
    GitBranch   string
    RecentFiles []string
    PromptText  string
}

func (ss *SeedSelector) SelectSeeds(topic string, ambient *AmbientContext) []string {
    candidates := make(map[string]float64) // moteID -> signal strength

    // Explicit topic: tag match
    if topic != "" {
        keywords := extractKeywords(topic)
        for _, mote := range ss.motes {
            overlap := tagOverlap(keywords, mote.Tags)
            if overlap > 0 {
                candidates[mote.ID] += float64(overlap)
            }
        }
    }

    // Ambient signals
    if ambient != nil {
        for _, signal := range ss.signals {
            switch signal.Type {
            case "built_in":
                ss.applyBuiltinSignal(signal.Name, ambient, candidates)
            case "co_access":
                ss.applyCoAccessSignal(signal, candidates)
            // extensible for dream-discovered signal types
            }
        }
    }

    return topN(candidates, 10)
}

func (ss *SeedSelector) applyBuiltinSignal(name string, ambient *AmbientContext, candidates map[string]float64) {
    switch name {
    case "git_branch":
        keywords := extractKeywords(ambient.GitBranch)
        ss.matchKeywordsToTags(keywords, candidates, 0.5)
    case "recent_files":
        keywords := extractKeywordsFromPaths(ambient.RecentFiles)
        ss.matchKeywordsToTags(keywords, candidates, 0.3)
    case "prompt_keywords":
        keywords := extractKeywords(ambient.PromptText)
        ss.matchKeywordsToTags(keywords, candidates, 0.8)
    }
}
```

### Config Manager

Reads `.memory/config.yaml` using `yaml.v3` with typed structs and sensible defaults.

```go
type Config struct {
    Scoring ScoringConfig `yaml:"scoring"`
    Priming PrimingConfig `yaml:"priming"`
    Dream   DreamConfig   `yaml:"dream"`
    Strata  StrataConfig  `yaml:"strata"`
}

func LoadConfig(root string) (*Config, error) {
    data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
    if err != nil {
        return DefaultConfig(), nil // missing config uses defaults
    }
    cfg := DefaultConfig() // start with defaults
    if err := yaml.Unmarshal(data, cfg); err != nil {
        return nil, fmt.Errorf("bad config: %w", err)
    }
    return cfg, nil
}
```

### Hierarchical Planning

Parent/child relationships enable structured task decomposition. A child mote references its parent via the `parent` field.

```go
func (mm *MoteManager) Children(parentID string) ([]*Mote, error)
```

`mote plan <parent-id> --child "Step 1" --child "Step 2" --sequential` creates child motes and optionally chains them with `depends_on` links. Children inherit the parent's tags.

**Acceptance criteria** are stored as parallel arrays: `Acceptance []string` holds the criteria text and `AcceptanceMet []bool` tracks which are satisfied. `mote check <id> <index>` sets `AcceptanceMet[index-1] = true`.

`mote progress <parent-id>` aggregates child completion status and acceptance criteria into a completion percentage.

### Import/Export

JSONL import/export enables backup, migration, and cross-project transfer of motes.

```go
type ExportMote struct {
    ID            string            `json:"id"`
    Type          string            `json:"type"`
    Status        string            `json:"status"`
    Title         string            `json:"title"`
    Tags          []string          `json:"tags"`
    Weight        float64           `json:"weight"`
    Origin        string            `json:"origin"`
    CreatedAt     time.Time         `json:"created_at"`
    Body          string            `json:"body"`
    DependsOn     []string          `json:"depends_on,omitempty"`
    Parent        string            `json:"parent,omitempty"`
    Acceptance    []string          `json:"acceptance,omitempty"`
    AcceptanceMet []bool            `json:"acceptance_met,omitempty"`
    Size          string            `json:"size,omitempty"`
    ExternalRefs  []ExternalRef     `json:"external_refs,omitempty"`
    // ... all link fields with omitempty
}
```

On import, deduplication uses SHA256 of `type + title + body` to skip motes that already exist. IDs are regenerated for the target project scope.

---

## Layer 3: Strata Engine

The strata layer provides reference knowledge search using BM25 — a term frequency-based ranking algorithm that requires no ML models, no network, and no external dependencies. It compiles into the Go binary.

### Architecture Decision: BM25 over Embeddings

Embedding-based semantic search requires infrastructure: either a local model runtime (Ollama, llama.cpp) with downloaded model files, or a remote API with keys and network access. Both create adoption friction that conflicts with the system's "single binary, zero config" philosophy.

BM25 (Best Match 25) ranks documents by term frequency, inverse document frequency, and document length normalization. It's the algorithm behind Elasticsearch, Lucene, and most production search engines. For the kind of queries the nebula generates — where mote tags, user queries, and strata content share vocabulary because they describe the same codebase — BM25 performs well. When Claude is working on OAuth and an anchor mote triggers a strata query for "token refresh authentication headers," keyword overlap with chunks about OAuth token refresh is strong enough to surface the right material.

Where BM25 falls short (synonym understanding, semantic paraphrasing), the dream cycle compensates. During cold-path processing, Claude evaluates strata query patterns with full semantic understanding, identifying chunks that are frequently relevant but missed by keyword matching, and proposing crystallization of those insights into proper motes. This is the architectural sweet spot: fast deterministic retrieval on the hot path, deep semantic evaluation on the cold path where latency doesn't matter.

### BM25 Implementation

~150 lines of Go. Precomputed at ingestion time, stored as JSON, loaded into memory on query.

```go
type BM25Index struct {
    DocCount   int                `json:"doc_count"`
    AvgDocLen  float64            `json:"avg_doc_len"`
    IDF        map[string]float64 `json:"idf"`

    Docs []BM25Doc `json:"docs"`
}

type BM25Doc struct {
    ChunkID  string             `json:"chunk_id"`
    TermFreq map[string]float64 `json:"tf"`
    DocLen   int                `json:"doc_len"`
}

const (
    bm25K1 = 1.2  // term frequency saturation
    bm25B  = 0.75 // document length normalization
)

func (idx *BM25Index) Search(query string, topK int) []ChunkResult {
    queryTerms := tokenize(query)
    scores := make([]float64, len(idx.Docs))

    for _, term := range queryTerms {
        idf, exists := idx.IDF[term]
        if !exists {
            continue
        }
        for i, doc := range idx.Docs {
            tf := doc.TermFreq[term]
            if tf == 0 {
                continue
            }
            numerator := tf * (bm25K1 + 1)
            denominator := tf + bm25K1*(1-bm25B+bm25B*float64(doc.DocLen)/idx.AvgDocLen)
            scores[i] += idf * (numerator / denominator)
        }
    }

    return topKResults(scores, idx.Docs, topK)
}

func BuildBM25Index(chunks []Chunk) *BM25Index {
    idx := &BM25Index{DocCount: len(chunks)}

    totalLen := 0
    docFreq := make(map[string]int) // term -> doc count

    for _, chunk := range chunks {
        tokens := tokenize(chunk.Text)
        totalLen += len(tokens)

        tf := make(map[string]float64)
        seen := make(map[string]bool)
        for _, token := range tokens {
            tf[token]++
            if !seen[token] {
                docFreq[token]++
                seen[token] = true
            }
        }
        for term := range tf {
            tf[term] /= float64(len(tokens))
        }

        idx.Docs = append(idx.Docs, BM25Doc{
            ChunkID:  chunk.ID,
            TermFreq: tf,
            DocLen:   len(tokens),
        })
    }
    idx.AvgDocLen = float64(totalLen) / float64(len(chunks))

    idx.IDF = make(map[string]float64)
    for term, df := range docFreq {
        idx.IDF[term] = math.Log((float64(idx.DocCount)-float64(df)+0.5)/(float64(df)+0.5) + 1)
    }

    return idx
}
```

### Tokenizer

Lowercases, splits on non-alphanumeric boundaries, removes stop words. No stemming — the shared vocabulary between queries and content handles morphological variants well enough for this system's domain-specific queries.

```go
var stopWords = map[string]bool{
    "the": true, "a": true, "an": true, "is": true, "are": true,
    "was": true, "were": true, "be": true, "been": true, "being": true,
    "have": true, "has": true, "had": true, "do": true, "does": true,
    "did": true, "will": true, "would": true, "could": true, "should": true,
    "in": true, "on": true, "at": true, "to": true, "for": true,
    "of": true, "with": true, "by": true, "from": true, "as": true,
    "and": true, "but": true, "or": true, "not": true, "if": true,
    "this": true, "that": true, "these": true, "those": true,
    "it": true, "its": true, "he": true, "she": true, "they": true,
}

var tokenRegex = regexp.MustCompile(`[a-zA-Z0-9_]+`)

func tokenize(text string) []string {
    words := tokenRegex.FindAllString(strings.ToLower(text), -1)
    result := make([]string, 0, len(words))
    for _, w := range words {
        if len(w) < 2 || stopWords[w] {
            continue
        }
        result = append(result, w)
    }
    return result
}
```

### Chunker

Splits reference material into searchable chunks. Three strategies, all implemented in Go with no external dependencies.

```go
type Chunker struct {
    strategy  string // heading-aware | function-level | sliding-window
    maxTokens int
    overlap   int
}

func (c *Chunker) Chunk(content string, sourcePath string) []Chunk
```

Heading-aware chunking splits markdown/text on `#` headers, keeping each section as a chunk (splitting further if a section exceeds `maxTokens`). Function-level chunking uses regex patterns to identify function/class boundaries in code. Both fall back to sliding window for content that doesn't match their patterns. Token counting uses a word count × 1.3 approximation.

### Strata Manager

Coordinates corpus operations: ingest, query, update, remove. All operations are local, deterministic, and require no network.

```go
type StrataManager struct {
    root     string
    config   StrataConfig
    chunker  *Chunker
    queryLog string
}

func (sm *StrataManager) AddCorpus(name string, paths []string, createAnchor bool) error
func (sm *StrataManager) Query(topic string, corpus string, topK int) ([]ChunkResult, error)
func (sm *StrataManager) UpdateCorpus(name string) error
func (sm *StrataManager) RebuildCorpus(name string) error
func (sm *StrataManager) RemoveCorpus(name string) error
func (sm *StrataManager) ListCorpora() ([]CorpusInfo, error)
func (sm *StrataManager) Stats() (*StrataStats, error)
```

Every strata query is logged to `query_log.jsonl` for dream cycle analysis:

```go
type QueryLogEntry struct {
    Timestamp    time.Time `json:"timestamp"`
    Query        string    `json:"query"`
    Corpus       string    `json:"corpus"`
    ResultsCount int       `json:"results_count"`
    TopChunkID   string    `json:"top_chunk_id"`
    TopScore     float64   `json:"top_score"`
}
```

---

## Layer 4: Dream Orchestrator

The dream cycle coordinates deterministic pre-scan, Claude CLI invocations for batch reasoning and reconciliation, lucid log accumulation, and vision output. It runs as a single-shot process outside any interactive session.

### Claude CLI as the LLM Interface

The dream orchestrator invokes Claude CLI via `os/exec`. It never handles OAuth tokens, API keys, or HTTP connections. The CLI manages authentication, model selection, and retry logic. The orchestrator's responsibility is prompt construction, response parsing, and state management.

```go
type ClaudeInvoker struct {
    batchModel string // e.g. "claude-sonnet-4-20250514"
    reconModel string // e.g. "claude-opus-4-20250514"
    timeout    time.Duration
}

func (ci *ClaudeInvoker) Invoke(prompt string, model string) (string, error) {
    modelName := ci.batchModel
    if model == "opus" {
        modelName = ci.reconModel
    }

    ctx, cancel := context.WithTimeout(context.Background(), ci.timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "claude",
        "--model", modelName,
        "--output-format", "text",
        "--print",
        "--max-turns", "1",
    )
    cmd.Stdin = strings.NewReader(prompt)

    output, err := cmd.Output()
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return "", fmt.Errorf("claude timed out after %v", ci.timeout)
        }
        return "", fmt.Errorf("claude invocation failed: %w", err)
    }
    return string(output), nil
}
```

### Dream Orchestrator

Coordinates the four-stage pipeline. Batch failures are logged and skipped, not fatal.

```go
type DreamOrchestrator struct {
    root     string
    config   DreamConfig
    scanner  *PreScanner
    batcher  *BatchConstructor
    prompts  *PromptBuilder
    invoker  *ClaudeInvoker
    parser   *ResponseParser
    lucidLog *LucidLog
    visions  *VisionWriter
}

func (do *DreamOrchestrator) Run(dryRun bool) (*DreamResult, error) {
    // Stage 1: Pre-scan (deterministic, no LLM)
    candidates, err := do.scanner.Scan()
    if err != nil {
        return nil, fmt.Errorf("pre-scan failed: %w", err)
    }
    if !candidates.HasWork() {
        return &DreamResult{Status: "clean", Visions: 0}, nil
    }

    batches := do.batcher.Build(candidates)
    do.lucidLog.Initialize()

    if dryRun {
        return &DreamResult{Status: "dry-run", Batches: len(batches)}, nil
    }

    // Stage 2: Batch reasoning (Claude Sonnet)
    for i, batch := range batches {
        prompt := do.prompts.BuildBatchPrompt(batch, do.lucidLog)
        response, err := do.invoker.Invoke(prompt, "sonnet")
        if err != nil {
            do.lucidLog.RecordBatchFailure(i, err.Error())
            continue
        }
        batchVisions, logUpdates, err := do.parser.ParseBatchResponse(response)
        if err != nil {
            do.lucidLog.RecordBatchFailure(i, fmt.Errorf("parse: %w", err).Error())
            continue
        }
        do.visions.WriteDrafts(batchVisions)
        do.lucidLog.Update(logUpdates)
    }

    // Stage 3: Reconciliation (Claude Opus)
    var finalVisions []Vision
    if do.config.Reconciliation.Enabled {
        reconPrompt := do.prompts.BuildReconciliationPrompt(do.lucidLog)
        reconResponse, err := do.invoker.Invoke(reconPrompt, "opus")
        if err == nil {
            finalVisions, _ = do.parser.ParseReconciliationResponse(reconResponse)
        }
        if finalVisions == nil {
            finalVisions = do.visions.ReadDrafts() // fallback to unreconciled
        }
    } else {
        finalVisions = do.visions.ReadDrafts()
    }

    do.visions.WriteFinal(finalVisions)

    // Stage 4: Log
    do.writeRunLog(len(batches), len(finalVisions))
    return &DreamResult{Status: "complete", Batches: len(batches), Visions: len(finalVisions)}, nil
}
```

### Pre-Scanner

Entirely deterministic. Reads all motes and the index, identifies candidates for each dream task. Parallelizes file reads with goroutines.

```go
type PreScanner struct {
    moteManager   *MoteManager
    indexManager   *IndexManager
    strataManager  *StrataManager
    config        DreamConfig
}

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
}

func (ps *PreScanner) Scan() (*ScanResult, error) {
    motes, err := ps.moteManager.ReadAllParallel()
    if err != nil {
        return nil, err
    }
    index, err := ps.indexManager.Load()
    if err != nil {
        return nil, err
    }
    return &ScanResult{
        LinkCandidates:          ps.findLinkCandidates(motes, index),
        ContradictionCandidates: ps.findContradictionCandidates(motes),
        OverloadedTags:          ps.findOverloadedTags(index.TagStats),
        StaleMotes:              ps.findStaleMotes(motes),
        ConstellationEvolution:  ps.findConstellationCandidates(motes),
        CompressionCandidates:   ps.findCompressionCandidates(motes),
        UncrystallizedIssues:    ps.findUncrystallized(motes),
        StrataCrystallization:   ps.findStrataCandidates(),
        SignalCandidates:        ps.findSignalPatterns(motes),
        MergeCandidates:         ps.findMergeCandidates(motes),
        SummarizationCandidates: ps.findSummarizationCandidates(motes, index),
    }, nil
}
```

#### Scan Cache

The scan cache enables incremental prescanning by tracking content hashes of motes. Unchanged motes are skipped between dream runs.

```go
type ScanCache struct {
    Hashes map[string]string `json:"hashes"` // mote ID -> content hash
}

func ComputeMoteHash(m *Mote) string {
    data, _ := SerializeMote(m)
    h := sha256.Sum256(data)
    return fmt.Sprintf("%x", h[:16])
}

func FilterChanged(motes []*Mote, cache *ScanCache) []*Mote
```

Stored at `.memory/dream/scan_state.json`. `FilterChanged` returns only motes whose hash differs from the cached value, updates the cache with current hashes, and prunes entries for deleted motes.

#### Summarization Candidates

The prescanner detects clusters of completed motes suitable for summarization:

- **Detection:** Completed motes grouped by tag pairs with 2+ overlap, threshold 5+ members per cluster
- **Exclusion:** Motes already linked via `builds_on` from a context mote (already summarized)

```go
type SummarizationCluster struct {
    SharedTags []string `json:"shared_tags"`
    MoteIDs    []string `json:"mote_ids"`
}
```

The `summarize` vision type creates a context mote with `builds_on` links to all source motes and archives the sources.

Parallel mote reading:

```go
func (mm *MoteManager) ReadAllParallel() ([]*Mote, error) {
    entries, err := os.ReadDir(filepath.Join(mm.root, "nodes"))
    if err != nil {
        return nil, err
    }

    type result struct {
        mote *Mote
        err  error
    }

    results := make([]result, len(entries))
    var wg sync.WaitGroup

    for i, entry := range entries {
        if !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        wg.Add(1)
        go func(idx int, name string) {
            defer wg.Done()
            path := filepath.Join(mm.root, "nodes", name)
            m, err := ParseMote(path)
            results[idx] = result{mote: m, err: err}
        }(i, entry.Name())
    }
    wg.Wait()

    var motes []*Mote
    for _, r := range results {
        if r.err != nil {
            fmt.Fprintf(os.Stderr, "⚠ %v\n", r.err)
            continue
        }
        if r.mote != nil {
            motes = append(motes, r.mote)
        }
    }
    return motes, nil
}
```

### Batch Constructor

Implements hybrid batching: Phase A (tag-clustered, 60%) then Phase B (interleaved, 40%).

```go
type BatchConstructor struct {
    config     BatchingConfig
    moteReader func(string) *Mote
}

type Batch struct {
    Phase          string   // "clustered" | "interleaved"
    PrimaryCluster string   // tag cluster name (clustered only)
    MoteIDs        []string
    Motes          []*Mote
    Tasks          []string // applicable dream tasks
}

func (bc *BatchConstructor) Build(candidates *ScanResult) []Batch {
    // Phase A: Group by primary tag, build clustered batches
    // Phase B: Mix remaining motes across clusters for cross-domain discovery
    // Apply configured clustered_fraction
    // Order: Phase A first, then Phase B
    // Within each phase: batches ordered by total weight descending
}
```

### Prompt Builder

Uses Go's `text/template` for maintainable prompt construction.

```go
type PromptBuilder struct {
    batchTmpl *template.Template
    reconTmpl *template.Template
    reader    func(string) *Mote
}

func NewPromptBuilder(reader func(string) *Mote) *PromptBuilder {
    funcMap := template.FuncMap{
        "joinTags":   func(tags []string) string { return strings.Join(tags, ", ") },
        "formatTime": func(t *time.Time) string { /* ... */ },
    }
    pb := &PromptBuilder{reader: reader}
    pb.batchTmpl = template.Must(template.New("batch").Funcs(funcMap).Parse(batchPrompt))
    pb.reconTmpl = template.Must(template.New("recon").Funcs(funcMap).Parse(reconPrompt))
    return pb
}

var batchPrompt = `You are performing dream cycle maintenance on a mote nebula.

## Lucid Log
{{.LucidLog}}

## Batch ({{.Phase}}: {{.Cluster}})
{{range .Motes}}
### {{.ID}} — {{.Title}}
Type: {{.Type}} | Origin: {{.Origin}} | Weight: {{.Weight}} | Tags: {{joinTags .Tags}}
Last accessed: {{formatTime .LastAccessed}} | Access count: {{.AccessCount}}

{{.Body}}
---
{{end}}

## Tasks
1. Link inference — shared concepts that should be connected
2. Contradiction detection — conflicting assertions
3. Tag refinement — overly broad tags to split
4. Staleness evaluation — outdated content
5. Compression — verbose motes to distill
6. Signal discovery — co-access or sequential patterns for priming

Respond with JSON: {"visions": [...], "lucid_log_updates": {...}}
`
```

### Lucid Log

Accumulates findings across batches. Each batch reads the full log, writes updates. Pruned if over token budget.

```go
type LucidLog struct {
    ObservedPatterns []Pattern       `json:"observed_patterns"`
    Tensions         []Tension       `json:"tensions"`
    VisionsSummary   []VisionSummary `json:"visions_summary"`
    Interrupts       []Interrupt     `json:"interrupts"`
    StrataHealth     []StrataFlag    `json:"strata_health"`
    Metadata         LucidMetadata   `json:"metadata"`
    maxTokens        int
}

func (ll *LucidLog) Update(updates LucidLogUpdates) {
    for _, p := range updates.ObservedPatterns {
        if existing := ll.findPattern(p.PatternID); existing != nil {
            existing.Merge(p)
        } else {
            ll.ObservedPatterns = append(ll.ObservedPatterns, p)
        }
    }
    // ... similar for tensions, interrupts
    ll.Metadata.BatchCount++
    ll.pruneIfOverLimit()
}

func (ll *LucidLog) Serialize() string {
    data, _ := json.Marshal(ll)
    return string(data)
}
```

### Response Parser

Extracts JSON from Claude's response, handles preamble text gracefully.

```go
func (rp *ResponseParser) ParseBatchResponse(raw string) ([]Vision, LucidLogUpdates, error) {
    jsonStr := extractJSON(raw) // finds first balanced {...} block
    if jsonStr == "" {
        return nil, LucidLogUpdates{}, fmt.Errorf("no JSON in response")
    }
    var resp struct {
        Visions         []Vision        `json:"visions"`
        LucidLogUpdates LucidLogUpdates `json:"lucid_log_updates"`
    }
    if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
        return nil, LucidLogUpdates{}, fmt.Errorf("invalid JSON: %w", err)
    }
    valid := filterValidVisions(resp.Visions)
    return valid, resp.LucidLogUpdates, nil
}
```

### Vision Review

Interactive terminal review via `mote dream --review`.

```go
func (vr *VisionReviewer) Review() (*ReviewResult, error) {
    visions, err := vr.loadVisions()
    if err != nil {
        return nil, err
    }
    if len(visions) == 0 {
        fmt.Println("No pending visions.")
        return &ReviewResult{}, nil
    }

    result := &ReviewResult{}
    var deferred []Vision

    for i, v := range visions {
        fmt.Printf("\n═══ Vision %d/%d ═══\n", i+1, len(visions))
        vr.display(v)
        fmt.Print("\n[a]ccept / [e]dit / [r]eject / [d]efer: ")

        switch readChoice() {
        case "a":
            if err := vr.apply(v); err != nil {
                fmt.Fprintf(os.Stderr, "⚠ apply failed: %v\n", err)
                deferred = append(deferred, v)
            } else {
                result.Accepted++
            }
        case "e":
            edited := vr.edit(v)
            vr.apply(edited)
            result.Accepted++
        case "r":
            result.Rejected++
        case "d":
            deferred = append(deferred, v)
            result.Deferred++
        }
    }

    vr.writeRemaining(deferred)
    return result, nil
}
```

---

## Layer 5: CLI Dispatcher

Thin cobra-based dispatcher routing commands to the appropriate layer.

```go
func main() {
    root := &cobra.Command{
        Use:   "mote",
        Short: "The Nebula: AI Context and Memory System",
    }

    // Core
    root.AddCommand(addCmd(), showCmd(), lsCmd(), pulseCmd(),
        linkCmd(), unlinkCmd(), contextCmd(), primeCmd(),
        crystallizeCmd(), constellationCmd(), doctorCmd(),
        statsCmd(), indexCmd(), promoteCmd(), sessionEndCmd(),
        updateCmd(), tagsCmd(), searchCmd(), quickCmd(),
        feedbackCmd(), deleteCmd(), trashCmd())

    // Hierarchical planning
    root.AddCommand(planCmd(), progressCmd(), checkCmd())

    // Import/Export
    root.AddCommand(exportCmd(), importCmd())

    // Strata (subcommands: add, query, ls, update, rebuild, rm, stats)
    root.AddCommand(strataCmd())

    // Dream
    root.AddCommand(dreamCmd())

    root.Execute()
}
```

Each command lives in its own file for clarity. `findMemoryRoot()` walks up from cwd looking for `.memory/`.

---

## Performance Budget

| Operation | Target | Why achievable |
|---|---|---|
| `mote prime` | < 100ms | Binary starts in < 1ms. Index load ~2ms. Seed + BFS ~5ms. Score ~3ms. BM25 strata augment ~10ms. |
| `mote context` | < 100ms | Same as prime with explicit topic seeds. |
| `mote show` | < 10ms | Single file read + YAML unmarshal. |
| `mote ls` | < 50ms | Parallel frontmatter reads via goroutines. Filter + format. |
| `mote strata query` | < 50ms | Load BM25 index ~5ms. Tokenize ~0.1ms. Score 10k chunks ~2ms. |
| `mote index rebuild` | < 1s / 500 motes | Parallel reads, single-pass edge extraction. |
| `mote dream` | < 10 min / 500 motes | Pre-scan ~200ms. Claude batches ~30s each. Reconciliation ~60s. |

No cross-invocation cache. Every call reads from disk. OS page cache handles repeat reads. Within a single invocation, motes are read once and held in a map.

---

## Error Handling

Go's explicit `if err != nil` maps naturally to the "warn and continue" philosophy. Malformed motes produce warnings, not crashes. Failed dream batches are logged and skipped.

```go
type Warnings struct {
    items []string
}

func (w *Warnings) Warn(format string, args ...interface{}) {
    msg := fmt.Sprintf(format, args...)
    w.items = append(w.items, msg)
    fmt.Fprintf(os.Stderr, "⚠ %s\n", msg)
}
```

---

## Testing Strategy

| Tier | What | How |
|---|---|---|
| **Unit** | Score engine, BM25, traversal, tokenizer, batch constructor | `go test` with in-memory motes, no filesystem |
| **Integration** | Full CLI commands against temp `.memory/` | `go test` with `t.TempDir()`, exec binary |
| **Dream** | Prompt quality, response parsing, lucid log | Record/replay with captured Claude response fixtures |

```go
func TestScoreEngine_RecencyDecay(t *testing.T) {
    engine := NewScoreEngine(DefaultScoringConfig(), map[string]int{"oauth": 3})
    recent := time.Now().Add(-24 * time.Hour)
    stale := time.Now().Add(-120 * 24 * time.Hour)

    scoreRecent := engine.Score(
        &Mote{Weight: 0.8, LastAccessed: &recent, Type: "lesson"},
        ScoringContext{EdgeType: "seed"},
    )
    scoreStale := engine.Score(
        &Mote{Weight: 0.8, LastAccessed: &stale, Type: "lesson"},
        ScoringContext{EdgeType: "seed"},
    )
    if scoreRecent <= scoreStale {
        t.Errorf("recent (%f) should outscore stale (%f)", scoreRecent, scoreStale)
    }
}

func BenchmarkBM25Search_10kChunks(b *testing.B) {
    index := buildTestBM25Index(10000)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        index.Search("oauth token refresh authentication", 5)
    }
}
```

---

## Build and Distribution

```bash
# Build
go build -o mote ./cmd/mote

# Install
cp mote ~/.local/bin/

# Cross-compile
GOOS=darwin GOARCH=arm64 go build -o mote-darwin-arm64 ./cmd/mote
GOOS=linux GOARCH=amd64 go build -o mote-linux-amd64 ./cmd/mote

# Slim build
go build -ldflags="-s -w" -o mote ./cmd/mote  # ~5-8 MB
```

---

## Security Considerations

**OAuth for dream cycle:** Claude CLI manages tokens. The dream orchestrator never handles credentials.

**No network for core + strata:** BM25 is entirely local. `mote prime`, `mote context`, `mote strata query`, and all CRUD operations work fully offline. Only `mote dream` requires network access.

**File permissions:** `mote doctor` flags `.memory/` directories with permissions more open than 700.

---

## Project Structure

```
mote/
├── cmd/mote/
│   ├── main.go                  # cobra root + dispatch
│   ├── helpers.go               # shared CLI helpers
│   ├── cmd_add.go
│   ├── cmd_show.go
│   ├── cmd_ls.go
│   ├── cmd_pulse.go             # alias: ls --status=active --type=task
│   ├── cmd_link.go
│   ├── cmd_context.go
│   ├── cmd_prime.go
│   ├── cmd_crystallize.go
│   ├── cmd_constellation.go
│   ├── cmd_doctor.go
│   ├── cmd_stats.go
│   ├── cmd_tags.go              # tag audit and management
│   ├── cmd_index.go
│   ├── cmd_promote.go
│   ├── cmd_session_end.go
│   ├── cmd_update.go            # update mote fields
│   ├── cmd_strata.go            # strata subcommands
│   ├── cmd_dream.go             # dream + dream --review
│   ├── cmd_init.go              # initialize .memory/
│   ├── cmd_onboard.go           # detect and migrate from beads/MEMORY.md
│   ├── cmd_migrate.go           # convert MEMORY.md to motes
│   ├── cmd_plan.go              # hierarchical task decomposition
│   ├── cmd_progress.go          # parent task completion tracking
│   ├── cmd_check.go             # acceptance criteria check-off
│   ├── cmd_export.go            # JSONL export with filters
│   ├── cmd_import.go            # JSONL import with content-hash dedup
│   ├── cmd_delete.go            # soft-delete to trash
│   ├── cmd_trash.go             # trash list/restore/purge
│   ├── cmd_feedback.go          # mote feedback (useful/irrelevant)
│   ├── cmd_quick.go             # quick capture without editor
│   └── cmd_search.go            # full-text BM25 search
├── internal/
│   ├── core/
│   │   ├── mote.go              # Mote struct, parse, serialize
│   │   ├── mote_manager.go      # CRUD
│   │   ├── index.go             # EdgeIndex, adjacency maps
│   │   ├── score.go             # ScoreEngine (full formula)
│   │   ├── traversal.go         # GraphTraverser (BFS)
│   │   ├── seed.go              # SeedSelector + ambient signals
│   │   ├── config.go            # Config types + loader
│   │   ├── id.go                # ID generation
│   │   ├── atomic.go            # Atomic file write (temp + rename)
│   │   └── link_types.go        # Link type definitions
│   ├── strata/
│   │   ├── manager.go           # StrataManager
│   │   ├── chunker.go           # Chunking strategies
│   │   ├── bm25.go              # BM25 index + search
│   │   └── tokenizer.go         # Tokenize + stop words
│   ├── dream/
│   │   ├── orchestrator.go      # Pipeline coordinator
│   │   ├── prescanner.go        # Deterministic pre-scan
│   │   ├── batch.go             # Hybrid batch construction
│   │   ├── prompt.go            # text/template prompts
│   │   ├── invoker.go           # Claude CLI subprocess
│   │   ├── parser.go            # Response parsing
│   │   ├── lucidlog.go          # Lucid log accumulation
│   │   ├── vision.go            # Vision types + writer
│   │   ├── types.go             # Dream cycle type definitions
│   │   └── scan_cache.go        # Content-hash cache for incremental prescanning
│   └── format/
│       └── output.go            # Terminal formatting
├── testdata/dream/              # Recorded Claude response fixtures
├── go.mod
├── go.sum
└── Makefile
```

`internal/` ensures these packages are not importable externally. This is a CLI tool, not a library.

---

## Migration

Migration from other systems (beads, MEMORY.md) is handled by `mote onboard` (auto-detects sources) and `mote migrate` (explicit MEMORY.md conversion). See `docs/onboarding.md` for details.
