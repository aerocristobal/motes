# Motes Internals

Developer reference for architecture, storage, and design decisions. For usage instructions, see the project `CLAUDE.md`.

## Architecture (4 Layers)

1. **Storage Layer** — `.memory/` directory: mote markdown files in `nodes/`, `index.jsonl` edge index, `config.yaml`, `constellations.jsonl`, strata corpora in `strata/`, dream artifacts in `dream/`
2. **Core Engine** — MoteManager (CRUD), IndexManager (edge index), ScoreEngine (relevance scoring), GraphTraverser (BFS with hop-limited spreading activation), SeedSelector (ambient signal matching), ConfigManager
3. **Strata Engine** — BM25-based reference knowledge search. StrataManager, Chunker (heading-aware/function-level/sliding-window), BM25Index (~150 LOC). No embeddings, no network.
4. **Dream Orchestrator** — Headless LLM maintenance cycle. PreScanner (deterministic candidate finding), BatchConstructor, PromptBuilder (builds batch + reconciliation prompts; holds `lensTmpls map[string]*template.Template` for 9 lens variants), ClaudeInvoker (shells out to `claude` CLI), ResponseParser, LucidLog, VisionWriter, VoteVisions (self-consistency voting across N runs per batch, legacy mode), MergeLensResults (tagged union across lens runs — all findings preserved, `CrossLensAgreement` populated when 2+ distinct lenses agree), KnownLenses (validated lens name registry)

## Three Processing Modes

| Mode | Latency | LLM? | Operations |
|------|---------|------|------------|
| **Hot path** | < 2s | No | Scoring, traversal, contradiction flagging, strata augmentation |
| **Warm path** | < 10s | In-session Claude | Crystallization prompts, link suggestions, strata queries |
| **Dream cycle** | 1-10min | Headless (Sonnet batches + Opus reconciliation) | Semantic analysis, link inference, constellation evolution, staleness review |

## Key Design Decisions

- **All file writes use write-to-temp-then-rename** for POSIX atomicity
- **Access count updates are batched** in `.access_batch.jsonl`, flushed at session end — never rewrite mote files on read
- **Edge index is a cache, not source of truth** — derived from mote frontmatter, self-healing via `mote index rebuild`
- **ID format:** `<scope>-<typechar><base36-timestamp><random-suffix>` (collision-resistant)
- **Mote types:** task, decision, lesson, context, question, constellation, anchor, explore
- **Link types:** depends_on/blocks (planning), relates_to, builds_on, contradicts, supersedes, caused_by, informed_by, discovered_from (memory). `discovered_from` is a directional, non-blocking provenance edge — the reverse `discovered_ref` lives in the index only (mirrors `builds_on`/`built_by_ref`).
- **Dream vision types:** link_suggestion, contradiction, tag_refinement, staleness, compression, signal, merge_suggestion, action_extraction, decompose_suggestion.
- **Lens mode vs voting:** `lens_mode.enabled: true` activates N runs with distinct mental model prompts; results are merged with `MergeLensResults` (tagged union — all visions kept, cross-lens matches tagged). `self_consistency_runs > 1` activates identical-prompt voting with `VoteVisions` (consensus filter — only majority visions kept). Mutually exclusive. The `merge_suggestion` vision merges 3+ redundant motes into one authoritative mote using `supersedes` links (auto-deprecation), with inbound/outbound link migration to the new merged mote. The `action_extraction` vision adds a prescriptive `Action` field to lesson/decision motes that lack one, surfaced prominently in `show`, `context`, and `prime`.
- **Scoring formula** combines: base weight + edge bonus + status penalty + recency decay + retrieval strength + salience boost + tag specificity + interference penalty
- **Prime truncation directive:** Every successful `mote prime` body begins with a fixed `[mote prime] ...` line (or carries it as the `truncation_notice` JSON field). The dispatcher captures the body, persists it atomically to `.memory/last_prime.txt`, and only then emits to stdout (or wraps in the hook envelope). When agent hosts truncate the displayed preview, the agent can `cat .memory/last_prime.txt` to recover the full priming context. The directive text is a single source-of-truth constant in `cmd/mote/cmd_prime.go` pinned by a test so wording drift fails CI.
- **Prime silent-failure contract:** `mote prime` exits 0 with no stderr output when it cannot produce a meaningful prime — no `.memory/` directory, unreadable `.memory/`, corrupt index, or an internal panic. The silent path emits the mode-appropriate empty payload: nothing on default text, literal `{}` on `--hook`, an empty `PrimeOutput` envelope on `--json`. The truncation directive is NOT emitted on the silent path. Every other `mote` command keeps its normal error model. Set `MOTE_DEBUG=1` or pass `--debug` to surface the underlying error for postmortem debugging. This contract exists so `mote prime --hook --mode=startup` is safe to chain into any session-start hook script regardless of whether the host machine has a mote project. See STORY-BR-23-4.

