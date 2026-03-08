# Motes Internals

Developer reference for architecture, storage, and design decisions. For usage instructions, see the project `CLAUDE.md`.

## Architecture (4 Layers)

1. **Storage Layer** — `.memory/` directory: mote markdown files in `nodes/`, `index.jsonl` edge index, `config.yaml`, `constellations.jsonl`, strata corpora in `strata/`, dream artifacts in `dream/`
2. **Core Engine** — MoteManager (CRUD), IndexManager (edge index), ScoreEngine (relevance scoring), GraphTraverser (BFS with hop-limited spreading activation), SeedSelector (ambient signal matching), ConfigManager
3. **Strata Engine** — BM25-based reference knowledge search. StrataManager, Chunker (heading-aware/function-level/sliding-window), BM25Index (~150 LOC). No embeddings, no network.
4. **Dream Orchestrator** — Headless LLM maintenance cycle. PreScanner (deterministic candidate finding), BatchConstructor, PromptBuilder, ClaudeInvoker (shells out to `claude` CLI), ResponseParser, LucidLog, VisionWriter, VoteVisions (self-consistency voting across N runs per batch)

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
- **Link types:** depends_on/blocks (planning), relates_to, builds_on, contradicts, supersedes, caused_by, informed_by (memory)
- **Dream vision types:** link_suggestion, contradiction, tag_refinement, staleness, compression, signal, merge_suggestion. The `merge_suggestion` vision merges 3+ redundant motes into one authoritative mote using `supersedes` links (auto-deprecation), with inbound/outbound link migration to the new merged mote.
- **Scoring formula** combines: base weight + edge bonus + status penalty + recency decay + retrieval strength + salience boost + tag specificity + interference penalty

## Storage Layout

```
.memory/
├── nodes/*.md              # One mote per file (YAML frontmatter + markdown body)
├── index.jsonl             # Edge index + tag stats (rebuilt from motes)
├── config.yaml             # Scoring, priming, dream, strata config
├── constellations.jsonl    # Constellation cluster records
├── .access_batch.jsonl     # Batched access updates
├── dream/                  # Visions, lucid log, run history
└── strata/<corpus>/        # manifest.json, chunks.jsonl, bm25.json
```

## Project Conventions

- Motes are parsed by splitting on `---` boundaries, unmarshaling YAML into Go structs, body is everything below second `---`
- Parallel file reads use goroutines + sync.WaitGroup (see `ReadAllParallel`)
- Dream cycle invokes `claude` CLI via `os/exec` — never handles OAuth/API keys directly
- BM25 tokenizer: lowercase, split on non-alphanumeric, remove stop words, no stemming
