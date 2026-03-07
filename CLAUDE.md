# CLAUDE.md

## Motes

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in `.memory/`.

**Do NOT use** markdown files, TodoWrite, TaskCreate, or external issue trackers for tracking work.

### Session Start

***Run `mote prime` at the start of every session for scored, relevant context.***

### Task Tracking

Find available work:

    mote ls --ready           # Tasks with no unfinished blockers
    mote pulse                # Active tasks sorted by weight

Create and manage tasks:

    mote add --type=task --title="Summary" --tag=topic --body "What and why"
    mote update <id> --status=completed

### Planning

Break work into task motes with dependency links:

    mote add --type=task --title="Epic: Feature X" --tag=epic --body "Goal"
    mote add --type=task --title="Implement Y" --tag=story --body "Details"
    mote link <story-id> depends_on <epic-id>

View execution chains:

    mote context --planning <id>

### During Work

Capture knowledge worth preserving:

    mote add --type=decision --title="Summary" --tag=topic --body "Rationale"
    mote add --type=lesson --title="Summary" --tag=topic --body "Details"
    mote add --type=explore --title="Summary" --tag=topic --body "Findings"

Link related motes:

    mote link <id1> relates_to <id2>
    mote link <id1> builds_on <id2>

### Session End

***Run `mote session-end` for access flush and maintenance suggestions.***

Run `mote dream` periodically for automated maintenance. Review with `mote dream --review`.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

## Project Overview

Motes is an AI-native context and memory system. Knowledge is stored as atomic units ("motes") linked in two dimensions: dependency links (planning/execution ordering) and semantic links (thematic memory connections). The CLI tool is `mote`.

**Language:** Go (single native binary, zero-config distribution)
**External deps:** `github.com/spf13/cobra` (CLI), `gopkg.in/yaml.v3` (frontmatter parsing). Everything else is stdlib.
**Storage:** Markdown files with YAML frontmatter in `.memory/nodes/`, no database.

## Key Documents

- `docs/prd.md` — Full PRD with 13 epics, 46 user stories, and acceptance criteria in Gherkin
- `docs/architecture.md` — Technical architecture with Go type definitions, algorithms, and layer design
- `docs/onboarding.md` — Getting started guide and migration from beads/MEMORY.md

## Architecture (4 Layers)

1. **Storage Layer** — `.memory/` directory: mote markdown files in `nodes/`, `index.jsonl` edge index, `config.yaml`, `constellations.jsonl`, strata corpora in `strata/`, dream artifacts in `dream/`
2. **Core Engine** — MoteManager (CRUD), IndexManager (edge index), ScoreEngine (relevance scoring), GraphTraverser (BFS with hop-limited spreading activation), SeedSelector (ambient signal matching), ConfigManager
3. **Strata Engine** — BM25-based reference knowledge search. StrataManager, Chunker (heading-aware/function-level/sliding-window), BM25Index (~150 LOC). No embeddings, no network.
4. **Dream Orchestrator** — Headless LLM maintenance cycle. PreScanner (deterministic candidate finding), BatchConstructor, PromptBuilder, ClaudeInvoker (shells out to `claude` CLI), ResponseParser, LucidLog, VisionWriter

## Three Processing Modes

| Mode | Latency | LLM? | Operations |
|------|---------|------|------------|
| **Hot path** | < 2s | No | Scoring, traversal, contradiction flagging, strata augmentation |
| **Warm path** | < 10s | In-session Claude | Crystallization prompts, link suggestions, strata queries |
| **Dream cycle** | 1-10min | Headless (Sonnet batches + Opus reconciliation) | Semantic analysis, link inference, constellation evolution, staleness review |

## Build & Development Commands

```bash
go build -o mote ./cmd/mote    # Build
go test ./...                   # Run all tests
go test ./internal/scoring      # Run tests for a single package
go test -run TestScoreEngine    # Run a specific test
go vet ./...                    # Lint
```

## Key Design Decisions

- **All file writes use write-to-temp-then-rename** for POSIX atomicity
- **Access count updates are batched** in `.access_batch.jsonl`, flushed at session end — never rewrite mote files on read
- **Edge index is a cache, not source of truth** — derived from mote frontmatter, self-healing via `mote index rebuild`
- **ID format:** `<scope>-<typechar><base36-timestamp><random-suffix>` (collision-resistant)
- **Mote types:** task, decision, lesson, context, question, constellation, anchor, explore
- **Link types:** depends_on/blocks (planning), relates_to, builds_on, contradicts, supersedes, caused_by, informed_by (memory)
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