## Storage Layout

```
.memory/
├── nodes/*.md              # One mote per file (YAML frontmatter + markdown body)
├── index.jsonl             # Edge index + tag stats (rebuilt from motes)
├── config.yaml             # Scoring, priming, dream, strata config
├── constellations.jsonl    # Constellation cluster records
├── .access_batch.jsonl     # Batched access updates
├── last_prime.txt          # Full body of the most recent `mote prime` (atomic write)
├── dream/
│   ├── log.jsonl               # Dream run history
│   ├── visions.jsonl           # Pending finalized visions
│   ├── visions_draft.jsonl     # Raw Sonnet output (pre-reconciliation)
│   ├── scan_state.json         # Content-hash cache for incremental prescanning
│   └── auto_applied.jsonl      # Auto-applied visions log
└── strata/<corpus>/        # manifest.json, chunks.jsonl, bm25.json

~/.motes/
├── nodes/*.md              # Global motes (decision, lesson, explore, context, question)
├── index.jsonl             # Global edge index
├── dream/, strata/         # Same shape as project-local
├── dream_quality.jsonl     # Cross-project dream quality metrics
├── config.yaml             # User-wide config defaults (overlaid before project config)
└── MOTES.md                # Cross-agent memory index — generated by dream/onboard
```

The legacy global path `~/.claude/memory/` is auto-migrated to `~/.motes/` on first command (see `internal/core/migrate_paths.go`). Files Claude's auto-memory mechanism owns (`MEMORY.md`, top-level `*.md`, `.obsidian/`) are intentionally left at the legacy location — motes never reads or writes them.

Each agent has its own native memory mechanism that motes treats as opaque:

- Claude Code's auto-memory at `~/.claude/memory/` (managed by the harness)
- Codex Memories at `~/.codex/memories/` (gated by `[features] memories = true`)
- Gemini's `save_memory` writes to `~/.gemini/GEMINI.md`; experimental Auto Memory feature

Motes never reads or writes any of these. Cross-agent shared knowledge flows through `MOTES.md`; per-agent thread carry-over stays in each agent's native subsystem.

## Project Conventions

- Motes are parsed by splitting on `---` boundaries, unmarshaling YAML into Go structs, body is everything below second `---`
- Parallel file reads use goroutines + sync.WaitGroup (see `ReadAllParallel`)
- Dream cycle invokes `claude` CLI via `os/exec` — never handles OAuth/API keys directly
- BM25 tokenizer: lowercase, split on non-alphanumeric, remove stop words, no stemming

## CI

Single workflow at `.github/workflows/ci.yml` with four jobs (`build`, `vet`, `test`, `lint-actions`) on `ubuntu-latest`. Top-level `concurrency` block uses the key `${{ github.workflow }}-${{ github.event_name }}-${{ github.event.pull_request.number || github.ref }}` with `cancel-in-progress: true` so a force-push to a PR cancels the in-flight run. The `event_name` segment is load-bearing: without it, a `push` to master and a `pull_request` to the same branch would share a group and cancel each other. Tracked by `internal/ci/workflow_test.go`.

Every `uses:` reference is pinned to a 40-character lowercase hex commit SHA followed by a trailing `# <tag>` comment (e.g. `actions/checkout@de0fac2e... # v6.0.2`). Tags are mutable — the action's maintainer (or an attacker with write access) can move them; a commit SHA cannot be silently rewritten. The tag comment preserves human-readable provenance. The rule applies uniformly to first-party (`actions/*`) and third-party actions, enforced by `scripts/lint-actions-pinning.sh` and the `lint-actions` CI job. Tested via `internal/ci/lint_actions_test.go` (fixture-based, mirrors the `internal/githook` shell-out pattern).

The concurrency cancellation *behavior* is a property of the GitHub-hosted runner and cannot be unit-tested locally; the Go tests prove the *declaration* is correct. One-time smoke test: push a branch, immediately push a second commit, and verify in the Actions UI that the first run transitions to "cancelled".
