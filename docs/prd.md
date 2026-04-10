# PRD: Motes — The Nebula: Unified AI Context and Memory System

## Product Overview

**Motes** is an AI-native context and memory system where knowledge and planning are stored as atomic units ("motes") linked in two dimensions: dependency links (for planning/execution ordering) and semantic links (for thematic memory connections). Unlike linear MEMORY.md files loaded wholesale into context, motes load selectively via graph traversal — surfacing the most relevant knowledge for the current task and scaling the context budget to match task complexity rather than applying a fixed cap.

Planning and memory are unified: when a task completes, it crystallizes into a permanent mote — a deliberate consolidation process modeled on how the brain replays and strengthens short-term experiences into long-term memory during sleep. Past decisions, lessons, and plans remain queryable and shape future work. The graph is Obsidian-compatible for visual exploration.

The system operates in three distinct processing modes — mirroring waking cognition, session transitions, and sleep — to separate time-critical retrieval from deep nebula maintenance. Deterministic scoring handles the hot path, while periodic LLM-driven "dream cycles" (Epic 12) perform the judgment-heavy work: discovering latent connections, detecting semantic contradictions, evolving constellations, and compressing stale knowledge. Claude Opus handles reconciliation and synthesis; Claude Sonnet handles focused batch reasoning.

A hybrid strata integration (Epic 13) extends the system with reference knowledge stores for stable, voluminous material that shouldn't pollute the graph. Anchor motes bridge the two systems — the graph controls when reference material is consulted, and the dream cycle crystallizes frequently needed reference knowledge into proper motes.

**CLI tool**: `mote`
**Storage**: Markdown files with YAML frontmatter, flat `nodes/` directory
**Scope**: Project-local (`.memory/`) with opt-in global layer (`~/.claude/memory/`)

---

## Terminology

| Term | What It Is | CLI Surface |
|---|---|---|
| **Mote** | Atomic unit of knowledge — a discrete fact, decision, lesson, exploration, or plan | `mote add`, `mote show`, `mote ls` |
| **Nebula** | The complete mote system: all motes, their links, scores, and the strata beneath them. A cloud of particles forming coherent structure. | Conceptual; referenced in output and docs |
| **Constellation** | A cluster of related motes organized around a common theme. Constellations are motes themselves (type=constellation) that act as hubs in the graph. | `mote constellation list`, `mote constellation synthesize` |
| **Anchor** | A mote (type=anchor) that bridges the nebula to a strata corpus. When an anchor scores high enough during retrieval, it triggers a strata query. | `mote add --type=anchor` |
| **Strata** | Deep, stable reference knowledge stores (API docs, specs, guides, codebases). Layered bedrock beneath the living nebula. Chunked, embedded, and searchable. | `mote strata add`, `mote strata query` |
| **Pulse** | A filtered view of active task motes — the nebula's current heartbeat of work in progress. Not a separate concept; a lens on existing motes. | `mote pulse` (alias for `mote ls --status=active --type=task`) |
| **Dream** | The periodic maintenance cycle where a headless LLM reviews the nebula holistically — discovering latent connections, detecting contradictions, evolving constellations, and proposing crystallizations. Runs outside active sessions. | `mote dream`, `mote dream --review` |
| **Vision** | A proposed change produced by the dream cycle. Visions are never applied automatically — they require human or in-session Claude review. | `mote dream --review` presents visions |
| **Lucid Log** | The accumulating context document maintained across dream batches. Ensures later batches reason with awareness of earlier discoveries. | `.memory/dream/lucid.json` |
| **Crystallize** | The process of converting ephemeral work (closed issues, session exploration) into permanent motes. Analogous to memory consolidation during sleep. | `mote crystallize <id>` |
| **Explore** | A mote type (type=explore) capturing investigative findings: API evaluations, library comparisons, architecture surveys, problem-space analyses. Expensive to reproduce, so scored with a type bonus. | `mote add --type=explore` |
| **External Ref** | A structured reference to an external system (GitHub issue, Jira ticket, etc.) stored as provider/ID/URL triples in mote frontmatter. Ref IDs are indexed for BM25 search. | `--ref provider:id[:url]` on `mote add` |
| **Scan Cache** | A content-hash index at `.memory/dream/scan_state.json` that tracks SHA256 hashes of serialized motes. Enables incremental prescanning — unchanged motes are skipped on subsequent dream runs. | Internal to `mote dream` |

---

## Problem Statement

| Current Pain | Impact |
|---|---|
| MEMORY.md loaded wholesale (~400-800 tokens every session) | Irrelevant knowledge crowds out useful context; no way to load more when the task demands it |
| Linear format — no relationships between facts | Claude cannot reason about connections between past decisions |
| Memory lost when tracked issues close | Planning knowledge disappears; same mistakes recur |
| Exploration from planning sessions is ephemeral | Web searches, API explorations, doc reads, and comparative analysis vanish when a session ends; future sessions repeat the same exploration |
| No cross-project learning | Each project restarts from zero |
| Cannot visualize knowledge structure | No way to see what's connected to what |
| All knowledge treated as equally relevant regardless of age | Stale motes crowd out fresh, actionable knowledge |
| No signal on which knowledge proved important over time | Frequently useful motes are indistinguishable from rarely accessed ones |
| Contradictory knowledge can coexist silently | Claude may act on outdated information without realizing a newer mote supersedes it |
| No holistic nebula maintenance without manual effort | Latent connections, stale motes, and emergent patterns go undetected until they cause problems |
| Nebula reasoning competes with task execution for context | Mid-session nebula analysis degrades both reasoning quality and task focus |

---

## Goals and Success Metrics

| Goal | Metric |
|---|---|
| Load the most relevant context, not all context | `mote prime` returns only signal-dense motes; irrelevant motes are excluded even when token budget remains |
| Scale context to task complexity | Simple tasks load 3-5 motes (~150 tokens); complex tasks load up to 12 motes (~500 tokens) — budget flexes with need |
| Surface relevant past decisions at session start | `mote prime` loads 3-8 relevant motes in < 1s |
| Preserve planning knowledge on issue close | Every significant issue close produces a mote candidate |
| Preserve relevant exploration findings from planning sessions | Explore-heavy sessions produce explore mote candidates at session end; key findings are captured, not just decisions |
| Enable emergent constellation discovery | After 20 motes, `mote constellation synthesize` finds 3+ natural constellations |
| Obsidian graph view works with no setup | Open `.memory/` as vault, all links resolve |
| Retrieval improves with use | Motes accessed across 3+ sessions score measurably higher than unaccessed peers |
| Nebula stays sharp as it grows | At 100+ motes, `mote prime` output quality remains comparable to a 20-nebula |
| Contradictions surface proactively | 100% of `contradicts` pairs are flagged during context loading |
| Dream cycle discovers latent structure | Each dream run proposes ≥ 1 new link or constellation for graphs with 20+ motes |
| Nebula maintenance is autonomous | Dream cycle runs on schedule with no human intervention required to trigger it |
| Reference knowledge is queryable without nebula pollution | Strata corpora serve as deep reference stores; anchor motes bridge to graph when relevant |
| Frequently needed reference knowledge crystallizes to motes | After 3+ sessions querying the same strata topic, the dream cycle proposes a crystallization candidate |

---

## Design Principles — Cognitive Science Foundations

The nebula is modeled on how human memory encodes, connects, and retrieves knowledge. These principles inform the design of every retrieval and scoring mechanism:

| Principle | Cognitive Basis | Mote Implementation |
|---|---|---|
| **Spreading activation** | Activating one concept sends energy to related concepts, decaying with distance | Context traversal walks edges from a seed mote with hop-limited, edge-weighted scoring |
| **Consolidation** | Short-term memories are replayed and strengthened into long-term storage during sleep | Crystallization (Epic 6) reviews ephemeral tracked issues and consolidates valuable ones into permanent motes |
| **Schema formation** | Knowledge clusters into organizing frameworks that accelerate future learning | Constellation synthesis (Epic 5) detects tag constellations and creates schema-like constellation motes as graph hubs |
| **Reconsolidation** | Recalling a memory subtly rewrites it; knowledge evolves | `supersedes` links and deprecation track how understanding changes over time |
| **Encoding specificity** | Retrieval improves when current context matches the encoding context | Priming (Epic 4) uses ambient signals — git branch, recent files, prompt content — as retrieval cues |
| **Emotional salience** | Emotionally significant events are flagged for priority storage and recall | Motes born from failures or reverts receive automatic salience boosts (Epic 11) |
| **Forgetting curve** | Unused memories decay exponentially over time | Time-decay factor in scoring reduces the influence of stale, unaccessed motes (Story 4.4) |
| **Spaced retrieval** | Memories recalled at increasing intervals become more durable | Access tracking strengthens motes that prove useful across multiple sessions (Epic 11) |
| **Cue overload** | Overloaded retrieval cues (too many associations) degrade recall for all items | Tag specificity scoring penalizes overly broad tags and rewards precise ones (Story 4.5) |
| **Interference** | Contradictory knowledge degrades retrieval of both items | Contradiction detection surfaces conflicting motes proactively during context loading (Story 4.6) |
| **REM dream cycles** | The brain cycles through memories multiple times during sleep, finding cross-experience connections that waking cognition misses; each cycle reconsolidates with awareness of prior cycles' discoveries | Sleep cycle (Epic 12) processes the graph in batches with an accumulating journal, so later batches reason with awareness of earlier findings; a reconciliation pass achieves coherence across all batches |
| **Declarative vs. procedural memory** | The brain maintains distinct memory systems — episodic/semantic memory (facts, events) vs. reference knowledge (how things work, what things mean) — that interact but are stored and retrieved differently | Motes hold experiential, relational knowledge (episodic); strata corpora hold stable reference material (semantic); anchor motes bridge the two systems (Epic 13) |

---

## Processing Modes

The mote system separates work into three processing modes, each matched to a different cognitive phase. This separation ensures that time-critical retrieval is never degraded by expensive nebula analysis, and that holistic reasoning gets the unhurried context it needs.

| Mode | Cognitive Analog | Timing | Latency Budget | What Runs | Who Reasons |
|---|---|---|---|---|---|
| **Hot path** | Waking recall | Inline during `mote context` / `mote prime` | < 2 seconds | Scoring arithmetic, graph traversal, structural contradiction flagging, access counter increment, strata augmentation via anchor motes | Deterministic (no LLM) |
| **Warm path** | Transition to/from sleep | Session start (post-prime) and session end | < 10 seconds | Ambient context collection, access flush, crystallization prompts, lightweight link suggestions, explicit strata queries with logging | Claude in-session (has task context) |
| **Dream cycle** | REM sleep | Scheduled (cron/systemd), manual (`mote dream`), or threshold-triggered nudge | 1–10 minutes | Semantic contradiction detection, link inference, tag refinement, constellation evolution, staleness evaluation, mote compression, crystallization triage, strata-to-mote crystallization, corpus health review | Claude Sonnet (batches) + Claude Opus (reconciliation) via headless CLI with OAuth |

**Key constraint:** The dream cycle never runs inline during a Claude Code session. It is always a separate invocation — either scheduled, manually triggered, or nudged at session start. This preserves the session's context budget for the user's actual task.

---

## User Roles

- **Claude (AI agent)**: Primary creator and consumer of motes; runs context loading, creates motes mid-session, reviews dream visions
- **User (human)**: Reviews/approves crystallization candidates, approves constellation synthesis, manually creates high-value motes, reviews dream visions
- **Dream cycle (headless Claude)**: Periodic nebula maintenance — discovers latent connections, detects contradictions, evolves constellations, proposes archival
- **Obsidian viewer**: Passive exploration role via visual graph

