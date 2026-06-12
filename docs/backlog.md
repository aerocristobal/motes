# Motes Improvement Backlog

**Derived from:** full-repo analysis (v0.4.47, 2026-06-11) against the problem statement and product overview in [docs/prd.md](prd.md).
**Format:** BDD epics ("In order to / As a / I want") with titled stories and brief acceptance criteria, per the team's BDD conventions.
**Workflow note:** this document is a recommendations backlog, not a task tracker. When an epic is picked up, plan it into motes via `/mote-plan`; do not track execution status here.

## Priority Legend

| Tier | Meaning |
|------|---------|
| **P1** | Correctness risks, silent failure modes, or friction that blocks adoption — do first |
| **P2** | Capability gaps vs the PRD, quality, and test debt — core roadmap |
| **P3** | Polish, refactoring, and longer-horizon learning features |

## Backlog Summary

| Epic | Title | Area | Priority | Stories |
|------|-------|------|----------|---------|
| [EPIC-SAFE-001](#epic-safe-001) | Write-safety and error-suppression cleanup | C — Hardening | P1 | 4 |
| [EPIC-PERF-001](#epic-perf-001) | Persistent metadata index and hot-path performance | C — Hardening | P1 | 4 |
| [EPIC-ONBD-001](#epic-onbd-001) | Onboarding hardening for Codex and Gemini | B — AI tools | P1 | 4 |
| [EPIC-ADOC-001](#epic-adoc-001) | Agent-facing documentation discoverability | B — AI tools | P1 | 5 |
| [EPIC-SEM-001](#epic-sem-001) | Pluggable embedding provider and semantic search | A — Capability | P2 | 4 |
| [EPIC-SIGNAL-001](#epic-signal-001) | Close the signal-discovery loop | A — Capability | P2 | 3 |
| [EPIC-SESS-001](#epic-sess-001) | Session-end intelligence | A — Capability | P2 | 3 |
| [EPIC-CONTRA-001](#epic-contra-001) | Real-time contradiction surfacing | A — Capability | P2 | 2 |
| [EPIC-MCP-001](#epic-mcp-001) | Ship the mote MCP server | B — AI tools | P2 | 4 |
| [EPIC-TEST-001](#epic-test-001) | Test-coverage and concurrency-safety debt | C — Hardening | P2 | 4 |
| [EPIC-ROBUST-001](#epic-robust-001) | Input robustness and process safety | C — Hardening | P2 | 4 |
| [EPIC-WIKI-001](#epic-wiki-001) | Wikilinks as first-class graph edges | A — Capability | P3 | 3 |
| [EPIC-TAGS-001](#epic-tags-001) | Co-occurrence tag-split suggestions | A — Capability | P3 | 2 |
| [EPIC-EXEC-001](#epic-exec-001) | Execution-outcome learning | A — Capability | P3 | 3 |
| [EPIC-DREAMQ-001](#epic-dreamq-001) | Dream-cycle depth and focus | A — Capability | P3 | 3 |
| [EPIC-REFAC-001](#epic-refac-001) | Decompose oversized modules | C — Hardening | P3 | 4 |

---

## Area A — Strengthen Capabilities vs the PRD Problem Statement

The PRD's problem statement centers on selective, relationship-aware, self-maintaining memory: knowledge weighted by relevance and age, contradictions surfaced rather than coexisting silently, exploration findings preserved, and a nebula that improves itself without competing with task execution. The hot path (`mote prime`, `mote context`, scoring) delivers on this well. The epics below close the gaps where the implementation falls short of the stated vision.

<a id="epic-sem-001"></a>
### EPIC-SEM-001: Pluggable Embedding Provider and Semantic Search (P2)

> In order to detect semantic relationships that keyword matching cannot see — corpus overlap, near-duplicate motes, thematic link candidates
> As a nebula maintainer
> I want a configurable embedding provider behind the existing BM25 search, as PRD Story 13.9 specifies

**Evidence:** `internal/strata/bm25.go` is the only ranking implementation ("no embeddings, no network" per docs/internals.md). PRD Story 13.9 specifies configurable embedding backends (local sentence-transformers/Ollama, remote Claude/OpenAI). PRD Story 13.8's corpus-overlap detection is impossible without semantic similarity.

**Stories:**
- **STORY-SEM-001: Embedding provider interface and config** — AC: `.memory/config.yaml` accepts an `embeddings:` block (provider, model, endpoint); a provider interface with at least one local (Ollama) and one remote (OpenAI/Claude) implementation; absent config means pure-BM25 behavior is unchanged.
- **STORY-SEM-002: Hybrid strata ranking** — AC: when embeddings are configured, `mote strata query` blends BM25 and cosine-similarity scores; ranking quality validated against a fixture corpus; falls back to BM25 on provider failure with a warning, never an error.
- **STORY-SEM-003: Embedding cache and incremental indexing** — AC: chunk embeddings are persisted alongside the BM25 index; `mote strata update` re-embeds only changed chunks (hash-gated); no network calls when nothing changed.
- **STORY-SEM-004: Corpus-overlap detection (PRD 13.8)** — AC: dream-cycle pre-scan flags corpora whose chunk embeddings show high pairwise similarity; produces a `corpus_overlap` vision recommending merge or deprecation.

<a id="epic-signal-001"></a>
### EPIC-SIGNAL-001: Close the Signal-Discovery Loop (P2)

> In order to make priming genuinely self-improving as usage grows, per the PRD's encoding-specificity vision
> As an agent session starting work
> I want signal-discovery visions accepted in dream review to register themselves in the seed-selector config automatically

**Evidence:** `internal/dream/prescanner.go` discovers access-pattern signals and emits `signal_discovery` visions; `internal/core/seed.go` reads signals from `.memory/config.yaml`; but applying an accepted vision requires a manual config edit — the pipeline is one-way (docs/maintenance.md confirms manual registration).

**Stories:**
- **STORY-SIGNAL-001: Auto-register accepted signal visions** — AC: accepting a `signal_discovery` vision in `mote dream --review` writes the signal into `.memory/config.yaml` atomically; rejection leaves config untouched; the dream log records the registration.
- **STORY-SIGNAL-002: Signal lifecycle management** — AC: registered signals carry provenance (vision ID, date); `mote doctor` flags signals that have not fired in N sessions as candidates for removal; a signal can be retired via a dream vision the same way it was registered.
- **STORY-SIGNAL-003: Signal effectiveness telemetry** — AC: `mote prime --debug` (or `mote stats`) reports per-signal hit rates so the dream cycle can later judge which discovered signals actually improved seed selection.

<a id="epic-sess-001"></a>
### EPIC-SESS-001: Session-End Intelligence (P2)

> In order to stop exploration findings from disappearing when a session ends — pain point #4 in the PRD problem statement
> As an agent finishing a session
> I want `mote session-end` to actively detect preservation-worthy exploration and propose lightweight links, as PRD Story 12.8 specifies

**Evidence:** `cmd/mote/cmd_session_end.go` flushes access batches and suggests crystallization candidates, but the PRD's exploration-detection heuristics (3+ web searches, comparative analysis, API exploration) and "up to 3 lightweight link proposals" are not implemented — the command is a data-flush checkpoint, not the lightweight maintenance pass the PRD envisions.

**Stories:**
- **STORY-SESS-001: Exploration-preservation heuristics** — AC: session-end inspects the session's access batch and (where available via hook payload) transcript signals; sessions matching the PRD heuristics produce an explicit "preserve this exploration?" prompt naming the candidate topic.
- **STORY-SESS-002: Lightweight link suggestions** — AC: session-end proposes up to 3 link candidates derived from co-accessed motes in the ending session only (no full graph scan); suggestions are advisory output, never auto-applied.
- **STORY-SESS-003: Hook-payload session summary ingestion** — AC: when the Stop/SessionEnd hook payload includes a transcript path, session-end extracts mentioned mote IDs and surfaces unlinked co-mentions as link candidates.

<a id="epic-contra-001"></a>
### EPIC-CONTRA-001: Real-Time Contradiction Surfacing (P2)

> In order to stop contradictory knowledge from coexisting silently — pain point #9 in the PRD problem statement
> As an agent loading context
> I want contradicting motes flagged prominently the moment both appear in a traversal result, per PRD Story 4.6

**Evidence:** `internal/core/traversal.go` counts contradictions for scoring, but the PRD's "⚠ Contradictions" section in `mote context` output is weakly implemented; today contradictions are reliably visible only via a separate `mote doctor` run.

**Stories:**
- **STORY-CONTRA-001: Contradiction section in context and prime output** — AC: whenever two motes connected by a `contradicts` edge both appear in `mote context` or `mote prime` results, the output includes a dedicated "⚠ Contradictions" section listing each pair with one-line summaries; present in plain, JSON, and hook-payload renderings.
- **STORY-CONTRA-002: Contradiction-aware ranking guardrail** — AC: when a contradicted pair surfaces, the newer/higher-scored mote is annotated as "preferred" using existing scoring components; the annotation cites why (recency, supersedes link), so agents do not have to adjudicate blind.

<a id="epic-wiki-001"></a>
### EPIC-WIKI-001: Wikilinks as First-Class Graph Edges (P3)

> In order to make body-text relationships participate in spreading activation rather than being decorative
> As a mote author linking knowledge inline
> I want `[[id]]` wikilinks resolved as traversal edges with their own (lower) edge weight

**Evidence:** wikilinks render in `cmd/mote/cmd_show.go` but `internal/core/traversal.go` follows only frontmatter edge types; PRD Story 3.1 says wikilinks are "resolved during search and graph traversal."

**Stories:**
- **STORY-WIKI-001: Index wikilink edges** — AC: `mote index rebuild` extracts `[[id]]` references from bodies into the edge index as a distinct `wikilink` edge type; broken wikilinks are reported by `mote doctor` (not silently dropped).
- **STORY-WIKI-002: Traversal and scoring participation** — AC: BFS traversal follows wikilink edges with a configurable weight (default lower than `relates_to`); scoring formula documentation in docs/architecture.md updated to match.
- **STORY-WIKI-003: Wikilink hygiene in doctor** — AC: `mote doctor --fix` offers to convert high-confidence repeated wikilink pairs into explicit `relates_to` frontmatter links.

<a id="epic-tags-001"></a>
### EPIC-TAGS-001: Co-occurrence Tag-Split Suggestions (P3)

> In order to keep tag specificity high without manual taxonomy work, per PRD Story 4.5
> As a nebula maintainer running `mote tags audit`
> I want overloaded tags to come with algorithmically suggested sub-tag splits

**Evidence:** `cmd/mote/cmd_tags.go` shows frequency, specificity scores, and overload flags (>15), but the PRD's co-occurrence-based sub-tag suggestions are absent — users see "tag X on 22 motes" with no proposed split.

**Stories:**
- **STORY-TAGS-001: Co-occurrence analysis** — AC: for each overloaded tag, the audit computes co-occurring tags and title-token clusters across its motes and prints up to 3 suggested sub-tags with member counts.
- **STORY-TAGS-002: Guided split application** — AC: `mote tags split <tag>` applies an accepted suggestion interactively (per-mote confirm or `--all`), updating frontmatter atomically and rebuilding the index once at the end.

<a id="epic-exec-001"></a>
### EPIC-EXEC-001: Execution-Outcome Learning (P3)

> In order to learn which agent/model/effort combinations actually work, instead of only recording dispatch intent
> As an orchestrator dispatching subagents
> I want the dream cycle to analyze past `execution_*` metadata against task outcomes and recommend dispatch defaults

**Evidence:** `internal/core/mote.go` carries `execution_agent_type`, `execution_suggested_model`, `execution_reasoning_effort`, etc., and docs/agents-guide.md documents the read-before-prose contract — but `internal/dream/prescanner.go` never analyzes these fields; the system stores dispatch intent without learning from results.

**Stories:**
- **STORY-EXEC-001: Outcome capture at task close** — AC: closing a task mote that carries execution metadata records the actual outcome (completed/reopened/superseded, wall-clock bucket) alongside the dispatch intent.
- **STORY-EXEC-002: Dream-cycle execution analysis** — AC: pre-scan aggregates outcome-by-model/effort patterns and emits an `execution_default` vision when a pattern is strong (e.g., "tasks tagged `docs` always completed at low effort").
- **STORY-EXEC-003: Dispatch-default suggestions** — AC: `mote add --type=task` suggests execution metadata defaults derived from accepted `execution_default` visions for matching tags; suggestions are never silently applied.

<a id="epic-dreamq-001"></a>
### EPIC-DREAMQ-001: Dream-Cycle Depth and Focus (P3)

> In order to make automated maintenance match the PRD's quality bar for compression, corpus health, and safety interrupts
> As a nebula maintainer running `mote dream`
> I want task-focused batches and hardened interrupt handling instead of mixed-task batches that rush deep analysis

**Evidence:** compression candidates (300+ words, `internal/dream/prescanner.go`) share batches with link/contradiction/tag work, diluting distillation quality (PRD Story 12.6). Stale-corpus flagging from PRD Story 13.8 is not in the pre-scan. Interrupt-driven vision withdrawal (PRD Story 12.5) exists structurally in `internal/dream/lucidlog.go` but the withdrawal mechanism is not explicitly coded.

**Stories:**
- **STORY-DREAMQ-001: Dedicated compression batches** — AC: pre-scan routes compression candidates into compression-only batches with a distillation-focused prompt; batch reports include before/after word counts.
- **STORY-DREAMQ-002: Stale-corpus flagging** — AC: pre-scan reads `query_log.jsonl` per corpus and emits a `stale_corpus` vision for corpora unqueried in a configurable window (default 60 days).
- **STORY-DREAMQ-003: Interrupt-driven vision withdrawal** — AC: reconciliation explicitly withdraws visions whose member motes are named by a high-severity interrupt; withdrawal is recorded in the lucid log; an interrupt touching >20% of scanned motes recommends a follow-up cycle, as designed.

---

## Area B — Improve Use of Motes by AI Tools (Claude, Codex, Gemini)

Integration is mature — unified hook payloads, adaptive prime sizing, four shipped skills, execution-metadata contracts. The remaining issues are silent failure modes at onboard time and the fact that the deepest contracts live in `docs/agents-guide.md` rather than the instruction files agents actually load every session.

<a id="epic-onbd-001"></a>
### EPIC-ONBD-001: Onboarding Hardening for Codex and Gemini (P1)

> In order to eliminate silent failure modes where a user completes `mote onboard` and hooks never fire
> As a new Codex or Gemini user onboarding motes
> I want onboarding to verify its own end state and fail loudly when it cannot

**Evidence:** `cmd/mote/cmd_onboard.go:1313-1361` (`ensureCodexFeatureFlag`) creates or appends `codex_hooks = true` in most paths, but when `~/.codex/config.toml` already has a `[features]` section without the key it only prints a warning (line 1345) and requires a manual edit — hooks then silently never fire. Gemini timeouts are written correctly by onboard (300000 ms) but there is no validation guarding manual edits against the seconds-vs-milliseconds trap (GEMINI.md warns; `ensureGeminiSettings` does not check). Gemini's `PreCompress` event is documented as unwired (GEMINI.md).

**Stories:**
- **STORY-ONBD-001: Codex feature-flag insertion in all paths** — AC: when `[features]` exists without `codex_hooks`, onboard inserts the key into that section (TOML-safe edit) instead of warning; onboard exits non-zero if the flag cannot be confirmed; success path prints "✓ Codex hooks enabled — restart Codex to activate."
- **STORY-ONBD-002: Onboard self-verification step** — AC: every onboard run ends with a verification summary (hooks file present, feature flag set, skills installed, timeouts sane) per tool; any failed check is listed with the exact manual fix.
- **STORY-ONBD-003: Gemini timeout validation** — AC: `ensureGeminiSettings` (and `mote doctor`) warn when any motes hook timeout in `~/.gemini/settings.json` is below 300000 ms for SessionEnd or otherwise implausibly small (likely seconds entered as ms).
- **STORY-ONBD-004: Wire Gemini PreCompress** — AC: onboard registers a PreCompress hook mirroring Claude's PreCompact priming behavior; documented in GEMINI.md and docs/example-gemini-config.md.

<a id="epic-adoc-001"></a>
### EPIC-ADOC-001: Agent-Facing Documentation Discoverability (P1)

> In order to make every agent that loads only AGENTS.md aware of the contracts that currently hide in docs/agents-guide.md
> As any AI agent starting a session in a motes project
> I want the load-bearing one-liners (hooks, execution metadata, attribution, promotion, timing) in the file I actually read

**Evidence:** AGENTS.md (auto-loaded by Codex, imported by Gemini) does not mention that lifecycle hooks exist, does not show `mote show <id> --execution-only`, omits `MOTE_AGENT_ID`, omits `mote promote`, and does not warn that `mote session-end` can take 1–3 minutes. The instruction-doc drift check (`doctor.instruction_docs.shared_sections`) is opt-in, so the four instruction files (CLAUDE.md, AGENTS.md, CODEX.md, GEMINI.md) can silently diverge outside the explicitly listed sections.

**Stories:**
- **STORY-ADOC-001: Hooks summary in AGENTS.md** — AC: AGENTS.md gains a 2–3 line "Session lifecycle" note naming the three hook points (session start, per-prompt, session end) and pointing at the per-tool files for setup.
- **STORY-ADOC-002: Execution-metadata one-liner in AGENTS.md** — AC: the subagent-dispatch section shows `mote show <id> --execution-only | jq .` with the read-before-prose rule, mirroring docs/agents-guide.md.
- **STORY-ADOC-003: Attribution and promotion discoverability** — AC: AGENTS.md states the `MOTE_AGENT_ID=subagent-<purpose>` requirement for subagent writes and mentions `mote promote <id>` for cross-project knowledge in one sentence each.
- **STORY-ADOC-004: Session-end timing expectation** — AC: the Landing-the-Plane section notes that the session-end hook may take 1–3 minutes on large graphs and must not be interrupted.
- **STORY-ADOC-005: Drift check default-on** — AC: the instruction-doc sync check runs by default in `mote doctor` for the shared sections (opt-out via config rather than opt-in), so divergence across the four instruction files is caught routinely.

<a id="epic-mcp-001"></a>
### EPIC-MCP-001: Ship the Mote MCP Server (P2)

> In order to let agents query the nebula through tool calls instead of shelling out, with payloads sized to tool-result budgets
> As an AI agent host that speaks MCP
> I want a `mote mcp` server exposing the read surface that the existing detection and `--mcp` sizing already anticipate

**Evidence:** `internal/prime/detect.go` auto-detects an `mcpServers.mote` entry across Claude/Codex/Gemini settings, and `mote prime --mcp` emits a 75-token payload — but no MCP server implementation exists in the repo; comments reference a future "mote MCP wrapper."

**Stories:**
- **STORY-MCP-001: Minimal stdio MCP server** — AC: `mote mcp` serves MCP over stdio exposing read tools (`prime`, `context`, `search`, `show`, `ls --ready`) that reuse the existing `--json` rendering paths; no write tools in v1.
- **STORY-MCP-002: Write tools behind explicit opt-in** — AC: `add`, `update --status`, and `link` are exposed only when the server is started with a write flag; writes carry agent attribution from the MCP client identity.
- **STORY-MCP-003: Onboard wiring** — AC: `mote onboard` offers to register `mcpServers.mote` in the detected host settings; detection in `internal/prime/detect.go` then activates the compact prime path end to end.
- **STORY-MCP-004: Payload budget contract** — AC: every MCP tool result respects a configurable token budget with the truncation directive on the first line, matching the documented prime contract.

---

## Area C — Harden and Optimize the Code

The codebase is well-tested by file count (182 test files) and has real safety infrastructure (`core.AtomicWrite`, `FileLock`, `internal/security/validation.go`). The epics below close the gaps that infrastructure doesn't yet cover: suppressed errors on write paths, a read-cache race, O(N) graph reloads on every command, and 28 untested command files.

<a id="epic-safe-001"></a>
### EPIC-SAFE-001: Write-Safety and Error-Suppression Cleanup (P1)

> In order to prevent silent data corruption when parent agents and subagents write concurrently
> As a multi-agent workflow writing to `.memory/`
> I want every write path to either succeed atomically or fail loudly

**Evidence:** `_ =` error suppression on critical writes/rebuilds: `cmd/mote/cmd_constellation.go:210` (`_ = mm.Link`), `:250` (`_ = im.Rebuild`); `cmd/mote/cmd_session_end.go:383` (raw `_ = os.WriteFile`, also bypassing `core.AtomicWrite`), `:397`; `cmd/mote/cmd_add.go:287-298`; `cmd/mote/cmd_strata.go:248,313,541`; `cmd/mote/cmd_github_import.go:93,249`. `constellations.jsonl` appends use `O_APPEND` without the existing `FileLock`. The read cache has a TOCTOU window: `mote_manager.go:1171-1186` can cache a stale mote if a concurrent write lands between `cache.Get` and `cache.Put` (`internal/core/read_cache.go`).

**Stories:**
- **STORY-SAFE-001: Eliminate silent error suppression on write paths** — AC: every `_ =` on a write, link, or index-rebuild call either propagates the error or logs a visible warning with explicit justification; `go vet`-style lint (errcheck or equivalent) gates regressions in CI.
- **STORY-SAFE-002: Atomic, locked node writes everywhere** — AC: all `.memory/nodes/` writes go through `core.AtomicWrite`; the session-end enrichment raw `os.WriteFile` is replaced; a single helper is the only sanctioned node-write entry point.
- **STORY-SAFE-003: Lock JSONL appends** — AC: `constellations.jsonl` (and any other multi-process append target) acquires the existing `FileLock` around append; interleaved-write test added.
- **STORY-SAFE-004: Fix read-cache TOCTOU** — AC: cache miss-and-parse re-validates mtime before `Put` (or moves to generation-counter invalidation); a `-race` test demonstrating the old staleness now passes.

<a id="epic-perf-001"></a>
### EPIC-PERF-001: Persistent Metadata Index and Hot-Path Performance (P1)

> In order to keep the hot path under the PRD's <2s budget as graphs grow past 1,000 motes
> As any command or subagent loop invoking `mote` repeatedly
> I want commands to stop re-reading and re-indexing the entire graph on every invocation

**Evidence:** 15+ commands call `mm.ReadAllParallel()`/`ReadAllWithGlobal()` to load every mote from disk per invocation. `cmd/mote/cmd_search.go:128-139` builds an ephemeral BM25 index from all motes on every search (the comment says "Build ephemeral BM25 index"). Access-batch appends acquire and release flock once per mote access (`internal/core/mote_manager.go:1252-1260`), so a 100-mote read costs 100 lock cycles.

**Stories:**
- **STORY-PERF-001: Persistent metadata index** — AC: a `.memory/.index/meta.json` (id, type, title, tags, status, weight, mtime) is maintained by the atomic-write helper; `mote ls`/`pulse`/filter paths answer from the index without parsing bodies; index self-heals via rebuild when stale.
- **STORY-PERF-002: Persistent mote BM25 index** — AC: `mote search` reuses a persisted BM25 index over motes, invalidated incrementally on write; cold-build only when missing; search latency on a 1k-mote fixture drops measurably (benchmark committed).
- **STORY-PERF-003: Buffered access-batch appends** — AC: access records buffer in memory per process and flush once per command under a single lock; crash mid-command loses at most that command's access records (documented trade-off).
- **STORY-PERF-004: Hot-path benchmarks in CI** — AC: `go test -bench` benchmarks for prime, context, search, and ls on a generated 1k-mote fixture, with results recorded so regressions are visible in review.

<a id="epic-test-001"></a>
### EPIC-TEST-001: Test-Coverage and Concurrency-Safety Debt (P2)

> In order to detect regressions in the commands agents depend on most
> As a maintainer changing core behavior
> I want the untested command surface and the untested concurrency model covered

**Evidence:** 28 of 47 `cmd/mote/cmd_*.go` files have no test file (including `cmd_add`, `cmd_update`, `cmd_dream`, `cmd_plan`, `cmd_crystallize`). No `-race` concurrency tests exercise concurrent read/write/claim on `internal/core/mote_manager.go` (2,067 LOC). No fuzz/corruption tests for `ParseMote` frontmatter edge cases. No lock-timeout or crash-cleanup tests for `internal/core/filelock.go`.

**Stories:**
- **STORY-TEST-001: Command coverage for the top-traffic ten** — AC: table-driven tests for `add`, `update`, `link`, `plan`, `dream` (dry-run), `crystallize`, `check`, `progress`, `quick`, `done` covering happy path + one failure mode each; runs under `bash scripts/test.sh`.
- **STORY-TEST-002: Concurrency suite under -race** — AC: tests covering concurrent Create/ReadAllParallel, concurrent `--claim` contention (exit-code-2 contract), and concurrent access-batch appends; suite green under `go test -race`.
- **STORY-TEST-003: Frontmatter fuzz tests** — AC: `FuzzParseMote` exercises missing closing `---`, invalid UTF-8, truncated files, and control characters; parser never panics and always reports a structured error.
- **STORY-TEST-004: FileLock failure-mode tests** — AC: tests for lock held past timeout, stale lock file after simulated crash, and nested-acquisition behavior, documenting the intended semantics.

<a id="epic-robust-001"></a>
### EPIC-ROBUST-001: Input Robustness and Process Safety (P2)

> In order to survive pathological inputs and stalled subprocesses without hangs, OOMs, or silent overwrites
> As a mote process running unattended inside an agent loop
> I want bounded reads, collision-safe IDs, and timeouts on every external process

**Evidence:** no file-size check before `os.ReadFile` in the node-read path (a corrupt or malicious multi-GB node file OOMs `ReadAllParallel`). `internal/core/id.go` generates timestamp + 4-char suffix IDs and `Create()` does not check for an existing ID before writing (silent overwrite on collision). `openEditor()` (`cmd/mote/helpers.go:130-146`) has no timeout; the gcloud token fetch in `internal/dream/gemini_invoker.go:270-280` depends on gcloud honoring context cancellation. Panic recovery exists only in `mote prime` (`cmd_prime.go:177-186`).

**Stories:**
- **STORY-ROBUST-001: Bounded node reads** — AC: node files larger than a configurable cap (default 10 MiB) are skipped with a doctor-visible warning instead of being read into memory.
- **STORY-ROBUST-002: ID-collision guard** — AC: `Create()` retries with a fresh suffix when the generated ID already exists; collision is impossible to turn into silent overwrite.
- **STORY-ROBUST-003: Subprocess timeouts everywhere** — AC: editor invocation gets a configurable timeout (generous default), gcloud token fetch gets a hard kill after context deadline, and dream-backend timeouts move from hardcoded 5 minutes to config.
- **STORY-ROBUST-004: Central panic recovery** — AC: a recover wrapper at the root `RunE` converts panics in any command into a non-zero exit with a clear message, matching what `prime` already does.

<a id="epic-refac-001"></a>
### EPIC-REFAC-001: Decompose Oversized Modules (P3)

> In order to keep the highest-churn files testable and reviewable
> As a contributor modifying core behavior
> I want the monolith files split along their natural seams and duplicated logic extracted

**Evidence:** `internal/core/mote_manager.go` (2,067 LOC, 30+ methods), `cmd/mote/cmd_onboard.go` (1,674 LOC), `cmd/mote/cmd_prime.go` (1,124 LOC), 294-LOC `runDreamInner` in `cmd_dream.go`, 721-LOC `internal/dream/orchestrator.go`. Mote→BM25-chunk building is duplicated in `cmd_search.go:129`, `helpers.go:188` (`rebuildMoteBM25`), and `cmd_add.go:208` (`appendAutoLinks`).

**Stories:**
- **STORY-REFAC-001: Extract shared mote→chunks helper** — AC: one `strata.MakeMoteChunks(motes)` replaces the three duplicates; behavior identical (existing search tests pass unchanged).
- **STORY-REFAC-002: Split mote_manager.go by concern** — AC: reader/writer/claim/access-batch concerns live in separate files within `internal/core`; no exported API changes; `go vet` and full test suite green.
- **STORY-REFAC-003: Decompose onboard per tool** — AC: Claude/Codex/Gemini setup paths each live in their own file with a shared orchestration entry point, making per-tool onboarding independently testable (supports EPIC-ONBD).
- **STORY-REFAC-004: Slim dream command orchestration** — AC: `runDreamInner` delegates to focused helpers (pipeline run, review loop, stats rendering); no function in `cmd_dream.go` exceeds ~80 LOC.

---

## Suggested Sequencing

1. **P1 first:** EPIC-SAFE-001 and EPIC-ONBD-001 remove silent failure modes (data loss; hooks that never fire). EPIC-ADOC-001 is cheap, pure documentation, and immediately improves every agent session. EPIC-PERF-001 matters before graphs grow further.
2. **Dependencies:** EPIC-SEM-001 is a prerequisite for STORY-DREAMQ-002's semantic sibling (corpus overlap, STORY-SEM-004). STORY-REFAC-003 (onboard decomposition) is best done alongside EPIC-ONBD-001 rather than after it. EPIC-TEST-001's concurrency suite should land with or immediately after EPIC-SAFE-001 to lock in the fixes.
3. **P3 epics** (WIKI, TAGS, EXEC, DREAMQ, REFAC) are independent of each other and can be scheduled opportunistically.
