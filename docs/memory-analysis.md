# Memory Analysis: Beads (`bd`) vs. Motes (`mote`)

A deep comparative analysis of two AI-native persistence systems.

- **Beads** — `gastownhall/beads` ([github.com/gastownhall/beads](https://github.com/gastownhall/beads)),
  v1.0.3 (2026-04-24), Steve Yegge et al., 23k stars in ~7 months.
  *"Distributed graph issue tracker for AI agents, powered by Dolt."*
- **Motes** — this repository, v0.4.17 (2026-05-06), MIT.
  *"AI-native context and memory system. Atomic units linked in two dimensions
  (planning + memory)."*

Date of analysis: **2026-05-06**. The motes repo already contains a
one-directional companion document (`recommendation.md`, 971 lines) cataloging
beads patterns to adopt; this report is intentionally **bidirectional and
analytical**, not prescriptive.

---

## 1. Executive Summary

Beads and motes occupy **adjacent but genuinely different niches**, despite
both being marketed as agent memory:

- **Beads is an issue tracker that grew into agent memory.** Every artifact is
  an `Issue` row in a Dolt SQL database. Workflow primitives (molecules,
  swarms, gates, wisps) and 17 dependency types are layered on top. Memory is
  a thin `bd remember/recall/forget` k/v facade over the same SQL backend.
  Multi-writer correctness is its calling card.
- **Motes is a knowledge graph that grew a task tracker.** Every artifact is a
  markdown file with YAML frontmatter, linked in two dimensions (planning vs.
  semantic). Tasks are one of eight types; the others (`decision`, `lesson`,
  `explore`, `question`, `context`, `constellation`, `anchor`) are first-class
  knowledge. A headless "dream cycle" continually proposes link inference,
  contradictions, merges, and crystallizations. Single-writer simplicity and
  zero-network operation are its calling cards.

Both ship as a single Go binary, both wrap themselves in agent-instruction
files (CLAUDE.md / AGENTS.md / GEMINI.md / CODEX.md), and both inject context
at session start (`bd prime` / `mote prime`). Past that surface symmetry, the
designs diverge sharply.

| Axis | Beads (`bd`) | Motes (`mote`) |
|---|---|---|
| **Mental model** | Distributed issue tracker | Two-dimensional knowledge graph |
| **Backend** | Dolt (versioned SQL, CGO) | Markdown + YAML files |
| **Multi-writer** | Native (cell-level merge, push/pull) | Single-writer per `.memory/` |
| **Search** | SQL `LIKE` + structured filters | BM25 (zero deps) over bodies + strata corpora |
| **Semantic ops** | None native (compaction is summarization only) | Dream cycle: link inference, contradiction, merge, crystallization, lens triangulation |
| **Item types** | `bug \| feature \| task \| epic \| chore \| decision \| message \| molecule` | `task \| decision \| lesson \| context \| question \| constellation \| anchor \| explore` |
| **Link types** | 17 (`blocks`, `parent-child`, `discovered-from`, `replies-to`, `relates-to`, `duplicates`, `supersedes`, etc.) | 8 (`depends_on/blocks`, `relates_to`, `builds_on`, `contradicts`, `supersedes`, `caused_by`, `informed_by`) |
| **Cross-project** | Per-project DBs + peer federation; no global memory | First-class: `mote promote` → `~/.motes/`, type-routed defaults, generated `MOTES.md` |
| **Multi-agent atomicity** | `bd update --claim` (atomic CAS) | Tag-only attribution via `MOTE_AGENT_ID`; no atomic claim |
| **CLI commands** | ~277 `Use:` declarations across `cmd/bd/` | ~30 top-level commands |
| **Agent integrations** | 10+ (claude/codex/gemini/factory/mux/cursor/windsurf/cody/kilocode/aider/opencode/junie) | 3 (claude/codex/gemini) |
| **Distribution channels** | Homebrew, npm, PyPI (MCP), AUR, winget, Nix, scripts | `go install`, source build |
| **Workflow primitives** | Molecules, swarms, gates, wisps, formulas | Plan/progress, constellations, strata, dream visions |
| **External deps** | Dolt (CGO), MCP server in Python | `cobra` + `yaml.v3`, stdlib |
| **License** | MIT | MIT |
| **Stars / age / latest** | 23,287 stars / ~7 months / v1.0.3 | private repo / pre-1.0 / v0.4.17 |

---

## 2. Conceptual Model

The deepest difference is **what an "atom" is**.

### Beads: the universal Issue

A single Go struct (`internal/types/types.go`, 100+ fields) handles regular
issues, epics, "molecules" (workflow templates), "wisps" (ephemeral inter-agent
messages), gates (async coordination), and events (state changes).
Discrimination is by `IssueType` and a constellation of flag fields
(`Pinned`, `IsTemplate`, `Ephemeral`, `NoHistory`, `MolType`, `WorkType`,
`EventKind`). A "memory" is a row in a `config` k/v table, not an issue.

This is **OOP-style polymorphism by convention**: one row shape, runtime
discrimination. It is operationally simple (one storage path), schema-evolves
through nullable columns, and merges cleanly under Dolt. The cost is
conceptual sprawl — a reader of the type file has to understand 8 issue types
× ~6 axis flags before they know what any given row means.

### Motes: typed knowledge atoms

Eight enumerated types (`internal/core/valid_enums.go`):

```go
ValidTypes = []string{"task", "decision", "lesson", "context", "question",
                       "constellation", "anchor", "explore"}
```

Each type has its own purpose, its own promotion behavior (decisions,
lessons, explores, questions auto-promote to global; tasks/contexts stay
local), and its own role in dream-cycle vision generation (e.g., the
`action_extraction` vision only fires on lessons). Behavior derives from the
type, not from runtime flags.

The trade-off mirrors statically-typed vs. dynamically-typed languages:
beads gets storage uniformity and runtime flexibility; motes gets clearer
contracts and easier semantic reasoning.

### Implications

- **Discoverability.** Asking *"what kinds of things live here?"* of beads
  requires reading a SKILL.md plus the docs to disentangle epics from
  molecules from wisps from messages. Motes' eight-type list is enumerable.
- **Knowledge as first-class.** A "lesson learned" in beads is just an issue
  with status=closed and a `decision` type tag. In motes, `lesson` is a type
  with auto-promotion, with action-extraction visions, and with retrieval
  weighting. The `mote search "<error>" --type=lesson` workflow has no
  beads equivalent more direct than `bd search` + LIKE.
- **Tasks as one type, not the type.** Motes positions tasks as one
  dimension of knowledge work; beads positions everything (including
  knowledge) as a dimension of task work.

---

## 3. Storage Architecture

### Beads: Dolt-backed SQL with cell-level merge

Two modes:

- **Embedded** (default `bd init`): Dolt runs in-process via `dolthub/driver`.
  Data in `.beads/embeddeddolt/`. Single-writer (file locking). CGO required;
  the `gms_pure_go` build tag is mandatory.
- **Server** (`bd init --server`): an external `dolt sql-server` over TCP/UDS.
  Multi-writer. Configurable via `BEADS_DOLT_SERVER_*`. A "shared-server" mode
  hosts every project on one Dolt instance at `~/.beads/shared-server/`.

Schema: SQL tables `issues`, `dependencies`, `labels`, `comments`, `events`,
`wisps` (dolt-ignored), `blocked_issues_cache` (a materialized cache —
25× speedup over recursive CTE for `bd ready`), and `config` (k/v backing
`bd remember`).

**Versioning is the killer feature.** Every write auto-commits to Dolt
history. `bd dolt push|pull` does native VCS-style sync between repos.
Cell-level merge resolves conflicts automatically. IDs are content-hashed
(`bd-a1b2`), birthday-paradox-aware (`docs/COLLISION_MATH.md`), so
concurrent issue creation across branches no longer collides.

The cost: the most-commented open issue at fetch time is *"Moving to dolt
pretty much made beads unusable for me"* (19 comments). Several others target
Dolt server mode under restart. Build complexity (CGO, mandatory tags,
custom `make install`) is non-trivial.

### Motes: markdown files + JSONL caches

```
.memory/
├── nodes/*.md              # one mote per file (YAML frontmatter + body)
├── index.jsonl             # edge index + tag stats footer (cache, regenerable)
├── audit.jsonl             # append-only mutation log
├── mote_bm25.json          # BM25 index over titles + bodies + refs
├── config.yaml
├── constellations.jsonl
├── trash/                  # soft-deletes (restorable via `mote trash restore`)
├── dream/
│   ├── visions.jsonl       # finalized dream visions awaiting review
│   ├── visions_draft.jsonl # raw stage-1 output (pre-reconciliation)
│   ├── lucid_log.json
│   ├── log.jsonl
│   ├── feedback.jsonl
│   └── auto_applied.jsonl
└── strata/<corpus>/
    ├── manifest.json
    ├── chunks.jsonl
    └── bm25.json
```

Three invariants from `AGENTS.md`:

1. All writes use `core.AtomicWrite` (write-temp + rename — POSIX atomic).
2. Reads never mutate. Access counts batch into `.access_batch.jsonl`,
   flushed at session end.
3. The edge index is a **cache**. `mote index rebuild` regenerates it from
   frontmatter — fully self-healing.

The cost: no native multi-writer story. Concurrent agents on the same
`.memory/` rely on the OS rename atomicity but have no merge semantics for
overlapping writes to the same node. Cross-machine sync is whatever git
gives you (which is fine for solo workflows, less fine for a swarm of
parallel agents).

### Trade-off summary

| | Beads (Dolt) | Motes (files) |
|---|---|---|
| Multi-writer | Native (cell-level merge) | OS rename atomicity only |
| Sync | `dolt push/pull`, federation, MCP routing | Git or rsync |
| Audit history | Free, native, queryable as SQL | `audit.jsonl` append-only log |
| Operational simplicity | CGO + Dolt server lifecycle | Cat a file in your editor |
| Recovery | Dolt restore, JSONL export | `mote index rebuild` |
| Inspect-by-eye | `bd show`, JSON dumps | Open the .md file |
| Embeddings/FTS5/BM25 | None | BM25 (file: `mote_bm25.json`) |
| Backup story | `bd backup init`, DoltHub, JSONL export | Copy `.memory/` |

The honest reading: beads pays measurable operational cost for
correctness guarantees that solo or small-team workflows rarely need;
motes is bare-metal-simple and trades multi-writer correctness for
zero-config setup.

---

## 4. CLI Surface

Beads' CLI is **massive**: ~277 `Use:` declarations across `cmd/bd/`,
spanning issue lifecycle, dependencies, dolt sync, federation, external
tracker integrations (GitHub, GitLab, Linear, Jira, Azure DevOps, Notion),
and a chemistry-themed workflow vocabulary (`mol`, `pour`, `bond`, `burn`,
`distill`, `cook`, `formula`, `wisp`, `swarm`, `gate`, `ship`).

Motes' CLI is **roughly 30 top-level commands**, organized around lifecycle
(`prime`, `session-end`, `onboard`), CRUD (`add`, `update`, `show`,
`delete`, `trash`), retrieval (`ls`, `pulse`, `search`, `context`), planning
(`plan`, `progress`, `check`), graph (`link`, `unlink`, `constellation`,
`crystallize`), strata (`strata add|query|ls`), and the dream cycle
(`dream`, `promote`, `doctor`, `stats`).

Beads' breadth is genuinely useful — sync to Linear, run a swarm across
ready waves, route via federation peers — but it carries a cognitive load
that motes' deliberately smaller surface avoids. The recommendation
document at `recommendation.md` §4 specifically calls out *"approaching
~30 top-level commands — discoverability concern,"* signaling that even
mote's smaller surface is hitting friction.

**Output ergonomics** are notable in both:

- Beads: every command supports `--json` (load-bearing convention),
  Unicode status icons (`○ ◐ ● ✓ ❄`), explicitly forbids emoji-style
  `🔴🟠🟡` in the docs, semantic colors via lipgloss.
- Motes: `--json`, `--compact`, TTY detection. The recommendation document
  flags missing `--plain`, `--explain`, `--short/--long` modes (rec §23.7–10),
  meaning motes' CLI ergonomics still trail beads by a measurable gap.

---

## 5. AI Workflow Integration

This is where both projects most clearly converge on the same vision (inject
context at session start, expose CLI as the agent's tool surface) and most
clearly differ in execution.

### Beads: recipe-based installer for ~10 agents

`bd setup <recipe>` is the entry point. Supported recipes include `claude`,
`gemini`, `codex`, `factory`, `mux`, `cursor`, `windsurf`, `cody`,
`kilocode`, `aider`, `opencode`, `junie`. Profiles are explicit: `minimal`
for hook-aware tools (~60% smaller, points at `bd prime`), `full` for tools
that read AGENTS.md as their primary integration surface.

The integration content is **machine-managed between markers**:

```
<!-- BEGIN BEADS INTEGRATION profile:full hash:d4f96305 -->
...
<!-- END BEADS INTEGRATION -->
```

A content hash drives freshness detection and safe upgrades. A bundled
plugin lives at `plugins/beads/` with a `SKILL.md` (`version: 0.60.0`,
`allowed-tools: Read,Bash(bd:*)`) and 28 sub-command markdown files.
Custom recipes are extensible via `bd setup --add myeditor .myeditor/rules.md`.

Position on Skills: officially, beads ships **without** Claude Skills,
arguing in `docs/CLAUDE_INTEGRATION.md` that *"MCP tool schemas can add
10–50k tokens to context. `bd prime` adds ~1–2k tokens of workflow context.
That's 10–50× less context overhead."* The bundled plugin breaks this
position by shipping a SKILL.md anyway.

### Motes: unified install for 3 agents, deep integration each

`mote onboard` is the entry point — a single ~1500-line file
(`cmd/mote/cmd_onboard.go`) handles:

- Source detection (beads `.beads/issues.jsonl`, legacy `MEMORY.md`,
  GitHub via `gh`)
- Interactive prompts (or `--from`)
- Scaffolding `.memory/`
- Migration with rebuild
- Updating the agent's instruction file (`CLAUDE.md`, `AGENTS.md`,
  `GEMINI.md`)
- **Migrating legacy `bd prime` / `bd sync` hooks to mote equivalents**
  (motes literally has explicit beads-migration code)
- Installing hooks across all detected agents
- Installing four Skills: `mote-capture`, `mote-retrieve`, `mote-plan`,
  `mote-subagent` (embedded in the binary via `skills/embed.go`)

The agent table:

| Agent | Config | Hooks | Skills | Instructions |
|---|---|---|---|---|
| Claude Code | `~/.claude/` | `~/.claude/settings.json` | `~/.claude/skills/` | `CLAUDE.md` |
| Codex | `~/.codex/` (auto / `--codex`) | `~/.codex/hooks.json` + `[features] codex_hooks=true` | `~/.agents/skills/` | `AGENTS.md` |
| Gemini CLI | `~/.gemini/` (auto / `--gemini`) | `~/.gemini/settings.json` | `~/.agents/skills/` | `GEMINI.md` (imports `@AGENTS.md`) |

Motes embraces Skills as a first-class integration point; beads ships them
half-heartedly while arguing they're redundant.

### Trade-off

Beads wins on **breadth** (10+ agents). Motes wins on **depth per agent** —
hooks, skills, and instruction files are coordinated into a single coherent
install, with explicit migration from beads built in.

For a team standardizing on Claude/Codex/Gemini, motes' integration is
markedly more polished. For a team using Aider or Cursor, beads is the only
option.

---

## 6. Hooks and Lifecycle

### Beads

`bd setup claude` installs two Claude hooks:

- `SessionStart` → `bd prime` (~1–2k token context injection)
- `PreCompact` → `bd prime` (preserve context across compaction)

`bd prime` has its own ADR (`adr/0001-bd-prime-as-source-of-truth.md`),
auto-detects MCP vs CLI mode, supports three layers of `PRIME.md` override
(clone-local, workspace, `~/.config/beads/PRIME.md` global), and emits a
"🚨 SESSION CLOSE PROTOCOL 🚨" checklist at the end of its output.
**There is no symmetric session-end hook in the standard recipes** —
the close protocol is enforced via instructions injected into context,
relying on the agent to comply. The repo also has its own
`.claude/hooks/{block-gh-watch.sh, block-interactive-cmds.sh}` PreToolUse
defensive guards used while dogfooding.

### Motes

Motes wires **seven** Claude hooks:

```go
SessionStart/startup → mote prime --hook --mode=startup
SessionStart/resume  → mote prime --hook --mode=resume
SessionStart/compact → mote prime --hook --mode=compact
SessionStart/clear   → mote prime --hook --mode=startup
PreCompact           → mote prime --hook --mode=compact
UserPromptSubmit     → mote prompt-context
Stop                 → mote session-end --hook
```

Plus a soft-warning **`.git/hooks/pre-commit`** that whispers when no
active task mote exists, and the equivalent Codex/Gemini hook sets (Gemini
gets `300000ms` timeouts so its default 60s doesn't kill the
session-end flush).

The `MOTE_AGENT_KIND=…` env-var prefix on every hook line is what enables
the **per-agent provider override** layer in `LoadConfig`
(`docs/configuration.md`) — different agents can use different LLM backends
for their own dream cycles.

### Trade-off

Motes' hook coverage is **strictly greater** — both more events and a real
session-end pipeline, vs. beads' inject-instructions-and-trust approach.
Beads' PreToolUse safety hooks (block `gh run watch` API quota burn, block
unflagged `rm`) are excellent footgun protection; the recommendation
document calls these out as the #1 highest-impact pattern to adopt
(rec §1).

---

## 7. Linking and Graph Features

### Beads: 17 dependency types

`DepBlocks, DepParentChild, DepConditionalBlocks, DepWaitsFor, DepRelated,
DepDiscoveredFrom, DepRepliesTo, DepRelatesTo, DepDuplicates, DepSupersedes,
DepAuthoredBy, DepAssignedTo, DepApprovedBy, DepAttests, DepTracks, DepUntil,
DepCausedBy, DepValidates, DepDelegatedFrom`

Only `blocks`, `parent-child`, `conditional-blocks`, `waits-for` gate the
ready queue. `discovered-from` is the signal-graph link agents are taught to
use when they find new work mid-task. Hierarchical IDs (`bd-a3f8.1.1`)
provide path-syntax traversal. Commands: `dep tree`, `dep cycles`, `graph`,
`graph check`, `swarm` (ready-front analysis with wave numbering for
parallel agent execution).

### Motes: 8 link types in two dimensions

| Type | Dimension | Behavior |
|---|---|---|
| `depends_on` / `blocks` | Planning | Inverse pair |
| `relates_to` | Memory | Symmetric (writes both sides) |
| `builds_on` | Memory | Reverse-indexed via `built_by_ref` |
| `contradicts` | Memory | Symmetric, scored as interference |
| `supersedes` | Memory | **Auto-deprecates target** (sets `deprecated_by`) |
| `caused_by` | Memory | Directional |
| `informed_by` | Memory | Directional |

Traversal is BFS with hop-limited spreading activation
(`internal/core/traversal.go`). Each visited node is scored via
`ScoreEngine` with per-link-type `EdgeBonuses`. Defaults: `max_hops=2`,
`max_results=12`, `min_relevance_threshold=0.25`.

`mote context <topic>` does seed selection (topic keywords vs. tags +
ambient signals: git branch, recent files, prompt keywords) then BFS.
`mote context --planning <id>` walks the dependency chain.
`mote constellation` surfaces tag-cluster overviews.

### Comparison

Beads has **more link types** but uses fewer in practice — only ~4
materially affect workflow. Motes has fewer types but each is
**operationally meaningful** (`supersedes` auto-deprecates,
`contradicts` scores as interference in retrieval, `builds_on` informs
ranking).

The big asymmetry is **graph traversal as retrieval**. Motes treats the
graph as a primary retrieval mechanism (BFS with relevance scoring is the
default for `mote context`). Beads treats the graph as workflow
metadata; retrieval is SQL queries. This reflects the deeper conceptual
split (knowledge graph vs. issue tracker).

The recommendation document (§8) flags one missing motes link type:
`discovered-from`, beads' non-blocking provenance link for "I noticed this
while doing something else."

---

## 8. Search and Retrieval

### Beads

SQL `LIKE` against Dolt + structured filters (`status`, `priority`, `type`,
`label`, `assignee`, deferred/due windows). **No FTS5/BM25/vector index
in evidence.** Memory search (`bd memories <kw>`) is plain substring match
over the k/v config table.

For an "AI memory" tool, this is a noticeable gap — agents must know exact
terms or rely on tag filtering. Time-based filters are first-class:
`--deferred`, `--defer-before`, `--defer-after`, `--due-before`,
`--due-after`, `--overdue`.

### Motes

BM25 is the only retrieval algorithm — explicitly justified in
`docs/architecture.md`:

> "Embedding-based semantic search requires infrastructure: either a local
> model runtime ... or a remote API with keys and network access. Both create
> adoption friction that conflicts with the system's 'single binary, zero
> config' philosophy."

The same ~150-LOC BM25 (`internal/strata/bm25.go`) powers both project-wide
mote search (`mote_bm25.json`) and external strata corpora. Strata is a
distinct subsystem for ingesting external `.md|.txt|.go|.py|.ts|...` files
into chunked, BM25-indexed local corpora. Three chunkers:
heading-aware (markdown), function-level (code regex),
sliding-window (fallback).

Every strata query is logged for the dream cycle to identify
**crystallization candidates** — chunks queried frequently across sessions
(thresholds: `min_queries=5`, `min_sessions=3`) get promoted to permanent
motes. This is a clean feedback loop with no beads equivalent.

### Comparison

| | Beads | Motes |
|---|---|---|
| Algorithm | SQL `LIKE` | BM25 |
| External corpora | None | Strata layer |
| Time filters | First-class (`--due-*`, `--defer-*`) | Absent (rec §5) |
| Frequency feedback | None | Strata → crystallization |
| Vector / embeddings | None | None (deliberate) |

Motes wins on **retrieval quality and external-corpus support**. Beads
wins on **temporal queries** — the missing time model in motes is rec §5
and is genuinely felt for date-sensitive task work.

---

## 9. Maintenance and Evolution

### Beads: conventional cleanup, sophisticated compaction

Maintenance commands: `doctor` (with `health|validate|fix|pollution|conventions|artifacts|agent|gastown_guard`), `compact`, `find-duplicates`, `gc`, `prune`, `purge`, `cleanup`, `flatten`, `lint`, `preflight`, `stale`, `orphans`, `config drift`, `config apply`.

`bd compact` has three modes: **analyze** (export candidates), **apply**
(accept agent-provided summary), **auto** (legacy AI-driven via
`ANTHROPIC_API_KEY`). Two tiers: 30-day/70%-reduction,
90-day/95%-reduction. Plus `--dolt` for Dolt GC. **Compaction is
explicitly lossy and permanent** — *"original content is discarded."*

### Motes: the dream cycle

`mote dream` is a four-stage pipeline:

1. **Pre-scan** (deterministic, no LLM) — find candidates: link
   suggestions (≥3 shared tags), contradictions, overloaded tags,
   stale motes (>180d), constellation evolution, compression candidates
   (>300 words), uncrystallized issues, strata crystallization, signal
   patterns, **merge clusters**, **summarization clusters**.
2. **Batch reasoning** (Sonnet/equivalent, 50 motes/batch, 60% tag-clustered + 40% interleaved) — emit draft visions + lucid-log updates.
3. **Reconciliation** (Opus/equivalent) — merge drafts across batches into final visions.
4. **Review** — `mote dream --review` is a TUI (`accept|edit|reject|defer`).

Vision types: `link_suggestion`, `contradiction`, `tag_refinement`,
`staleness`, `compression`, `signal`, `merge_suggestion`, `action_extraction`,
`decompose_suggestion`, `summarize`. The `merge_suggestion` vision
auto-deprecates merged motes via `supersedes`. The `action_extraction`
vision adds prescriptive `Action:` fields to lessons/decisions.

**Five backends** dispatched through an `Invoker` interface
(`internal/dream/invoker.go`):

| Backend | Auth |
|---|---|
| `claude-cli` (default) | OAuth via `claude` binary |
| `openai` | API key |
| `gemini` (Vertex AI) | ADC via `gcloud auth print-access-token` |
| `codex-cli` | Inherits `codex login` |
| `gemini-cli` | Inherits `gemini` CLI auth |

**Lens mode** (v0.4.7+): replaces self-consistency voting (N identical
runs) with N runs that each use a **distinct mental-model prompt** —
`structural`, `survivorship_bias`, `feedback_loops`, `confirmation_bias`,
`inversion`, `probabilistic`, `first_principles`, `opportunity_cost`,
`occams_razor`. Visions flagged by 2+ lenses earn `CrossLensAgreement` —
a stronger confidence signal than identical-prompt voting. This pattern
appears genuinely novel.

### Comparison

| | Beads | Motes |
|---|---|---|
| Compaction | Lossy, permanent | Compression vision (proposed, reviewable) |
| Dedup | `find-duplicates` (mechanical + AI) | `merge_suggestion` vision |
| Link inference | None | First-class |
| Contradiction detection | None | First-class |
| Action extraction | None | First-class |
| LLM backends | One (Anthropic) | Five |
| Cost reporting | Basic | Per-stage with pricing table |
| Quality triangulation | None | Lens mode |

This is the most asymmetric category. Motes' dream cycle is **dramatically
more sophisticated** as a knowledge-evolution engine. Beads' compaction is
straightforward summarization-and-discard.

The cost: motes' dream cycle is opt-in compute that requires reviewer
attention. Beads' approach is "delete old stuff" — simple, predictable,
permanent.

---

## 10. Multi-Agent Coordination

### Beads: built for swarms

- **Atomic claim**: `bd update <id> --claim` is a compare-and-swap that
  sets `assignee` and `in_progress` together. Critical for racing agents
  against the same ready queue.
- **Inter-agent messaging**: `IssueType="message"` with `Sender`,
  `replies-to`, `Ephemeral`, `NoHistory`, threading. The `wisps` table is
  dolt-ignored (local-only).
- **Worktrees**: `bd worktree`, with worktree-aware hooks paths,
  fingerprints, doctor checks.
- **Swarm**: `bd swarm` does ready-front analysis across an epic to
  compute `MaxParallelism` and "waves" for parallel agent execution.
- **Work types**: `mutex` (default) vs. `open_competition` per Decision 006.
- **Cross-tool compatibility**: a single Dolt DB is visible regardless of
  which agent edits it. `compatible-with: [claude-code, codex]` in the
  bundled SKILL.

### Motes: tag-only attribution, no atomicity

- `MOTE_AGENT_ID=subagent-<purpose>` env var tags writes for traceability.
- `MOTE_AGENT_KIND` (set by hooks) drives per-agent provider config
  overrides.
- `mote-subagent` skill enforces a contract: subagents do NOT create task
  motes (only the parent does), do NOT run `mote prime` or `mote session-end`,
  must capture findings before returning.
- **No atomic claim.** `mote ls --ready` returns a list; two agents can
  both grab the top item and write conflicting `in_progress` updates,
  with last-write-wins.

### Comparison

Beads is genuinely built for swarms; motes is built for one
agent-with-subagents. The recommendation document calls out the missing
atomic `--claim` (rec §7) and the missing `discovered-from` provenance
link (rec §8) as the highest-priority gaps to close for multi-agent work.

For solo or small-team workflows, motes' simpler model is fine. For
parallel agent fleets working a shared backlog, beads' design is
substantially safer.

---

## 11. Cross-Project Knowledge

### Beads: per-project, federated

Each `.beads/` directory is its own Dolt database. Cross-project
mechanisms exist but are weak:

- `bd promote <wisp-id>` promotes an ephemeral wisp to a permanent bead
  **within the same project** — not to a global layer.
- `bd remember "insight"` stores per-project memory in the local config
  table.
- v1.0.1 added a `beads_global` shared-server database "for cross-project
  state" + a global `~/.config/beads/PRIME.md` override.
- "Federation" exists (`bd federation add-peer`, `sync`) but is
  peer-to-peer DB sync, not a global memory tier.
- `bd setup codex --global` writes to `$CODEX_HOME/AGENTS.md` for tool-
  instruction sharing.

### Motes: first-class global tier

`mote promote <id>` copies a mote to `~/.motes/nodes/` (legacy
`~/.claude/memory/`, auto-migrated). Type-routed defaults (in
`valid_enums.go`):

| Type | Default scope |
|---|---|
| `decision` | **Global** |
| `lesson` | **Global** |
| `explore` | **Global** |
| `question` | **Global** |
| `task` | Project |
| `context` | Project (was global until v0.4.13 — found to dominate global pollution) |
| `constellation` | Project |
| `anchor` | Project |

`~/.motes/MOTES.md` is a generated cross-agent memory index injected at
every `mote prime`. Cross-project dream metrics live at
`~/.motes/dream_quality.jsonl`. v0.4.16 added "intake guardrails"
preventing global pollution — explicit lessons from real use.

### Comparison

This is **the dimension where motes most clearly leads**. Cross-project
knowledge is treated as a primary feature with type-routed defaults,
auto-migration, generated indices, and intake guardrails informed by
operational experience. Beads' "federation" is fundamentally a sync
mechanism for the same database across machines, not a knowledge tier
spanning projects.

For a developer working across many projects (the user's described
workflow), motes' design is materially better.

---

## 12. Documentation and Maturity

### Beads

- **66 files in `docs/`** + a versioned Docusaurus site at
  `gastownhall.github.io/beads/` with `llms-full.txt` snapshots per release.
- ADRs (`adr/0001-bd-prime-as-source-of-truth.md`,
  `adr/0002-init-safety-invariants.md`).
- Topical docs: `ARCHITECTURE.md`, `INTERNALS.md`, `MOLECULES.md`,
  `CHEMISTRY_PATTERNS.md`, `MULTI_REPO_AGENTS.md`,
  `MULTI_REPO_MIGRATION.md`, `COLLISION_MATH.md`, `CLAUDE_INTEGRATION.md`,
  `SETUP.md`, `DOLT.md`, `PROTECTED_BRANCHES.md`, `SYNC_SETUP.md`.
- 15 working examples in `examples/`.
- 5,586-line CHANGELOG.
- 23,287 stars / ~7 months / v1.0.3 (latest 2026-04-24).
- 207 open issues, 1,540 forks, 90+ contributors.
- Heavy CI discipline: golangci-lint, pre-commit, `release-gates/`,
  cross-version smoke tests, migration test harness, Renovate.
- Distribution: Homebrew, npm, PyPI (MCP), AUR, winget, Nix.
- **Asterisk:** `steveyegge` has 4,554 commits to the next contributor's
  409. Bus factor is 1. Most-commented open issue is the Dolt regression.

### Motes

- Comprehensive but smaller doc tree: `prd.md` (13 epics, 46 stories with
  Gherkin), `architecture.md` (1400+ lines of Go types + algorithms),
  `internals.md`, `onboarding.md`, `configuration.md`, `providers.md`,
  `maintenance.md`, `agents-guide.md`, `ml-lens-backlog.md`.
- Five example config docs (claude global/project, codex, gemini, settings).
- Per-agent files: `AGENTS.md`, `CLAUDE.md`, `CODEX.md`, `GEMINI.md`.
- v0.4.17 (most recent commit `d2cf80a` 2026-05-06).
- Active development: dream features (v0.4.7, v0.4.15), tool-neutral global
  memory (v0.4.14), intake guardrails (v0.4.16), salvage logic (v0.4.17).
- Bench harness at `bench/` with committed v0.4.7 baseline (score engine
  112ns/op zero-alloc, traversal 332µs/op for 500 motes, create 98µs/op).
- Test coverage broad — 45+ `_test.go` files.
- Live dogfooding (19+ motes in own `.memory/nodes/`).
- MIT.
- Pre-1.0, single-developer signal, no public release distribution beyond
  source build.

### Comparison

Beads is dramatically more **mature, distributed, and adopted**. Motes is
in active pre-1.0 development with thoughtful design and credible
benchmarks but a vastly smaller surface area, single primary author, and
no published binaries.

---

## 13. Strengths Summary

### Beads' distinctive strengths

1. **Dolt storage** — version-controlled SQL with cell-level merge.
   Native distributed sync. Hash-based collision-free IDs.
2. **Multi-agent atomicity** — `bd update --claim` is the right primitive.
   `swarm` waves for parallel orchestration. Atomic across machines.
3. **Massive integration breadth** — 10+ recipes for AI tools, content-hash
   markers for safe upgrades, custom recipe extension.
4. **`bd prime` source-of-truth discipline** — ADR-backed, three-layer
   override, MCP/CLI auto-detection.
5. **Rich workflow primitives** — molecules, swarms, gates, formulas,
   wisps. Genuinely useful for complex multi-agent processes.
6. **External tracker sync** — GitHub, GitLab, Linear, Jira, ADO, Notion.
7. **Time-aware queries** — `--due-*`, `--defer-*`, `--overdue` are
   first-class.
8. **PreToolUse safety hooks** — `block-gh-watch`, `block-interactive-cmds`
   are real footgun protection.
9. **Distribution maturity** — Homebrew, npm, PyPI, AUR, winget, Nix all
   working.
10. **Documentation depth** — ADRs, versioned site, working examples.

### Motes' distinctive strengths

1. **Knowledge graph as primary** — semantic links, contradiction
   detection, BFS retrieval with relevance scoring, eight purposeful types.
2. **The dream cycle** — link inference, contradiction flagging, merge
   suggestions, action extraction, lens-mode triangulation. No beads
   equivalent.
3. **Strata layer** — external corpus ingestion + crystallization
   feedback loop.
4. **BM25 retrieval** — works offline, zero infrastructure, indexed
   automatically.
5. **First-class cross-project memory** — type-routed promotion,
   generated `MOTES.md` index, intake guardrails informed by real use.
6. **Five LLM backends** — claude-cli, openai, gemini (Vertex), codex-cli,
   gemini-cli. Per-agent provider override layer.
7. **Coordinated multi-agent install** — onboard handles three agents in
   one command, including beads migration.
8. **Comprehensive lifecycle hooks** — seven Claude events, real
   session-end pipeline, per-event mode dispatch.
9. **Markdown + YAML storage** — inspectable by hand, recoverable via
   index rebuild, no daemon, no CGO.
10. **Performance discipline** — committed bench baselines (sub-100ms hot
    path), zero-alloc score engine.

---

## 14. Weaknesses Summary

### Beads' notable weaknesses

1. **No semantic search.** SQL `LIKE` is a real gap for an "AI memory"
   tool.
2. **Lossy compaction discards content permanently.** Trades fidelity for
   size with no review step.
3. **No first-class cross-project memory tier.** Federation is sync, not
   knowledge promotion.
4. **Storage stack complexity.** Dolt + CGO + mandatory build tags + open
   bugs around server-mode restart. Most-commented issue is a Dolt
   regression.
5. **CLI sprawl.** ~277 commands across many themes is a discoverability
   tax.
6. **Single dominant author risk.** Bus factor is 1.
7. **No semantic operations.** No link inference, no contradiction
   detection, no action extraction, no clustering — issue tracker
   semantics only.
8. **`bd edit` requires `$EDITOR`** — explicitly documented as
   agent-hostile.
9. **17 dependency types but only 4 documented as load-bearing** — the
   rest are aspirational.
10. **No native graph visualization in the terminal**.

### Motes' notable weaknesses

1. **No multi-writer correctness.** No atomic claim, no cross-machine
   merge, no merge semantics for concurrent writes to the same node.
2. **No temporal model.** Missing `--due`, `--defer`, `--overdue`,
   `--include-deferred`. Big felt gap (rec §5).
3. **No execution metadata.** No agent/model/reasoning/parallel-group
   hints (rec §6).
4. **Limited integration breadth.** Three agents (claude/codex/gemini)
   vs. beads' ten. No Cursor, Aider, Windsurf.
5. **Pre-1.0, single-author, no published binaries.** Long way from beads'
   distribution maturity.
6. **CLI ergonomics gaps:** no `--plain` mode (rec §23.8), no `--explain`
   on ready (rec §23.9), no `--short`/`--long` progressive disclosure
   (rec §23.10), partial JSON envelope contract (rec §23.7).
7. **No first-class `mote remember/forget/recall/memories` k/v verbs**
   (rec §23.5).
8. **No customizable `PRIME.md` override** (rec §23.3).
9. **No silent-failure prime.** mote prime emits stderr/non-zero on
   failure (rec §23.4).
10. **Hooks live in user dotfiles, not bundled plugin packages**
    (rec §3, §19).
11. **No CI doc-flag freshness check** (rec §12) — risk of doc rot.
12. **No `--stealth`/`--contributor` init modes** (rec §18).

---

## 15. Notable Considerations

### When beads is the better fit

- Teams running **multiple agents in parallel** against shared backlogs.
- Workflows with **strong temporal requirements** (deadlines, defers,
  staleness windows).
- Teams that **already use external trackers** (GitHub, Linear, Jira) and
  want bidirectional sync.
- Multi-machine setups requiring **distributed correctness** with native
  push/pull.
- Editor diversity beyond Claude/Codex/Gemini (Cursor, Aider, Windsurf,
  Cody, Junie).
- Teams that value **single-tool consolidation** over best-of-breed
  per-niche.

### When motes is the better fit

- Solo developers or small teams operating **across many projects** who
  want cross-project knowledge promotion.
- Workflows where **knowledge artifacts** (decisions, lessons, explores)
  are first-class and need to evolve, not just accumulate.
- Setups standardized on **Claude/Codex/Gemini** with deep integration
  per agent.
- Environments where **zero-config, no-CGO, no-daemon** is a hard
  requirement.
- Workflows that benefit from **headless background processing** of
  knowledge maintenance (the dream cycle's link inference, contradiction
  detection, merge suggestions).
- Teams that want **inspectable, hand-editable storage** (markdown files
  in git).
- Use cases needing **multiple LLM backends** with per-agent override.

### Where they could productively borrow

Beads → motes (already enumerated in `recommendation.md`):

- PreToolUse safety hooks (`block-gh-watch`, `block-interactive-cmds`).
- Atomic `--claim` for multi-agent ready races.
- `discovered-from` non-blocking provenance link.
- Temporal model (`--due-*`, `--defer-*`, `--overdue`).
- `bd prime`-style three-layer `PRIME.md` override.
- Hierarchical IDs (`bd-a3f8.1.1`-style child-path syntax).
- JSON envelope contract (`schema_version`, `MOTE_JSON_ENVELOPE=1`).
- `--plain`, `--explain`, `--short/--long` output modes.
- Bundled plugin packaging instead of user dotfile editing.
- CI `release-gates/` for evidence files and doc-flag freshness.

Motes → beads (the inverse, not previously enumerated anywhere):

- BM25 search algorithm (~150 LOC, no infrastructure cost) — fixes the
  `LIKE`-only gap.
- Strata layer for external corpus ingestion + crystallization feedback.
- Type taxonomy with operationally meaningful link semantics
  (`supersedes` auto-deprecates; `contradicts` scores as interference).
- Dream cycle architecture (pre-scan → batch → reconcile → review) —
  a real semantic-evolution engine vs. lossy compaction.
- Lens mode for vision triangulation via diverse mental models.
- Type-routed cross-project promotion with intake guardrails.
- `MOTE_AGENT_KIND` per-agent provider override layer.
- Coordinated multi-agent install in one command.
- Per-event hook mode dispatch (`startup` vs. `resume` vs. `compact`
  vs. `clear`).

---

## 16. Synthesis

The deepest insight from this comparison is that **beads and motes are
solving slightly different problems while marketing themselves as
solving the same problem**.

Beads frames itself as agent memory but is structurally an
**issue-tracking substrate** — its primary persistence unit is a row in a
SQL database, its primary dimensions are workflow state and dependencies,
and its memory affordances (`bd remember/recall/forget`) are a thin k/v
overlay. Its operational excellence is in **multi-writer distributed
correctness**, which makes sense if you imagine fleets of agents racing a
shared backlog.

Motes frames itself as memory and means it — its primary persistence unit
is a typed knowledge atom, its primary dimensions are
planning-vs-semantic relations, and its task tracker is one of eight
types. Its operational excellence is in **headless knowledge evolution
via the dream cycle** and **cross-project promotion**, which makes sense
if you imagine a single developer building up a personal knowledge base
across many projects with AI assistance.

Neither one is wrong. Both are credible answers to "how should AI agents
maintain context across sessions." The choice between them is a choice
about what shape that context takes — issues with workflow metadata, or
knowledge atoms with semantic relations.

The honest critique of each:

- **Beads' weakness is its memory model.** The k/v `remember` is not
  going to scale into rich personal knowledge; the SQL `LIKE` search is
  not going to find things by paraphrase; the lossy compaction discards
  exactly the content that would matter for long-horizon insight.
- **Motes' weakness is its multi-agent story.** The dream cycle is
  brilliant for solo work, but the lack of atomic claim, lack of
  temporal queries, and limited multi-machine sync mean motes is not
  ready for fleets.

The most useful direction for both projects, at this point, is
**convergence on each other's strengths** — beads adopting BM25 + dream-
cycle-style maintenance, motes adopting atomic claim + temporal queries +
PreToolUse safety hooks. The `recommendation.md` in this repo already
maps the motes side of that work in exhaustive detail.

---

*Sources used for this analysis (verified via cloned repo + live source
inspection):*

- `gastownhall/beads` v1.0.3 — README, AGENTS.md, AGENT_INSTRUCTIONS.md,
  CLAUDE.md, CODEX.md, GEMINI.md, internal/types/types.go, cmd/bd/*,
  plugins/beads/skills/beads/SKILL.md, integrations/beads-mcp/,
  docs/{ARCHITECTURE,INTERNALS,MOLECULES,CLAUDE_INTEGRATION,SETUP}.md,
  CHANGELOG.md.
- `motes` v0.4.17 — README.md, recommendation.md, docs/{prd,architecture,
  internals,onboarding,configuration,providers,maintenance,agents-guide}.md,
  cmd/mote/cmd_onboard.go, internal/core/{valid_enums,link_types,traversal}.go,
  internal/dream/{invoker,prescanner,*_invoker}.go, internal/strata/bm25.go,
  skills/mote-subagent/SKILL.md, AGENTS.md, CLAUDE.md, CODEX.md, GEMINI.md.