---

## Epics and User Stories

---

### Epic 1: Mote Creation and Management

---

#### Story 1.1 — Create a Mote

**As a** Claude session or user
**I want to** create a new mote with a unique ID, type, title, and body
**So that** a discrete piece of knowledge is captured atomically and persistently

**Acceptance Criteria:**

```gherkin
Given I run `mote add`
When I provide a type (task|decision|lesson|context|question|constellation|anchor|explore) and title
Then a new .md file is created in .memory/nodes/ with:
  - A unique ID following the pattern <scope>-<typechar><base36-timestamp>
  - Valid YAML frontmatter with all required fields
  - Retrieval metadata initialized: created_at, last_accessed: null, access_count: 0
  - $EDITOR opens for body content
  - The file is saved on editor exit

Given the mote file is saved
When `mote index rebuild` runs
Then the new mote appears in index.jsonl with no edges (it has no links yet)

Given two concurrent `mote add` calls run within the same second
Then the resulting IDs are distinct (no collision)
```

---

#### Story 1.2 — View a Mote

**As a** Claude session or user
**I want to** view a mote's full content and its current edges
**So that** I can understand what it contains and how it connects to other motes

**Acceptance Criteria:**

```gherkin
Given a mote ID exists in .memory/nodes/
When I run `mote show <id>`
Then the output contains:
  - Full frontmatter fields (id, type, status, title, tags, weight, links)
  - Retrieval stats: last_accessed, access_count
  - Full body content
  - A resolved list of linked mote titles (not just IDs)
And the mote's last_accessed is updated to the current timestamp
And the mote's access_count is incremented by 1

Given a mote ID does not exist
When I run `mote show <nonexistent-id>`
Then a clear error message is printed and exit code is non-zero
```

---

#### Story 1.3 — List Motes

**As a** Claude session or user
**I want to** list motes with optional filters
**So that** I can navigate the graph and find relevant motes without loading them all

**Acceptance Criteria:**

```gherkin
Given motes exist in .memory/nodes/
When I run `mote ls`
Then all motes are listed with: id, type, status, title, weight

When I run `mote ls --type=lesson`
Then only motes with type=lesson are returned

When I run `mote ls --tag=oauth`
Then only motes containing "oauth" in their tags list are returned

When I run `mote ls --status=active`
Then only motes with status=active are returned

When I run `mote pulse`
Then it behaves as an alias for `mote ls --status=active --type=task`
And output shows only active task motes, sorted by weight descending
(This is the nebula's "heartbeat" — current work in progress at a glance)

When I run `mote ls --stale` (no access in 90+ days)
Then only motes with last_accessed older than 90 days (or null) are returned

Given no motes match the filter
Then an empty list (with a friendly message) is returned, exit code 0
```

---

#### Story 1.4 — Deprecate a Mote

**As a** user
**I want to** mark a mote as deprecated
**So that** stale or superseded knowledge is clearly flagged but not deleted

**Acceptance Criteria:**

```gherkin
Given mote A supersedes mote B
When I run `mote link A supersedes B`
Then mote B's status is automatically set to "deprecated"
And mote A's frontmatter contains "supersedes: [B]"
And mote B's frontmatter contains "deprecated_by: A"

Given a mote has status=deprecated
When I run `mote ls` (no filters)
Then deprecated motes are shown with a visual indicator (e.g., [deprecated])

When I run `mote ls --status=active`
Then deprecated motes are excluded
```

---

### Epic 2: Graph Linking — Planning Dimension

---

#### Story 2.1 — Add a Dependency Link

**As a** Claude planning session
**I want to** declare that one mote depends on another
**So that** execution order is encoded in the graph and parallel work is identifiable

**Acceptance Criteria:**

```gherkin
Given motes A and B exist
When I run `mote link A depends_on B`
Then mote A's frontmatter contains "depends_on: [B]"
And mote B's frontmatter contains "blocks: [A]" (auto-populated inverse)
And index.jsonl contains a directed edge from A to B with type "depends_on"
And index.jsonl contains a directed edge from B to A with type "blocks"
```

---

#### Story 2.2 — Identify Parallel Work

**As a** Claude planning session
**I want to** see which active motes have no open blockers
**So that** I can identify work that can proceed in parallel

**Acceptance Criteria:**

```gherkin
Given a set of active task motes with various depends_on links
When I run `mote ls --status=active --ready`
Then only motes where all depends_on targets are status=archived|completed are returned
And the output is sorted by weight descending
```

---

#### Story 2.3 — Traverse the Dependency Chain

**As a** Claude planning session
**I want to** see the full dependency chain from a seed mote
**So that** I can understand the complete execution order for a body of work

**Acceptance Criteria:**

```gherkin
Given mote C depends_on B, B depends_on A
When I run `mote context C --planning`
Then the output shows:
  - A → B → C as the execution chain
  - Any parallel branches at each level
  - Estimated total motes in chain
```

---

### Epic 3: Graph Linking — Memory Dimension

---

#### Story 3.1 — Add a Semantic Memory Link

**As a** Claude session
**I want to** add a typed semantic link between two motes
**So that** thematic connections are captured and traversable

**Acceptance Criteria:**

```gherkin
Given motes X and Y exist
When I run `mote link X relates_to Y`
Then X's frontmatter contains "relates_to: [Y]"
And Y's frontmatter contains "relates_to: [X]" (bidirectional for symmetric types)
And index.jsonl is updated with edges in both directions

When I run `mote link X builds_on Y`
Then X's frontmatter contains "builds_on: [Y]"
And Y's frontmatter does NOT get a reverse "built_by" entry (directional link)
And index.jsonl contains edge X→Y type=builds_on AND Y→X type=built_by_ref (reverse index only)

Supported link types and their bidirectionality:
  relates_to    → bidirectional (stored both ways)
  builds_on     → directional (A deepens B; not symmetric)
  contradicts   → bidirectional
  supersedes    → directional (A replaces B)
  caused_by     → directional (A exists because of B)
  informed_by   → directional (A was shaped by reading B)
```

---

#### Story 3.2 — Remove a Link

**As a** user
**I want to** remove an incorrect or outdated link between motes
**So that** the nebula stays accurate as understanding evolves

**Acceptance Criteria:**

```gherkin
Given a link of type T exists between motes A and B
When I run `mote unlink A T B`
Then the link is removed from A's frontmatter
And the inverse is removed from B's frontmatter if bidirectional
And index.jsonl is updated to remove the corresponding edges
```

---

### Epic 4: Context Loading and Retrieval

---

#### Story 4.1 — Load Context for a Topic (Memory Mode)

**As a** Claude session starting work on a topic
**I want to** load a ranked set of relevant motes from the graph
**So that** I begin with the most pertinent history instead of loading everything

**Acceptance Criteria:**

```gherkin
Given motes exist with various tags and semantic links
When I run `mote context oauth`
Then the output contains:
  - A ranked list of motes related to "oauth" (by tag match + link traversal)
  - Up to 12 motes (ceiling, not target — fewer if fewer are relevant)
  - Each entry shows: id, type, title, weight, and a one-line summary
  - Total output is proportional to match quality: strong matches may load up to 12 motes;
    weak or sparse matches return fewer rather than padding with low-relevance results
  - Traversal goes at most 2 hops from seed motes
  - All returned motes have their last_accessed and access_count updated

Given no motes match the topic
When I run `mote context unknown-topic`
Then a clear "no matching motes found" message is returned, exit code 0
```

---

#### Story 4.2 — Load Context for a Tracked Issue (Planning Mode)

**As a** Claude session starting work on a specific tracked issue
**I want to** load motes related to that issue
**So that** past decisions and lessons relevant to this work surface automatically

**Acceptance Criteria:**

```gherkin
Given a tracked issue ID and motes with source_issue links or matching tags
When I run `mote context <issue-id>`
Then the output contains:
  - Motes that have source_issue: <issue-id> (direct matches)
  - Motes linked from those via memory links (hop 1)
  - Ranked by: direct match > explore/lesson/decision type > relates_to > weight
    (explore motes rank high because they prevent re-investigation)
  - Planning dependency chain if the issue ID has a corresponding task mote

Given no motes reference the tracked issue
Then the command falls back to tag-based search using the issue's labels
```

---

#### Story 4.3 — Prime Context at Session Start

**As a** Claude session
**I want to** load the most relevant context block for the current project's active work
**So that** I can start a session with appropriate memory without manual topic specification

**Acceptance Criteria:**

```gherkin
Given a project has .memory/nodes/ and active tracked issues
When I run `mote prime`
Then the output:
  - Identifies the current active task motes (`mote pulse`)
  - Loads mote context for each active issue (up to 2 issues)
  - Produces a compact summary block sized to relevance:
    minimal for routine sessions, richer when active work has deep history
  - Sections: "Active work", "Relevant decisions", "Key lessons",
    "Prior explorations", "Available strata"
    (Prior explorations section included only when explore motes match active context)
    (Available strata section included only when anchor motes score above threshold)
  - Completes in under 2 seconds
  - Updates last_accessed and access_count for all returned motes

Given no tracked issues are in_progress
Then `mote prime` falls back to listing the 5 highest-weight active motes

Given anchor motes score above threshold during prime traversal
When the prime output is assembled
Then a "Available strata" section is included listing:
  - Each relevant anchor mote's title and strata corpus name
  - A brief note of what the corpus covers (from strata_query_hint)
  - Prompt: "Reference material available — query with `mote strata query <topic>`
    before searching the web"
This ensures Claude knows reference material exists in the nebula's strata layer
before defaulting to external sources. Strata that has already been ingested and
indexed is faster, more reliable, and more relevant than a general web search for
the same domain.

Given ambient context signals are available
When `mote prime` runs
Then the priming signal is enriched with available signals, starting with:
  - Current git branch name (used as an additional tag-match signal)
  - Recently modified files (last 30 min; file paths parsed for keyword signals)
  - Content of the user's initial prompt if piped (keyword extraction for seed matching)
And these signals contribute to seed mote selection before graph traversal begins

The set of ambient signals is extensible. New signal types can be discovered and
registered by the dream cycle (see Design Note below). The prime command reads
the active signal registry from .memory/config.yaml and applies all registered
signal extractors during priming.

Given pending dream visions exist in .memory/dream/visions.jsonl
When `mote prime` runs
Then the output includes a notice:
  "💤 Dream cycle found N visions. Run `mote dream --review` to process."

Given the last dream cycle was more than dream.schedule_hint_days ago (per config)
When `mote prime` runs
Then the output includes a nudge:
  "💤 Dream cycle overdue (last: X days ago). Run `mote dream` when convenient."
```

**Design Note — Encoding Specificity and Emergent Signals:**
Human memory retrieval is strongest when the retrieval context matches the encoding context. By incorporating ambient signals (branch, files, prompt), priming reconstructs a richer retrieval cue — analogous to how returning to a familiar room helps you remember what you went there to do.

The initial signal set (git branch, recent files, prompt keywords) is a starting point, not a ceiling. The dream cycle can discover new signal types by analyzing access patterns and session context:

- **Time-of-day signals**: If certain motes are consistently accessed during morning sessions vs. evening sessions, time becomes a useful priming cue.
- **Co-access patterns**: If motes A and B are always accessed together, loading A should prime B even without an explicit link.
- **Sequential patterns**: If mote A is consistently accessed before mote B across sessions, A's presence should boost B's score.
- **Strata query patterns**: If certain strata corpora are always queried when specific tags appear, the anchor motes for those corpora should be primed automatically.
- **Error context**: If the session starts with an error message or stack trace, keywords from the error can seed retrieval.

The dream cycle proposes new signals as visions (type: `signal_discovery`). When accepted, they are added to the signal registry in `.memory/config.yaml` under `priming.signals`. The prime command reads this registry and applies all active signal extractors. This makes the priming system self-improving — the more the nebula is used, the better it becomes at reconstructing the right retrieval context.

---

#### Story 4.4 — Context Scoring and Ranking

**As a** Claude session
**I want** context traversal to rank motes by relevance, importance, recency, and retrieval history
**So that** the most signal-dense motes fill the limited context budget and the nebula stays sharp as it grows

**Acceptance Criteria:**

```gherkin
Given a traversal produces more than 12 candidate motes
When ranking is applied
Then the score formula is:

  # --- Base score ---
  base = mote.weight                                  # 0.0–1.0

  # --- Edge bonuses (spreading activation weights) ---
  edge_bonus:
    builds_on | supersedes   → +0.3
    caused_by | informed_by  → +0.2
    relates_to               → +0.1

  # --- Status penalty ---
  deprecated                 → -0.5

  # --- Recency decay (forgetting curve) ---
  days_since_access = (now - mote.last_accessed).days
  recency_factor:
    last_accessed < 7 days   → ×1.0
    last_accessed < 30 days  → ×0.85
    last_accessed < 90 days  → ×0.65
    last_accessed ≥ 90 days or never → ×0.4

  # --- Retrieval strength (spaced retrieval) ---
  retrieval_bonus = min(mote.access_count × 0.03, 0.15)

  # --- Salience boost (emotional tagging) ---
  salience_bonus:
    origin = failure | revert | hotfix → +0.2
    origin = discovery                 → +0.1
    origin = normal                    → +0.0
    type = explore                    → +0.1 (exploration is expensive to reproduce)

  # --- Tag specificity (cue overload prevention) ---
  For each matching tag t:
    specificity(t) = 1 / log2(count_of_motes_with_tag(t) + 1)
  tag_specificity_bonus = avg(specificity(t) for matched tags) × 0.2

  # --- Interference penalty (contradiction awareness) ---
  For each active mote that contradicts this mote:
    interference_penalty = -0.1 per contradicting active mote

  # --- Final score ---
  raw = base + edge_bonus + deprecated_penalty
        + retrieval_bonus + salience_bonus
        + tag_specificity_bonus + interference_penalty
  final = raw × recency_factor

And only motes with final score ≥ scoring.min_relevance_threshold are included
And at most 12 results are returned (ceiling, not target)
And scoring parameters are defined in .memory/config.yaml for tuning
```

**Design Note — Cognitive Model Alignment:**

- **Recency decay** models the Ebbinghaus forgetting curve: unaccessed knowledge fades from relevance.
- **Retrieval bonus** models spaced retrieval: motes that keep proving useful across sessions are genuinely important and should surface more readily.
- **Salience bonus** models the amygdala's role in tagging emotionally significant events: lessons from failure deserve priority recall. Explore motes receive an additional type bonus because exploration is expensive to reproduce — re-doing a comparative analysis or API exploration wastes significant session time.
- **Tag specificity** prevents cue overload: a tag on 40 motes is a weak retrieval cue; a tag on 3 motes is a strong one.
- **Interference penalty** models proactive interference: contradictory knowledge should trigger awareness, not silent inclusion.

---

#### Story 4.5 — Tag Specificity Tracking

**As a** Claude session
**I want** the system to track tag frequency and specificity across the graph
**So that** overly broad tags are identified and retrieval cue quality is maintained

**Acceptance Criteria:**

```gherkin
Given motes exist with various tags
When `mote index rebuild` runs
Then index.jsonl includes a tag_stats section with:
  - Each tag and its count (number of motes using it)
  - A computed specificity score: 1 / log2(count + 1)

When I run `mote tags audit`
Then output shows:
  - Tags sorted by count descending
  - Tags with count > 15 are flagged as "overloaded — consider splitting"
  - Suggested sub-tags based on co-occurrence patterns
  - Tags with count = 1 are flagged as "unique — verify intentional"

Given a tag appears on more than 20 motes
When that tag is used as a retrieval seed
Then context scoring applies the specificity penalty from Story 4.4
And the output includes a note: "Tag '<tag>' is broad (N motes); results may be less focused"
```

**Design Note — Cue Overload:**
In human memory, when a single cue is associated with too many memories, retrieval of any one of them degrades. Tag specificity tracking keeps the nebula's retrieval cues sharp by identifying and flagging overloaded tags before they erode recall quality.

---

#### Story 4.6 — Contradiction Detection During Context Loading

**As a** Claude session
**I want** context loading to detect and surface contradictions among returned motes
**So that** I am aware of conflicting knowledge rather than acting on it silently

**Acceptance Criteria:**

```gherkin
Given mote A has a "contradicts" link to mote B
And both A and B are active (not deprecated)
When both appear in a context traversal result
Then the output includes a "⚠ Contradictions" section listing:
  - The contradicting pair (A ↔ B) with titles
  - A prompt: "Consider resolving: deprecate one, or create a superseding mote"

Given mote A contradicts mote B and mote B is deprecated
When context loading encounters this pair
Then only mote A is included and no contradiction warning is shown

Given mote A contradicts motes B and C (multiple contradictions)
When context loading runs
Then all active contradictions involving loaded motes are surfaced
And mote A's score receives -0.1 per contradicting active mote (per Story 4.4)
```

**Design Note — Memory Interference:**
In human memory, contradictory knowledge creates interference — both proactive (old knowledge distorts new learning) and retroactive (new learning distorts old memories). By surfacing contradictions explicitly, the system prevents the AI equivalent: acting on conflicting information without awareness of the conflict.

---

### Epic 5: Constellation Discovery and Emergence

---

#### Story 5.1 — View Tag Frequency

**As a** user
**I want to** see which tags appear most frequently across motes
**So that** I can identify natural constellations before running synthesis

**Acceptance Criteria:**

```gherkin
Given motes exist with various tags
When I run `mote constellation list`
Then output shows tags sorted by frequency (count of motes using that tag)
And each row shows: tag, count, specificity score, whether a constellation mote already exists for it
```

---

#### Story 5.2 — Synthesize a Constellation Mote

**As a** user
**I want to** run a synthesis pass that finds tag constellations and proposes constellation motes
**So that** emergent patterns in my knowledge graph become first-class nebula nodes

**Acceptance Criteria:**

```gherkin
Given tags exist that appear in 3 or more motes
When I run `mote constellation synthesize`
Then for each qualifying tag constellation without an existing constellation mote:
  - Claude drafts a constellation mote body summarizing the common thread
  - The draft is shown to the user for review
  - User can: approve (writes mote), edit (opens $EDITOR), or skip

Given user approves a constellation mote
Then the constellation mote is written to nodes/ with type=constellation
And all motes in the cluster receive a "relates_to: [constellation-id]" link
And constellations.jsonl is updated with the cluster entry

Given a tag constellation already has a constellation mote
When synthesis runs again
Then that cluster is skipped (no duplicate constellation motes)
```

---

#### Story 5.3 — Constellation Motes Appear in Obsidian Graph as Hubs

**As a** user viewing the Obsidian graph
**I want** constellation motes to be visually distinguishable as hubs
**So that** the nebula's cluster structure is immediately apparent

**Acceptance Criteria:**

```gherkin
Given constellation motes exist with relates_to links to member motes
When the .memory/ directory is opened as an Obsidian vault
Then:
  - Constellation motes have more connections than non-constellation motes
  - Constellation motes appear as visual hubs in Obsidian's graph view
  - Wikilinks in constellation mote bodies resolve to member mote files
  - No Obsidian plugin is required for this to work
```

---

### Epic 6: Issue Integration — Crystallization (Consolidation)

**Design Note — Memory Consolidation:**
In the brain, the hippocampus coordinates the replay of recent experiences during sleep, strengthening important ones into durable cortical memories and letting trivial ones fade. Crystallization is the nebula's consolidation process: ephemeral planning artifacts (tracked issues) are reviewed, and the valuable ones are consolidated into permanent knowledge — including not just decisions and lessons, but the exploration that informed them. Exploration findings (API evaluations, comparative analyses, exploration results) are among the most frequently re-needed and most expensive to reproduce, making them high-value crystallization candidates. The human review step mirrors the brain's selectivity — not everything is worth remembering.

---

#### Story 6.1 — Crystallize a Closed Issue to a Mote

**As a** user
**I want to** convert a closed tracked issue into a permanent mote
**So that** the planning knowledge and lessons from that work are preserved in the memory graph

**Acceptance Criteria:**

```gherkin
Given a tracked issue exists (open or closed)
When I run `mote crystallize <id>`
Then:
  - The source mote or issue data is retrieved
  - A draft mote file is created with:
    - title from source title
    - type inferred: "decision" if source type=decision,
        "explore" if source contains substantial exploration notes (comparisons,
          evaluations, links to external sources, API exploration results),
        "lesson" for sources with operational notes,
        else "context"
    - tags from source labels
    - body populated from description + notes fields
    - source_issue: <id> field set
    - crystallized_at: timestamp set
    - origin: inferred from source metadata (see Story 11.2 for origin classification)
  - $EDITOR opens with the draft for human review
  - On save+exit, the mote is confirmed and added to index

Given user exits editor without saving
Then no mote is created and the crystallization is cancelled

Given the crystallization is confirmed
Then the source mote's status is updated to "completed" if it was a task mote
```

---

#### Story 6.2 — Crystallize Multiple Issues in Batch

**As a** user reviewing closed tracked issues
**I want to** identify which closed issues have not yet been crystallized
**So that** I can selectively crystallize the high-value ones without missing any

**Acceptance Criteria:**

```gherkin
Given multiple closed tracked issues exist
When I run `mote crystallize --candidates`
Then output lists closed issues without a corresponding mote (source_issue not found in index)
And each candidate shows: issue id, title, type, close date

Given I select an issue to crystallize
When I run `mote crystallize <id>`
Then the crystallization flow from Story 6.1 runs
```

---

### Epic 7: Index Management

---

#### Story 7.1 — Rebuild the Edge Index

**As a** user or Claude session
**I want to** rebuild the edge index from mote files
**So that** the index stays consistent after manual file edits or when first initializing

**Acceptance Criteria:**

```gherkin
Given .memory/nodes/ contains mote files
When I run `mote index rebuild`
Then:
  - All .md files in nodes/ are read
  - YAML frontmatter link fields are parsed
  - index.jsonl is written with one edge record per directed link
  - Bidirectional link types (relates_to, contradicts) produce two records each
  - tag_stats section is computed (tag, count, specificity for each tag)
  - The rebuild completes without requiring any external database or service
  - Output shows: "Rebuilt index: X motes, Y edges, Z tags"

Given a mote file has malformed YAML frontmatter
When `mote index rebuild` runs
Then a warning is printed for that file and the rebuild continues for all others
```

---

#### Story 7.2 — Validate Graph Integrity

**As a** user
**I want to** check for broken links or orphaned motes in the graph
**So that** the nebula stays internally consistent

**Acceptance Criteria:**

```gherkin
Given motes with links to other mote IDs
When I run `mote doctor`
Then output identifies:
  - Links pointing to non-existent mote IDs (broken links)
  - Motes with no incoming or outgoing links (isolated motes)
  - Motes with status=deprecated that are still linked as depends_on targets
  - Active contradictions: pairs of active motes linked by "contradicts" (unresolved conflicts)
  - Overloaded tags: tags appearing on > 15 motes (retrieval cue degradation risk)
  - Stale motes: active motes with last_accessed > 180 days or null (forgetting curve candidates)
And each issue shows the affected mote ID and the specific problem
And exit code is non-zero if any issues are found
```

---

### Epic 8: Obsidian Compatibility

---

#### Story 8.1 — Open Motes Directory as Obsidian Vault

**As a** user
**I want to** open `.memory/` as an Obsidian vault with no configuration
**So that** I can visually explore the nebula without any setup

**Acceptance Criteria:**

```gherkin
Given .memory/nodes/ contains mote .md files with wikilinks in their bodies
When the .memory/ directory is opened as an Obsidian vault
Then:
  - All mote files appear in Obsidian's file explorer
  - [[mote-id]] wikilinks in bodies resolve to the correct mote files
  - YAML frontmatter renders in Obsidian's Properties panel
  - Graph view shows motes as nodes and wikilinks as edges
  - No plugin installation is required
```

---

#### Story 8.2 — Global + Project Vault

**As a** user
**I want to** see both project-local motes and global motes in a single Obsidian vault
**So that** I can explore cross-project connections visually

**Acceptance Criteria:**

```gherkin
Given project motes in .memory/nodes/ and global motes in ~/.claude/memory/nodes/
When both directories are added to an Obsidian vault configuration
Then:
  - Both sets of motes appear in the vault
  - Cross-scope links (project mote linking to global mote by ID) resolve correctly
  - Graph view shows cross-scope edges
```

---

### Epic 9: Migration from MEMORY.md

---

#### Story 9.1 — Migrate Existing MEMORY.md Sections to Motes

**As a** user with an existing MEMORY.md
**I want to** migrate its sections into individual motes
**So that** historical knowledge enters the graph and the linear format is retired

**Acceptance Criteria:**

```gherkin
Given ~/.claude/projects/<project>/memory/MEMORY.md exists
When I run the migration plan for myproject:

  Sections to migrate:
  - "Workflow instructions" → NOT a mote; extract to workflow skill
  - "Project state snapshot" → mote type=context, tags=[project-state, services], weight=0.4
  - "API integration notes" → mote type=lesson, tags=[api, integration, auth], weight=0.9
  - "Config notes" → mote type=context, tags=[config, deployment], weight=0.6
  - "Service aliases" → mote type=context, tags=[services, config], weight=0.6
  - "Environment setup fix" → mote type=lesson, tags=[environment, setup, debugging], weight=0.9

Then after migration:
  - 5-6 mote files exist in .memory/nodes/
  - `mote index rebuild` runs without errors
  - `mote context oauth` returns the OAuth lesson mote
  - `mote context ollama` returns the CUDA fix and model alias motes
  - MEMORY.md is renamed to MEMORY.md.archived (not deleted)
  - The project CLAUDE.md is updated to reference `mote prime` instead of MEMORY.md

  Salience classification for migrated motes:
  - "OAuth/x-api-key patches" → origin=failure (discovered through debugging)
  - "Ollama CUDA/CachyOS fix" → origin=failure (system crash recovery)
  - Others → origin=normal
```

---

### Epic 10: Global Memory Layer

---

#### Story 10.1 — Promote a Mote to Global Scope

**As a** user
**I want to** promote a project-scoped mote to the global memory layer
**So that** general lessons (not project-specific) are available across all Claude sessions

**Acceptance Criteria:**

```gherkin
Given a project mote contains knowledge applicable beyond the current project
When I run `mote promote <mote-id>`
Then:
  - A copy of the mote is created in ~/.claude/memory/nodes/ with scope=global
  - The new ID uses the "global-" prefix
  - The original project mote is updated with: promoted_to: <global-mote-id>
  - A relates_to link connects the project mote to the global mote
  - index.jsonl in both scopes is updated

Given the promoted global mote exists
When `mote prime` runs in any project
Then global motes are included in the context traversal
And global motes are visually distinguished in output (e.g., [global] prefix)
```

---

### Epic 11: Retrieval Adaptation and Salience

---

#### Story 11.1 — Track Mote Access Patterns

**As a** Claude session
**I want** mote access patterns to be recorded automatically
**So that** the system learns which knowledge is genuinely useful over time

**Acceptance Criteria:**

```gherkin
Given a mote is loaded via `mote context`, `mote prime`, or `mote show`
When the mote content is returned
Then the mote's frontmatter is updated:
  - last_accessed: current ISO timestamp
  - access_count: incremented by 1

Given a mote has been accessed across 3+ distinct sessions (different dates)
Then its retrieval_bonus in scoring is measurably positive (up to +0.15)

Given a mote has never been accessed (last_accessed: null)
Then its recency_factor is 0.4 (lowest tier in forgetting curve)

Given access tracking would add latency
Then writes are batched and flushed at session end, not per-mote
```

---

#### Story 11.2 — Automatic Salience Classification

**As a** Claude session or user
**I want** motes born from failures, reverts, or hotfixes to be automatically flagged as high-salience
**So that** hard-won lessons surface with priority in future retrieval

**Acceptance Criteria:**

```gherkin
Given a mote is created via `mote crystallize` from a tracked issue
When the issue metadata contains indicators of failure:
  - Labels include: "bug", "fix", "revert", "hotfix", "regression", "incident"
  - Issue type is "bugfix" or "incident"
  - Issue title contains: "fix", "revert", "broke", "broken", "crash", "fail"
Then the mote's frontmatter includes: origin: failure
And the mote receives a +0.2 salience bonus in scoring

Given a mote is created manually via `mote add`
When the user specifies `--origin=failure` or `--origin=revert`
Then the origin field is set accordingly

Given no origin indicator is detected or specified
Then origin defaults to "normal" (no salience bonus)

Supported origin values:
  failure    → +0.2 (bugs, crashes, regressions)
  revert     → +0.2 (rolled-back changes)
  hotfix     → +0.2 (emergency fixes)
  discovery  → +0.1 (unexpected findings during exploration)
  research   → +0.0 (origin, not type — explore motes get a separate type bonus in scoring)
  normal     → +0.0 (routine knowledge)
```

**Design Note — Emotional Salience:**
The amygdala tags emotionally significant experiences — particularly failures and threats — for priority storage and easier future recall. This is why you remember a painful mistake years later but forget routine successes. The origin field is the nebula's amygdala: it ensures that knowledge born from things going wrong surfaces with appropriate priority, because those are exactly the lessons most worth remembering.

**Design Note — Explore as a Distinct Knowledge Type:**
Explore motes capture the *findings and reasoning* from investigative sessions: API evaluations, library comparisons, architectural explorations, performance benchmarks, and problem-space investigations. They differ from lessons (which capture what went wrong or right) and decisions (which capture what was chosen and why). A explore mote preserves the *landscape of options* and the *evidence gathered* — knowledge that is expensive to reproduce and frequently needed when revisiting a domain. Good explore motes include:
- What was investigated and why
- Key findings, comparisons, or tradeoffs discovered
- Sources consulted (URLs, docs, repos)
- What was ruled out and why (negative results are valuable)
- Open questions that remain

Explore motes should NOT be raw session transcripts or exhaustive notes. They should be distilled summaries that a future session can use to avoid re-doing the same investigation.

---

#### Story 11.3 — Retrieval Health Dashboard

**As a** user
**I want to** see how the retrieval system is performing over time
**So that** I can tune scoring parameters and identify degradation

**Acceptance Criteria:**

```gherkin
When I run `mote stats`
Then output shows:
  - Total motes by status (active, deprecated, archived)
  - Access distribution: how many motes were accessed in last 7/30/90 days
  - "Never accessed" count (potential dead weight)
  - Top 10 most-accessed motes (proven high-value)
  - Tag health: count of overloaded tags (>15 motes), count of singleton tags
  - Active contradictions count
  - Average recency_factor across active motes (nebula freshness indicator)
  - Dream cycle status: last run, pending visions count

When I run `mote stats --decay-preview`
Then output shows:
  - List of motes that would lose ≥30% of their score from recency decay
  - Suggested actions: archive, refresh (re-access), or leave as-is
```

---

### Epic 12: Dream Cycle — Consolidation and Maintenance

**Design Note — REM Sleep and Memory Consolidation:**
During sleep, the brain does two things that cannot happen during waking cognition: it replays experiences without the pressure of real-time response, and it finds connections across the full day's input that couldn't be detected in the moment. The brain doesn't do this in a single pass — it cycles through multiple REM periods, each one reconsolidating memories with the benefit of what was discovered in prior cycles. Early cycles tend to replay specific experiences; later cycles do more creative recombination.

The dream cycle (`mote dream`) is the nebula's equivalent. A headless Claude session — outside any active task — reasons over the full graph without latency pressure, context budget competition, or task focus distraction. It processes motes in batches (analogous to REM cycles), maintaining an accumulating lucid log so that later batches reason with awareness of earlier discoveries. A final reconciliation pass achieves coherence across all batches, revising or withdrawing visions that conflict with later findings.

All dream cycle outputs are **visions, not mutations**. The nebula is never modified during dreaming. Visions are reviewed and accepted by the user or Claude at the next session start, preserving human authority over nebula evolution.

---

#### Story 12.1 — Dream Cycle Pipeline

**As a** user or scheduled process
**I want to** run a holistic nebula maintenance pass using LLM reasoning
**So that** latent connections, contradictions, stale knowledge, and emergent constellations are discovered without competing with active task work

**Acceptance Criteria:**

```gherkin
Given .memory/nodes/ contains motes
When I run `mote dream`
Then the following pipeline executes:

  Stage 1 — Deterministic Pre-scan:
    - Rebuild tag stats (frequency, specificity)
    - Compute recency decay tiers for all motes
    - Identify link inference candidates (mote pairs sharing 3+ tags with no existing link)
    - Identify contradiction candidates (motes sharing 2+ tags with opposing conclusions)
    - Identify overloaded tags (count > dream.tag_overload_threshold)
    - Identify stale motes (last_accessed > dream.staleness_threshold_days or null)
    - Identify constellation evolution candidates (constellation motes where member count grew 30%+ since last synthesis)
    - Identify compression candidates (body > 300 words and access_count > 0)
    - Identify uncrystallized tracked issues (closed issues without corresponding motes)
    - Identify strata crystallization candidates from query_log.jsonl (Story 13.7)
    - Analyze access patterns for emergent priming signals:
      co-access pairs, sequential patterns, time-of-day correlations,
      strata-tag associations (Story 4.3 signal discovery)
    - Construct batch groups using hybrid strategy (Story 12.3)
    - Initialize empty lucid log

  Stage 2 — Batch Reasoning (Claude Sonnet via headless CLI, OAuth):
    - For each batch:
      - Load the current lucid log into context
      - Load the batch's mote contents
      - Reason over applicable tasks: link inference, contradiction detection,
        tag refinement, staleness evaluation, compression candidates,
        strata crystallization candidates (Story 13.7)
      - Write visions to .memory/dream/visions_draft.jsonl
      - Update the lucid log (patterns, tensions, visions summary)
      - Check for interrupt signals; record any interrupts to lucid log

  Stage 3 — Reconciliation Pass (Claude Opus via headless CLI, OAuth):
    - Load the full lucid log + all draft visions + any interrupt signals
    - Coherence check: identify visions that conflict with each other or
      with findings from later batches
    - Pattern completion: detect emergent patterns in the lucid log that no
      individual batch proposed action on (e.g., cross-batch constellation candidates)
    - Confidence adjustment: revise confidence scores on early-batch visions
      that lacked later-batch context
    - Revise, withdraw, or qualify visions as needed
    - Draft constellation synthesis and crystallization candidates (deep reasoning tasks)
    - Write final visions to .memory/dream/visions.jsonl

  Stage 4 — Log:
    - Write run record to .memory/dream/log.jsonl with:
      timestamp, mote_count, batch_count, vision_count,
      interrupt_count, duration_seconds, lucid_hash

Given `mote dream` completes
Then .memory/dream/visions.jsonl contains only final (post-reconciliation) visions
And .memory/dream/lucid.json is preserved for debugging and audit
And no mote files have been modified (visions only, no mutations)

Given `mote dream` is run and no candidates pass the pre-scan filters
Then the pipeline completes quickly with: "No maintenance work identified."
And log.jsonl records a clean run with vision_count: 0
```

---

#### Story 12.2 — Lucid Log Schema and Accumulation

**As a** dream cycle batch
**I want to** read what previous batches discovered and contribute my own findings
**So that** later batches reason with awareness of earlier discoveries and the reconciliation pass has a complete picture

**Acceptance Criteria:**

```gherkin
Given the lucid log is initialized at the start of `mote dream`
Then it is a JSON document with three sections:

  {
    "observed_patterns": [],
    "tensions": [],
    "visions_summary": [],
    "interrupts": [],
    "strata_health": [],
    "metadata": {
      "batch_count": 0,
      "motes_processed": 0,
      "last_updated_by_batch": null
    }
  }

Given batch N completes its reasoning
Then it updates the lucid log as follows:

  observed_patterns[]:
    Each entry: {
      "pattern_id": "<slug>",
      "description": "Retry logic appears across HTTP client, queue worker, and OAuth modules",
      "supporting_motes": ["proj-L1a", "proj-C3b", "proj-D7x"],
      "first_seen_batch": 2,
      "last_updated_batch": 5,
      "strength": "strong|moderate|tentative"
    }
    - If a pattern was already recorded by a prior batch, update it
      (add supporting motes, adjust strength) rather than creating a duplicate
    - New patterns are added with first_seen_batch = current batch number

  tensions[]:
    Each entry: {
      "tension_id": "<slug>",
      "description": "proj-L1a says service restart required after config change;
                       proj-D2b says hot-reload works for the same service",
      "motes": ["proj-L1a", "proj-D2b"],
      "status": "unresolved|resolved|superseded",
      "first_seen_batch": 3,
      "resolution": null
    }
    - Later batches may resolve a tension by setting status and resolution
    - Unresolved tensions are flagged for the reconciliation pass

  visions_summary[]:
    Each entry: {
      "vision_type": "link|contradiction|tag_split|stale|constellation|compression|crystallization|strata_crystallization|signal_discovery",
      "batch": 2,
      "affected_motes": ["proj-L1a", "proj-C3b"],
      "brief": "Proposed relates_to link based on shared retry-logic pattern",
      "confidence": 0.82
    }
    - This is a compact index of visions, not the full proposal records
    - Enables later batches to check for conflicts without reading full visions

  interrupts[]:
    Each entry: {
      "batch": 4,
      "severity": "high|medium",
      "description": "Dependency on removed library X affects 8 motes",
      "affected_motes": ["proj-A1", "proj-A2", ...],
      "recommendation": "Targeted follow-up dream cycle for affected subgraph"
    }

Given the lucid log grows across batches
Then each batch reads the full lucid log before reasoning
And each batch writes its updates after completing reasoning
And the lucid log remains a single document (updated in place, not appended)

Given the lucid log must fit in context alongside batch motes
Then the lucid log is kept compact:
  - Pattern descriptions are 1-2 sentences max
  - Tension descriptions are 1-2 sentences max
  - Proposals summary uses only the brief format above
  - Total journal size should not exceed 2000 tokens at any point
  - If approaching the limit, the oldest resolved tensions and
    weakest tentative patterns are pruned
```

---

#### Story 12.3 — Hybrid Batch Construction

**As a** dream cycle pipeline
**I want to** group motes into batches that maximize both within-cluster and cross-cluster discovery
**So that** the lucid log captures both focused per-topic findings and unexpected cross-domain connections

**Acceptance Criteria:**

```gherkin
Given N active motes need processing and max_motes_per_batch = M
When batch construction runs
Then batches are built using a hybrid strategy:

  Phase A — Tag-clustered batches (first ~60% of batches):
    - Group motes by their primary tag constellation (most specific shared tag)
    - Each batch contains motes that are thematically related
    - Purpose: build a strong journal of per-topic findings
    - If a tag constellation exceeds M motes, split into multiple batches
      preserving cluster identity
    - Motes with no tags or only singleton tags are deferred to Phase B

  Phase B — Interleaved batches (remaining ~40% of batches):
    - Deliberately mix motes from different tag constellations
    - Prioritize motes that were deferred from Phase A (untagged, singleton-tagged)
    - Include at least one mote from each major cluster that hasn't been
      cross-referenced yet
    - Purpose: find unexpected cross-domain connections the clustered
      batches missed

  Batch ordering:
    - Phase A batches run first (build journal foundation)
    - Phase B batches run second (cross-pollinate with lucid log context)
    - Within each phase, batches are ordered by total weight descending
      (highest-value motes processed first)

Given fewer than M total active motes exist
Then all motes are processed in a single batch and no hybrid split is needed

Given the clustered_fraction config value is changed
Then the Phase A / Phase B split adjusts accordingly

Given batch construction produces batch assignments
Then each batch record includes:
  - batch_number (sequential)
  - phase: "clustered" | "interleaved"
  - mote_ids: list of mote IDs in this batch
  - primary_cluster: tag name (for Phase A) | "mixed" (for Phase B)
```

**Design Note — Dream Cycle Architecture:**
This mirrors how the brain's early dream cycles tend to replay specific experiences (consolidating within-domain knowledge), while later cycles do more creative recombination across domains. The lucid log from the clustered phase gives the interleaved phase a strong foundation of per-topic understanding, enabling more meaningful cross-domain connections.

---

#### Story 12.4 — Reconciliation Pass

**As a** dream cycle pipeline
**I want** a final reasoning pass that reviews all visions with full lucid log context
**So that** cross-batch conflicts are resolved, emergent patterns are completed, and early-batch visions are revised with later knowledge

**Acceptance Criteria:**

```gherkin
Given all batches have completed and the lucid log is finalized
When the reconciliation pass runs (Claude Opus via headless CLI, OAuth)
Then it receives:
  - The full lucid log (all patterns, tensions, visions summary, interrupts)
  - All draft visions from .memory/dream/visions_draft.jsonl
  - The ability to request specific mote contents by ID (targeted re-fetch)

The reconciliation pass performs:

  1. Coherence Check:
     - Identify visions that conflict with each other
       (e.g., batch 2 proposes linking M12→M9; batch 5 discovers M12 contradicts M47)
     - For each conflict: revise the earlier proposal, withdraw it, or add a caveat
     - Flag: "Revised by reconciliation" with rationale

  2. Pattern Completion:
     - Review journal patterns that no batch proposed action on
     - If a pattern has 3+ supporting motes and no corresponding constellation:
       propose a new constellation mote with a drafted body
     - If a pattern suggests a link type not yet proposed: draft the link proposal

  3. Confidence Adjustment:
     - Proposals from early batches (batch 1-2) had less lucid log context
     - Adjust confidence scores based on whether later batches confirmed
       or undermined the reasoning
     - Proposals confirmed by later evidence: confidence +0.1
     - Proposals with no later corroboration: confidence unchanged
     - Proposals undermined by later findings: confidence -0.2 or withdraw

  4. Interrupt Processing:
     - For each interrupt signal in the lucid log:
       - Identify which already-processed visions touch affected motes
       - Flag those visions for withdrawal or re-evaluation
       - If the interrupt affects a large subgraph (>20% of processed motes):
         recommend a targeted follow-up dream cycle
       - Write interrupt disposition to the final visions file

  5. Deep Reasoning Tasks:
     - Constellation synthesis drafts (reading 5-10 motes per constellation, writing coherent prose)
     - Crystallization candidate evaluation (judging which closed issues are worth preserving)
     - Exploration distillation (identifying session transcripts or verbose motes that
       contain exploration findings and drafting focused explore motes from them)
     - Mote compression drafts (distilling verbose motes to essential insights)
     - These tasks use the full lucid log context for informed reasoning

Given reconciliation completes
Then:
  - visions_draft.jsonl is consumed (may be deleted or archived)
  - visions.jsonl contains only final, reconciled visions
  - Each vision includes:
    - reconciliation_status: "confirmed" | "revised" | "withdrawn"
    - reconciliation_notes: rationale for any changes (null if confirmed unchanged)
    - final_confidence: adjusted confidence score
  - The lucid log is preserved as lucid.json for debugging

Given the reconciliation pass needs to re-read specific motes
Then it requests them by ID through a targeted fetch mechanism
And the total re-fetched motes do not exceed dream.reconciliation.max_refetch_motes
```

---

#### Story 12.5 — Interrupt Mechanism

**As a** dream cycle batch
**I want to** signal when a discovery is significant enough to affect the interpretation of previously processed batches
**So that** the reconciliation pass can handle disruptions without halting the pipeline

**Acceptance Criteria:**

```gherkin
Given a batch discovers a significant issue during reasoning
When the issue meets interrupt criteria:
  - A foundational mote is discovered to be obsolete (dependency removed, API changed)
  - A cluster of 5+ motes share an incorrect assumption
  - A critical contradiction is found between high-weight motes
Then the batch writes an interrupt signal to the lucid log:
  {
    "batch": N,
    "severity": "high" | "medium",
    "description": "Description of the disruptive finding",
    "affected_motes": ["proj-A1", "proj-A2", ...],
    "recommendation": "What the reconciliation pass should consider"
  }

Given an interrupt is recorded
Then the pipeline does NOT halt — the current batch completes normally
And all subsequent batches see the interrupt in the lucid log
And subsequent batches may factor the interrupt into their reasoning
And the reconciliation pass treats the interrupt as a first-class input

Given a high-severity interrupt affects > 20% of total motes
When the reconciliation pass processes it
Then the final visions include a recommendation:
  "Targeted follow-up dream cycle recommended for [affected subgraph]"
And the recommendation includes the specific mote IDs to re-examine

Given a medium-severity interrupt
When the reconciliation pass processes it
Then affected visions are flagged and confidence-adjusted
But no follow-up dream cycle is recommended unless corroborated by other findings
```

---

#### Story 12.6 — Dream Vision Schema and Review

**As a** user or Claude session
**I want to** review dream cycle visions interactively before they modify the graph
**So that** I maintain authority over nebula evolution and can reject low-quality suggestions

**Acceptance Criteria:**

```gherkin
Given .memory/dream/visions.jsonl contains visions
When I run `mote dream --review`
Then each proposal is presented interactively:
  - Vision type, affected motes, confidence score, rationale
  - Reconciliation status and notes (if revised or flagged)
  - User can: accept (applies the change), edit (modify before applying),
    reject (discard), or defer (keep in visions for next review)

Vision types and their apply actions:

  link:
    {"type": "link", "action": "add", "source": "proj-L1a",
     "target": "proj-C4d5", "link_type": "relates_to",
     "confidence": 0.82,
     "rationale": "Both address Ollama configuration edge cases...",
     "reconciliation_status": "confirmed", "final_confidence": 0.82}
    → Apply: runs `mote link proj-L1a relates_to proj-C4d5`

  contradiction:
    {"type": "contradiction", "motes": ["proj-L1a", "proj-D2b3"],
     "confidence": 0.71,
     "rationale": "L1a says restart required; D2b3 says hot-reload works...",
     "reconciliation_status": "confirmed", "final_confidence": 0.71}
    → Apply: runs `mote link proj-L1a contradicts proj-D2b3`

  tag_split:
    {"type": "tag_split", "tag": "config",
     "proposed": ["config-docker", "config-ollama", "config-systemd"],
     "affected_motes": ["proj-C1", "proj-C2", ...],
     "confidence": 0.88,
     "rationale": "Tag 'config' appears on 22 motes across 3 distinct domains..."}
    → Apply: updates tags in each affected mote's frontmatter

  stale:
    {"type": "stale", "mote": "proj-C7x8",
     "recommendation": "archive",
     "rationale": "Project state snapshot from 6 months ago; superseded...",
     "reconciliation_status": "confirmed", "final_confidence": 0.90}
    → Apply: sets mote status to "archived"

  constellation:
    {"type": "constellation", "tag_constellation": "retry-logic",
     "member_motes": ["proj-L1a", "proj-C3b", "proj-D7x", ...],
     "drafted_body": "## Retry Logic Patterns\n\nAcross HTTP client...",
     "confidence": 0.85}
    → Apply: creates constellation mote via `mote add --type=constellation`, links members

  compression:
    {"type": "compression", "mote": "proj-L5f2",
     "original_word_count": 420, "compressed_word_count": 95,
     "compressed_body": "Distilled insight...",
     "confidence": 0.77}
    → Apply: replaces mote body with compressed version (original preserved in git)

  crystallization:
    {"type": "crystallization", "source_issue": "issue-47",
     "drafted_mote": { "title": "...", "type": "lesson", "tags": [...], ... },
     "confidence": 0.80}
    → Apply: runs crystallization flow from Story 6.1 with pre-filled draft

  strata_crystallization:
    {"type": "strata_crystallization", "corpus": "anthropic-api-docs",
     "source_chunks": ["chunk-id-1", "chunk-id-2"],
     "drafted_mote": { "title": "...", "type": "lesson", "tags": [...],
       "informed_by": ["proj-Ranchor1"], "origin": "discovery", ... },
     "query_pattern": "auth token refresh — queried 7 times across 4 sessions",
     "confidence": 0.78}
    → Apply: creates mote, links to anchor mote, annotates query_log

  signal_discovery:
    {"type": "signal_discovery", "signal_name": "co-access-oauth-litellm",
     "signal_type": "co_access",
     "description": "Motes tagged 'oauth' and 'litellm' are co-accessed in 85% of sessions",
     "extractor": "When any mote with tag 'oauth' is loaded, boost motes with tag 'litellm' by +0.15",
     "confidence": 0.72}
    → Apply: adds signal to priming.signals in config.yaml

Given all proposals are processed (accepted, rejected, or deferred)
Then accepted proposals are applied to the graph
And visions.jsonl is updated: accepted/rejected visions removed,
  deferred visions retained for next review
And a summary is printed: "Applied N visions, rejected M, deferred K"

Given `mote dream --review` is run with no pending visions
Then output: "No pending dream visions. Last dream: <timestamp>"
```

---

#### Story 12.7 — Dream Scheduling and Invocation

**As a** user
**I want to** run the dream cycle manually, on a schedule, or be nudged when it's overdue
**So that** nebula maintenance happens reliably without requiring me to remember

**Acceptance Criteria:**

```gherkin
Given the user runs `mote dream` manually
Then the full pipeline (Story 12.1) executes
And output shows progress: pre-scan stats, batch progress, reconciliation status
And final output summarizes: "Dream complete: N visions generated. Run `mote dream --review`."

Given the user runs `mote dream --dry-run`
Then the full pipeline executes including LLM reasoning
But proposals are written to stdout, not to proposals.jsonl
And no files in .memory/ are created or modified
And output is clearly marked: "[DRY RUN] No files written."

Given a cron job or systemd timer invokes `mote dream`
Then the pipeline runs non-interactively
And visions are written to .memory/dream/visions.jsonl
And the log is written to .memory/dream/log.jsonl
And exit code 0 on success, non-zero on failure

Example cron entry:
  0 3 * * * cd /path/to/project && mote dream 2>&1 >> /tmp/mote-dream.log

Example systemd timer:
  [Timer]
  OnCalendar=*-*-* 03:00:00
  Persistent=true

Given `mote prime` runs at session start
When .memory/dream/log.jsonl shows the last dream was more than
  dream.schedule_hint_days ago (from config)
Then prime output includes:
  "💤 Dream cycle overdue (last: X days ago). Run `mote dream` when convenient."

Given `mote prime` runs at session start
When .memory/dream/visions.jsonl contains pending visions
Then prime output includes:
  "💤 N dream visions pending review. Run `mote dream --review` to process."
```

---

#### Story 12.8 — Session Bookends (Warm Path)

**As a** Claude Code session
**I want** lightweight maintenance to run at session boundaries
**So that** access data is flushed, crystallization candidates are flagged, and simple link suggestions surface without waiting for a full dream cycle

**Acceptance Criteria:**

```gherkin
Given a Claude Code session is ending
When `mote session-end` runs
Then:
  - All batched access updates (last_accessed, access_count) are flushed to mote files
  - If tracked issues were closed during the session:
    output lists crystallization candidates with: "Consider: `mote crystallize <id>`"
  - If Claude noticed thematic connections during the session that aren't yet linked:
    output suggests up to 3 link proposals (lightweight, no full graph scan)
  - If the session involved significant exploration (web searches, API exploration,
    documentation analysis, comparative evaluation, or architectural investigation):
    output identifies exploration preservation candidates:
      - A brief summary of what was researched and key findings
      - Suggested mote type: "explore"
      - Suggested tags inferred from exploration topics
      - Prompt: "Exploration findings detected. Create mote? `mote add --type=explore`"
    Exploration detection heuristics:
      - Session included 3+ web searches or doc lookups on a related topic
      - Session produced comparative analysis (evaluating options, tradeoffs)
      - Session explored an API, library, or framework not yet represented in the graph
      - Session investigated a problem space before making a decision
  - Total execution time < 5 seconds

Given `mote session-end` is run with no pending updates
Then it completes silently with no output
```

---

### Epic 13: Reference Knowledge Base — Hybrid Strata Integration

**Design Note — Declarative vs. Reference Memory:**
The brain maintains distinct but interacting memory systems. Episodic memory holds personal experiences — things that happened, decisions you made, lessons you learned. Semantic/reference memory holds general knowledge — how APIs work, what a protocol specifies, what a framework's conventions are. You don't memorize a reference manual; you remember *that it exists*, *where to find it*, and *the key insights you've drawn from it*. The manual stays on the shelf. Your understanding of it lives in your head.

The nebula is episodic memory: experiential, relational, evolving, subject to decay and salience. Strata corpora are reference memory: stable, voluminous, queried rather than connected. Anchor motes bridge the two systems — they live in the graph and represent your *relationship* to reference material, enabling the nebula's scoring and traversal to control when and whether reference knowledge is consulted.

The dream cycle completes the cognitive model: during deep reasoning, frequently consulted reference knowledge can crystallize into proper motes — the system's equivalent of an insight becoming internalized understanding.

---

#### Story 13.1 — Ingest Reference Material into a Strata Corpus

**As a** user
**I want to** add reference documents (API docs, specs, guides, codebases) to a searchable corpus
**So that** large bodies of stable knowledge are queryable without polluting the nebula

**Acceptance Criteria:**

```gherkin
Given a file or directory of reference material
When I run `mote strata add <path> --corpus=<name>`
Then:
  - The content is chunked using a configurable strategy:
    - Markdown/text: heading-aware splitting (sections as natural chunks)
    - Code: function/class-level splitting
    - Default: sliding window with overlap
  - Each chunk is embedded and stored in .memory/strata/<corpus-name>/
  - A corpus manifest is written/updated: .memory/strata/<corpus-name>/manifest.json
    containing: corpus name, source paths, chunk count, created_at, last_updated
  - A anchor mote is automatically created (or updated if one exists for this corpus):
    - type: context
    - tags: inferred from content + corpus name
    - strata_corpus: <corpus-name>
    - body: auto-generated summary of what the corpus contains
    - weight: 0.3 (default for anchor motes; tunable)

Given the same path is added again to the same corpus
When `mote strata add` runs
Then existing chunks for that path are replaced (upsert, not duplicate)
And the manifest's last_updated timestamp is refreshed

Given I run `mote strata add <path> --corpus=<name> --no-anchor`
Then the corpus is indexed but no anchor mote is created
(for cases where the user wants manual control over graph integration)

Supported input formats:
  - Markdown (.md)
  - Plain text (.txt)
  - Code files (.py, .ts, .js, .rs, .go, .sh, etc.)
  - PDF (text extraction)
  - HTML (content extraction, tags stripped)
```

---

#### Story 13.2 — Anchor Mote Schema

**As a** nebula
**I want** anchor motes to carry metadata that connects the graph to strata corpora
**So that** the context loading pipeline knows when and how to consult reference material

**Acceptance Criteria:**

```gherkin
Given a anchor mote is created for a strata corpus
Then its frontmatter includes all standard mote fields plus:

  strata_corpus: "anthropic-api-docs"        # corpus name in .memory/strata/
  strata_query_hint: "authentication, streaming, tool use, models"
                                           # topics this corpus covers (aids query formulation)
  strata_query_count: 0                       # auto-incremented when this anchor triggers a strata query
  strata_last_queried: null                   # ISO timestamp of last strata query via this anchor

Given a anchor mote exists in the graph
Then it participates fully in scoring, linking, decay, and traversal
And it can be linked to other motes via standard link types
  (e.g., a lesson mote can have "informed_by: [anchor-mote-id]")

Given a anchor mote's strata_corpus references a corpus that no longer exists
When `mote doctor` runs
Then it flags the anchor as having a broken corpus reference

Given a anchor mote has strata_query_count > 0
Then its access patterns reflect actual use and contribute to retrieval scoring
```

---

#### Story 13.3 — Direct Strata Query

**As a** Claude session or user
**I want to** query a strata corpus directly
**So that** I can look up reference information without navigating the nebula

**Acceptance Criteria:**

```gherkin
Given a strata corpus exists in .memory/strata/<corpus-name>/
When I run `mote strata query <topic> [--corpus=<name>]`
Then:
  - A semantic similarity search is run against the corpus
  - The top K most relevant chunks are returned (K = strata.default_top_k from config)
  - Each result shows: chunk source file, section heading (if available),
    relevance score, and the chunk text
  - If --corpus is omitted, all corpora are searched and results are interleaved by score

Given the query matches a chunk with high relevance
When the result is returned
Then the corresponding anchor mote's strata_query_count is incremented
And the anchor mote's strata_last_queried is updated

Given no corpus exists
When I run `mote strata query <topic>`
Then a clear "no strata corpora found" message is returned with exit code 0

When I run `mote strata ls`
Then all corpora are listed with: name, chunk count, source paths, last updated,
  associated anchor mote ID (if any)
```

---

#### Story 13.4 — Context-Triggered Strata Augmentation (Hot Path)

**As a** Claude session loading context
**I want** the context pipeline to automatically query strata when a anchor mote scores high enough to be included
**So that** relevant reference material surfaces alongside experiential knowledge without requiring explicit strata queries

**Acceptance Criteria:**

```gherkin
Given `mote context <topic>` or `mote prime` runs graph traversal
When a anchor mote (strata_corpus field is non-null) appears in the scored result set
And the anchor mote's final score ≥ scoring.min_relevance_threshold
Then:
  - A strata query is automatically fired against the anchor's corpus
  - The query is formulated from: the original topic + the anchor's strata_query_hint
  - The top strata.context_augment_chunks results are appended to the context output
  - Results are clearly demarcated in the output:
    "📚 Reference (from <corpus-name>):" followed by compact chunk excerpts
  - The anchor mote's strata_query_count and strata_last_queried are updated
  - Total strata augmentation time is included in the 2-second performance budget

Given multiple anchor motes score above threshold in the same traversal
Then strata queries fire for each (up to strata.max_augment_corpora per context call)
And results from all corpora are merged and ranked by relevance

Given a anchor mote scores above threshold but the strata query returns no relevant chunks
Then no strata section is included in the output (fail silent, no noise)
And the anchor mote's strata_query_count is still incremented (the attempt is tracked)

Given strata.context_augment_enabled is false in config
Then anchor motes are included in context results as normal motes
But no automatic strata query is fired
(user can still query explicitly via `mote strata query`)
```

**Design Note — Nebula-Controlled strata:**
The nebula is the orchestrator. Strata corpora are never queried blindly or exhaustively — the nebula's scoring and traversal determine *whether* reference material is relevant to the current context. A anchor mote that has decayed (low recency factor) or been deprecated won't trigger a strata lookup even if the corpus still exists. This prevents reference material from overriding the nebula's judgment about what matters right now.

---

#### Story 13.5 — Session-Triggered Strata Query (Warm Path)

**As a** Claude session working on a task
**I want to** query strata mid-session when I encounter a question that reference material could answer
**So that** I can access deep reference knowledge during work, not just at context load time

**Acceptance Criteria:**

```gherkin
Given Claude is mid-session and encounters a question suited to reference material
When Claude (or the user) runs `mote strata query <topic>` during the session
Then:
  - The query runs against all corpora (or a specified corpus)
  - Results are returned inline for Claude to reason with
  - The corresponding anchor mote(s) have strata_query_count incremented
  - The query is logged to .memory/strata/query_log.jsonl:
    {
      "timestamp": "ISO",
      "query": "<topic>",
      "corpus": "<name>",
      "results_count": N,
      "top_chunk_score": 0.87,
      "session_context": "optional — what Claude was working on when it queried"
    }

Given a session ends (`mote session-end` runs)
When strata queries were made during the session
Then the session-end summary includes:
  "strata queries this session: N queries across M corpora"
And if a specific corpus was queried 3+ times in one session:
  "📚 <corpus-name> queried N times — consider if key insights should be motes"
```

**Design Note — Query Logging for Crystallization:**
Every strata query is logged not just for audit, but as a signal for the dream cycle. Repeated queries against the same corpus for the same topic suggest that the relevant knowledge is important enough to internalize. The dream cycle (Story 13.7) uses these logs to identify crystallization candidates — reference knowledge that should become proper motes.

---

#### Story 13.6 — Corpus Maintenance

**As a** user
**I want to** update, rebuild, and remove strata corpora
**So that** reference material stays current and storage doesn't grow unboundedly

**Acceptance Criteria:**

```gherkin
Given a strata corpus exists
When I run `mote strata update <corpus-name>`
Then:
  - Source paths from the manifest are re-read
  - Changed files are re-chunked and re-embedded
  - Deleted source files have their chunks removed
  - Unchanged files are skipped (hash comparison)
  - Manifest is updated with new chunk count and last_updated

When I run `mote strata rebuild <corpus-name>`
Then:
  - The entire corpus is deleted and rebuilt from source paths
  - Useful when the chunking strategy or embedding model changes

When I run `mote strata rm <corpus-name>`
Then:
  - The corpus directory .memory/strata/<corpus-name>/ is deleted
  - The associated anchor mote (if any) is flagged:
    status set to "deprecated", body updated with:
    "⚠ strata corpus '<name>' has been removed."
  - The anchor mote is NOT deleted (preserves link history and query stats)

When I run `mote strata stats`
Then output shows per-corpus:
  - Chunk count, total size, source file count
  - Query stats: total queries, queries in last 7/30/90 days
  - Top queried topics (from query_log.jsonl)
  - Associated anchor mote ID and its current score
```

---

#### Story 13.7 — Dream Cycle: Strata-to-Mote Crystallization

**As a** dream cycle
**I want to** analyze strata query patterns and identify reference knowledge that should crystallize into proper motes
**So that** frequently needed insights are internalized in the graph rather than requiring repeated strata lookups

**Acceptance Criteria:**

```gherkin
Given the dream cycle pre-scan stage runs
When .memory/strata/query_log.jsonl contains query records
Then the pre-scan identifies strata crystallization candidates:

  Candidate criteria (any of):
  - Same corpus + similar query topic queried N+ times across M+ distinct sessions
    (configurable: strata.crystallization.min_queries, strata.crystallization.min_sessions)
  - A single chunk appears in the top results for 3+ distinct queries
    (the same piece of reference knowledge keeps being relevant)
  - A corpus is queried in combination with the same mote(s) repeatedly
    (reference material and experiential knowledge are co-accessed)

Given crystallization candidates are identified
When the dream cycle batch reasoning processes them (Claude Sonnet)
Then for each candidate:
  - The frequently-retrieved chunk(s) are loaded
  - The LLM evaluates: "Is there an insight here that would benefit from being
    a first-class mote with links, decay, and salience — or is this better
    left as a reference lookup?"
  - If crystallization is recommended:
    - A draft mote is proposed with:
      type: lesson | decision | context (LLM-inferred)
      tags: inferred from chunk content + query context
      body: distilled insight (not raw chunk — the LLM extracts the actionable knowledge)
      informed_by: [anchor-mote-id] (link to the source corpus)
      origin: discovery
    - The proposal is written to visions_draft.jsonl with type: "strata_crystallization"
  - If the LLM judges the knowledge is better left in strata:
    - No proposal is generated
    - The candidate is logged as "evaluated, not crystallized" in the lucid log

Given the reconciliation pass processes strata_crystallization proposals
Then it checks:
  - Does a mote already exist that captures this insight? (deduplication)
  - Would the proposed mote contradict any existing motes?
  - Does the proposed mote naturally link to existing nebula nodes?
And adjusts confidence accordingly

Given a strata_crystallization proposal is accepted via `mote dream --review`
Then:
  - The new mote is created in .memory/nodes/
  - An `informed_by` link connects the new mote to the anchor mote
  - The query_log entries that triggered this crystallization are annotated:
    "crystallized_to: <mote-id>"
  - Future strata queries that would return the same chunk can note:
    "This knowledge has been internalized as mote <id>"
```

**Design Note — Consolidation Across the Boundary:**
This is the system's equivalent of reading a reference book so many times that the key insights become part of your own understanding. The reference book stays on the shelf (strata), but the lessons you've drawn from it are now in your head (motes), connected to your experiences, subject to evolution, and available without having to open the book again. The dream cycle is what makes this transition happen — unhurried, holistic evaluation of what's worth internalizing.

---

#### Story 13.8 — Dream Cycle: Strata Corpus Health Review

**As a** dream cycle
**I want to** review the health and relevance of strata corpora during maintenance
**So that** stale or unused corpora are flagged and anchor motes stay accurate

**Acceptance Criteria:**

```gherkin
Given the dream cycle runs
When strata corpora exist in .memory/strata/
Then the pre-scan evaluates corpus health:

  Stale corpus detection:
  - Corpus source files have been modified since last `strata update`
    → Flag: "Corpus '<name>' may be stale (source modified <date>)"
  - Corpus has not been queried in strata.staleness_threshold_days
    → Flag: "Corpus '<name>' unused for N days — consider removing"

  Anchor mote drift:
  - Anchor mote's strata_query_hint no longer matches actual query patterns
    (query_log shows queries on topics not in the hint)
    → Proposal: update strata_query_hint to reflect actual usage
  - Anchor mote's tags don't align with the corpus content
    → Proposal: update anchor mote tags

  Corpus overlap:
  - Two corpora have chunks with high embedding similarity
    → Flag: "Corpora '<A>' and '<B>' may overlap — consider merging"

Given health issues are found
Then they are included in the lucid log under a "strata_health" section
And proposals are written for actionable items (tag updates, hint updates)
And flags are surfaced in the final dream report
```

---

#### Story 13.9 — Embedding Provider Configuration

**As a** user
**I want to** configure which embedding model and backend is used for strata
**So that** I can use local embeddings for privacy/speed or remote embeddings for quality

**Acceptance Criteria:**

```gherkin
Given strata is configured in .memory/config.yaml
When `mote strata add` or `mote strata query` runs
Then the configured embedding provider is used

Supported providers:
  - Local: sentence-transformers via Python (no network required)
  - Local: Ollama embedding models (e.g., nomic-embed-text)
  - Remote: Claude API embeddings (if/when available)
  - Remote: OpenAI-compatible embedding endpoints

Given the embedding model is changed in config
When `mote strata rebuild <corpus>` is run
Then all chunks are re-embedded with the new model
And a warning is printed: "Embedding model changed — existing query
  results may differ. Consider rebuilding all corpora."

Given no embedding provider is configured
When `mote strata add` is run
Then a helpful error message explains how to configure one
And suggests the simplest option (Ollama with nomic-embed-text)
```

---

## Non-Functional Requirements

| Requirement | Spec |
|---|---|
| **No external dependencies** | `mote` CLI requires only bash, awk, python3 (stdlib). Dream cycle additionally requires Claude CLI with OAuth. Strata requires an embedding provider (Ollama recommended for zero-config local). |
| **No daemon or server** | All operations are stateless file reads/writes. Dream cycle is a one-shot process (cron/systemd/manual), not a daemon. |
| **Performance** | `mote prime` and `mote context` complete in < 2 seconds for < 500 motes (including strata augmentation if triggered). Dream cycle completes in < 10 minutes for < 500 motes. |
| **Git-friendly** | All files are plain text, mergeable with standard git tooling. Dream visions are JSONL. |
| **Portability** | Works on Linux, macOS; no OS-specific APIs |
| **Obsidian compatibility** | No Obsidian plugin required; standard vault format |
| **Atomicity** | index.jsonl writes are atomic (write to temp, rename); access_count updates are batched; dream visions are written atomically |
| **Resilience** | Malformed mote files produce warnings, not crashes. Dream cycle batch failures do not abort the pipeline — failed batches are logged and skipped. |
| **Tunability** | All scoring and dream parameters are configurable via `.memory/config.yaml` |
| **Visions, not mutations** | Dream cycle never modifies mote files directly. All changes are visions requiring explicit human/Claude review. |
| **Idempotency** | Running `mote dream` twice with no graph changes produces no new visions. |

---

## Mote Frontmatter Schema (Updated)

```yaml
id: proj-L1a2b3c4
type: lesson          # task | decision | lesson | context | question | constellation | anchor | explore
status: active        # active | deprecated | archived | completed
title: "OAuth token refresh requires x-api-key header"
tags: [oauth, anthropic, litellm, patch-required]
weight: 0.9           # 0.0–1.0, manually assigned importance
origin: failure       # normal | failure | revert | hotfix | discovery

# Retrieval metadata (auto-managed)
created_at: 2025-02-15T10:30:00Z
last_accessed: 2025-02-27T14:00:00Z
access_count: 7

# Planning links
depends_on: []
blocks: []

# Memory links
relates_to: [proj-C4d5e6f7]
builds_on: []
contradicts: []
supersedes: []
caused_by: []
informed_by: []

# Issue integration
source_issue: "issue-12"
crystallized_at: 2025-02-16T09:00:00Z

# Global promotion
promoted_to: null     # global-Lx9y8z if promoted

# strata integration (anchor motes only)
strata_corpus: null          # corpus name in .memory/strata/ (null for non-anchor motes)
strata_query_hint: null      # comma-separated topics this corpus covers
strata_query_count: 0        # auto-incremented when this anchor triggers a strata query
strata_last_queried: null    # ISO timestamp of last strata query via this anchor
```

---

## Configuration Schema

`.memory/config.yaml`:

```yaml
# --- Context Scoring (Hot Path) ---
scoring:
  edge_bonuses:
    builds_on: 0.3
    supersedes: 0.3
    caused_by: 0.2
    informed_by: 0.2
    relates_to: 0.1

  status_penalties:
    deprecated: -0.5

  recency_decay:
    tiers:
      - max_days: 7
        factor: 1.0
      - max_days: 30
        factor: 0.85
      - max_days: 90
        factor: 0.65
      - max_days: null    # 90+ or never
        factor: 0.4

  retrieval_strength:
    per_access_bonus: 0.03
    max_bonus: 0.15

  salience:
    failure: 0.2
    revert: 0.2
    hotfix: 0.2
    discovery: 0.1
    normal: 0.0
    explore_type_bonus: 0.1    # additional bonus for type=explore (expensive to reproduce)

  tag_specificity:
    weight: 0.2          # multiplied by avg specificity
    overload_threshold: 15

  interference:
    per_contradiction: -0.1

  max_results: 12
  max_hops: 2
  min_relevance_threshold: 0.25  # motes scoring below this are excluded even if budget remains

# --- Priming Signals (Extensible) ---
priming:
  signals:
    # Built-in signals (always active)
    - name: git_branch
      type: built_in
      description: "Current git branch name used as tag-match signal"

    - name: recent_files
      type: built_in
      description: "Files modified in last 30 min; paths parsed for keyword signals"

    - name: prompt_keywords
      type: built_in
      description: "Keywords extracted from user's initial prompt"

    # Dream-discovered signals (added via signal_discovery visions)
    # Example:
    # - name: co-access-oauth-litellm
    #   type: co_access
    #   trigger_tags: [oauth]
    #   boost_tags: [litellm]
    #   boost_amount: 0.15
    #   discovered_at: 2025-03-15T03:00:00Z

# --- Dream Cycle (Cold Path) ---
dream:
  schedule_hint_days: 2           # nudge after this many days without a dream run

  provider:
    batch:
      backend: claude-cli
      auth: oauth
      model: claude-sonnet-4-20250514
    reconciliation:
      backend: claude-cli
      auth: oauth
      model: claude-opus-4-20250514

  batching:
    strategy: hybrid              # hybrid | clustered | interleaved
    max_motes_per_batch: 10       # tunable based on model context window
    clustered_fraction: 0.6       # 60% of batches are tag-clustered (Phase A)

  reconciliation:
    enabled: true
    max_refetch_motes: 15         # limit on targeted mote re-reads during reconciliation

  pre_scan:
    link_candidate_min_shared_tags: 3       # min shared tags to qualify as link candidate
    staleness_threshold_days: 180           # days since last_accessed to flag as stale
    tag_overload_threshold: 15              # tag count above which to flag for splitting
    theme_growth_threshold_pct: 30          # % member growth to trigger constellation evolution review
    compression_min_words: 300              # min body word count to consider compression

  journal:
    max_tokens: 2000              # soft limit on journal size; prune weakest entries if exceeded

  interrupts:
    high_severity_mote_pct: 20    # % of processed motes affected to qualify as high-severity

# --- strata Integration ---
strata:
  chunking:
    strategy: heading-aware       # heading-aware | function-level | sliding-window
    max_chunk_tokens: 512         # max tokens per chunk
    overlap_tokens: 50            # overlap between sliding-window chunks

  retrieval:
    default_top_k: 5              # chunks returned per query
    min_relevance_score: 0.3      # chunks below this similarity threshold are excluded

  context_augment:
    enabled: true                 # whether anchor motes trigger automatic strata queries
    max_augment_corpora: 2        # max corpora queried per context/prime call
    chunks_per_corpus: 3          # chunks included per corpus in augmented output

  crystallization:
    min_queries: 5                # min total queries on similar topic to trigger candidate
    min_sessions: 3               # min distinct sessions querying the topic
    staleness_threshold_days: 180 # days since last query to flag corpus as unused
```

---

## File Structure — Dream Cycle and Strata

```
.memory/
├── nodes/                        # mote files (existing)
├── index.jsonl                   # edge index (existing)
├── config.yaml                   # configuration (existing)
├── constellations.jsonl          # constellation cluster records (existing)
├── dream/
│   ├── visions.jsonl             # final reconciled visions (pending review)
│   ├── visions_draft.jsonl       # pre-reconciliation visions (consumed by reconciliation)
│   ├── lucid.json                # last lucid log (preserved for debugging/audit)
│   └── log.jsonl                 # append-only log of all dream runs
└── strata/
    ├── query_log.jsonl           # append-only log of all strata queries (drives crystallization)
    └── <corpus-name>/            # one directory per corpus
        ├── manifest.json         # corpus metadata: sources, chunk count, timestamps
        ├── chunks.jsonl          # chunked document content
        └── bm25.json             # BM25 search index
```

---

## Addendum: Post-v0.1 Enhancements

The original epics above document the foundational design. The following enhancements were implemented in subsequent releases and extend the system beyond the original 13 epics.

### v0.1.7 — Soft Delete

- `mote delete <id>` soft-deletes by moving the mote file to `.memory/trash/` and setting `deleted_at` timestamp
- `mote trash list` shows trashed motes with deletion timestamps
- `mote trash restore <id>` moves a mote back to `.memory/nodes/`
- `mote trash purge` permanently removes trashed motes
- `mote feedback <id> useful|irrelevant` provides explicit retrieval feedback

### v0.2.0 — Hierarchical Planning

- **Parent/child mote relationships** via `parent` field in frontmatter and `mote plan <parent-id> --child "..." [--sequential]`
- **Acceptance criteria tracking** via `acceptance` (list of strings) and `acceptance_met` (parallel list of booleans) fields; `mote check <id> [index] [--all]` marks criteria met
- **Effort sizing** via `size` field (xs|s|m|l|xl) set with `--size` flag on `mote add`
- **Completion tracking** via `mote progress <parent-id>` showing child status, acceptance criteria, and overall completion percentage
- **Sequential dependency chaining** via `--sequential` flag on `mote plan`, which auto-creates `depends_on` links between consecutive children

### v0.3.0 — Beads Feature Transfer

- **JSONL import/export** via `mote export` and `mote import`
  - Content-hash deduplication on import (SHA256 of type + title + body)
  - Filter flags matching `mote ls` (--type, --tag, --status)
  - `--output` flag for file output, `--dry-run` for preview
- **Structured external references** via `ExternalRef` type (provider/ID/URL triples in YAML frontmatter)
  - `--ref provider:id[:url]` flag on `mote add`
  - Ref IDs are indexed in BM25 search
- **`--json` flag** on 7+ commands (ls, show, pulse, search, stats, tags, dream --review) for machine-readable output
- **Content-hash prescanner cache** at `.memory/dream/scan_state.json`
  - SHA256 of serialized motes for change detection
  - Incremental scan skips unchanged motes between dream runs
  - Cache auto-prunes entries for deleted motes
- **Cluster summarization** in dream cycle
  - Detects 5+ completed motes with 2+ shared tags (tag-pair grouping)
  - Excludes already-summarized motes (those linked via `builds_on` from context motes)
  - `summarize` vision type creates a context mote with `builds_on` links and archives source motes
- **Quick capture** via `mote quick "your thought here"` (auto-typed, no editor)

---

## Implementation Phases (Backlog Priority Order)

| Phase | Stories | Deliverable |
|---|---|---|
| Phase | Stories | Deliverable | Status |
|---|---|---|---|
| **0: Format + First Mote** | 1.1, 1.2 | Storage root decided; first mote created manually; format validated with new frontmatter fields | Done |
| **1: MVP CLI** | 1.1-1.4, 2.1, 3.1-3.2, 7.1 | `mote add/show/link/unlink/ls/index rebuild` working; access tracking live | Done |
| **2: Migration** | 9.1 | Project MEMORY.md fully migrated; origin fields set; MEMORY.md.archived | Done |
| **3: Context Loading** | 4.1-4.4 | `mote context` and `mote prime` with full scoring formula; ambient priming signals | Done |
| **4: Issue Bridge** | 6.1-6.2 | `mote crystallize` working; automatic salience classification on crystallization | Done |
| **5: Nebula Health** | 7.2, 4.5 | `mote doctor` with contradiction and tag audit checks; `mote tags audit` | Done |
| **6: Constellation Discovery** | 5.1-5.3, 8.1-8.2 | `mote constellation synthesize`; Obsidian graph view working | Done |
| **7: Global Layer** | 10.1 | `mote promote`; cross-project memory active | Done |
| **8: Retrieval Adaptation** | 11.1-11.3, 4.6 | Access pattern tracking; salience automation; contradiction surfacing; `mote stats` dashboard | Done |
| **9: Dream Cycle MVP** | 12.1, 12.2, 12.3, 12.6, 12.7 | `mote dream` pipeline with lucid log, hybrid batching, vision review; manual + cron invocation | Done |
| **10: Dream Reconciliation** | 12.4, 12.5 | Reconciliation pass (Opus); interrupt mechanism; cross-batch coherence | Done |
| **11: Session Bookends** | 12.8 | `mote session-end` warm path; access flush, crystallization prompts, lightweight link suggestions | Done |
| **12: Strata Core** | 13.1, 13.2, 13.3, 13.6, 13.9 | `mote strata add/query/ls/update/rm/stats`; anchor motes; BM25 search | Done |
| **13: Strata-Nebula Integration** | 13.4, 13.5 | Context-triggered strata augmentation (hot path); session strata queries with logging (warm path) | Done |
| **14: Strata Crystallization** | 13.7, 13.8 | Dream cycle strata-to-mote crystallization; corpus health review | Done |
| **15: Hierarchical Planning** | — | Parent/child tasks, acceptance criteria, effort sizing, `mote plan/progress/check` (v0.2.0) | Done |
| **16: Beads Feature Transfer** | — | JSONL import/export, external refs, `--json` flags, scan cache, cluster summarization (v0.3.0) | Done |
| **17: Lens Mode** | — | ML-1 through ML-5: lens mode architecture, 9 mental model lens prompts, cross-lens reconciliation, `CrossLensAgreement` confidence scoring, per-lens quality observability, vision provenance in review (v0.4.7) | Done |

---

## Critical Files

- `~/.local/bin/mote` — CLI binary
- `<project>/.memory/nodes/` — project-local mote storage
- `<project>/.memory/config.yaml` — scoring, dream, and strata configuration
- `<project>/.memory/index.jsonl` — edge index (cache, rebuilt from motes)
- `<project>/.memory/dream/` — dream cycle artifacts (visions, lucid log, run history)
- `<project>/.memory/strata/` — strata corpora and query log
- `~/.claude/memory/nodes/` — global mote storage (via `mote promote`)
- `<project>/CLAUDE.md` — project instructions, updated by `mote init`/`mote onboard`
- `~/.claude/CLAUDE.md` — global instructions, references `mote prime` at session start
