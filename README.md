# Motes

An AI-native context and memory system. Knowledge is stored as atomic units ("motes") linked in two dimensions: **dependency links** for planning/execution ordering and **semantic links** for thematic memory connections.

Motes runs as a single native Go binary with zero external services. All data lives in `.memory/` as markdown files with YAML frontmatter — no database, no network for core operations.

## Install

```bash
go build -o mote ./cmd/mote
cp mote ~/.local/bin/   # or: make install
```

Requires Go 1.25+.

## Getting Started

New project? Run `mote init`. Coming from beads or MEMORY.md? Run `mote onboard` — it auto-detects and migrates your existing data. See [docs/onboarding.md](docs/onboarding.md) for the full guide.

## Quick Start

```bash
# Initialize a project
mote init

# Create a task
mote add --type=task --title="Implement auth" --tag=auth --body "JWT-based auth for API"

# Create a decision
mote add --type=decision --title="Use JWT over sessions" --tag=auth --body "Stateless, scales horizontally"

# Link them
mote link <decision-id> informed_by <task-id>

# Find available work
mote ls --ready

# Plan hierarchical work
mote plan <task-id> --child "Design API" --child "Implement endpoints" --child "Write tests" --sequential

# Track progress
mote progress <task-id>

# Check off acceptance criteria
mote check <task-id> 1

# Get session context
mote prime
```

## Concepts

### Mote Types

| Type | Purpose |
|------|---------|
| `task` | Work items with status tracking and dependency ordering |
| `decision` | Architectural choices with rationale |
| `lesson` | Insights from failures, debugging, or experience |
| `context` | Background information for future sessions |
| `question` | Open questions needing resolution |
| `explore` | Investigation findings and research notes |
| `anchor` | Strata corpus references linking to external docs |
| `constellation` | Hub motes synthesizing a theme cluster |

### Link Types

**Planning links** create execution order:

| Link | Effect |
|------|--------|
| `depends_on` | Source depends on target (inverse: `blocks`) |
| `blocks` | Source blocks target (inverse: `depends_on`) |

**Memory links** create semantic connections:

| Link | Effect |
|------|--------|
| `relates_to` | Symmetric — both motes get the link |
| `builds_on` | Directional with index reverse reference |
| `contradicts` | Symmetric — flags interference in scoring |
| `supersedes` | Auto-deprecates the target mote |
| `caused_by` | Directional — traces causality |
| `informed_by` | Directional — traces influence |

### Scoring

When you run `mote prime` or `mote context`, motes are scored by combining:

- **Base weight** (0.0-1.0, set at creation)
- **Edge bonus** (0.1-0.3 depending on link type)
- **Status penalty** (-0.5 for deprecated)
- **Recency decay** (1.0x within 7 days, down to 0.4x after 90 days)
- **Retrieval strength** (+0.03 per access, capped at 0.15)
- **Salience boost** (+0.2 for failure/revert/hotfix origins)
- **Tag specificity** (rare tags score higher)
- **Interference penalty** (-0.1 per active contradiction)

### Three Processing Modes

| Mode | Latency | LLM? | Operations |
|------|---------|------|------------|
| **Hot path** | < 2s | No | Scoring, traversal, contradiction flagging, strata search |
| **Warm path** | < 10s | In-session | Crystallization prompts, link suggestions |
| **Dream cycle** | 1-10min | Headless batches | Semantic analysis, link inference, constellation evolution |

## CLI Reference

### Session Lifecycle

```bash
mote prime                  # Start: scored context for current work
mote session-end            # End: flush access counts, get suggestions
```

### Creating Motes

```bash
mote add --type=<type> --title="Title" [--tag=<t>]... [--weight=0.5] [--origin=normal] [--body="text"]
mote add --type=task --title="Title" --accept "criterion A" --accept "criterion B" --size=m --parent=<id>
```

Body can come from `--body`, stdin (`--body -` or pipe), or an editor (default).

Origins: `normal`, `failure`, `revert`, `hotfix`, `discovery`

Attach external references with `--ref provider:id[:url]` (e.g., `--ref github:42:https://github.com/org/repo/issues/42`). Ref IDs are indexed for BM25 search.

Beyond tasks, capture knowledge as it happens: `lesson` for debugging insights, `explore` for research findings, `decision` for architectural choices. Link them into the graph with `mote link`. See [docs/maintenance.md](docs/maintenance.md#knowledge-capture) for guidance.

Quick capture without opening an editor:

```bash
mote quick "your thought here"
```

### Querying

```bash
mote ls                             # All active motes
mote ls --type=task --status=active # Filtered
mote ls --ready                     # Tasks with no unfinished blockers
mote ls --stale                     # Motes not accessed in 90+ days
mote ls --json                      # Machine-readable JSON output
mote pulse                          # Active tasks sorted by weight
mote show <id>                      # Full detail with resolved links
mote search <query>                 # Full-text BM25 search
mote context <topic>                # Scored context via seed selection + BFS
mote context --planning <id>        # Dependency chain view
```

Most query commands accept `--json` for machine-readable output (ls, show, pulse, search, stats, tags, dream --review).

### Updating

```bash
mote update <id> --status=completed
mote update <id> --title="New title" --weight=0.8
mote update <id> --add-tag=newtag --add-tag=another
```

### Hierarchical Planning

```bash
mote plan <parent-id> --child "Step 1" --child "Step 2" [--sequential]
mote progress <parent-id>                     # Completion tracking with acceptance criteria
mote check <id> [index]                       # Mark acceptance criterion met (1-indexed)
mote check <id> --all                         # Mark all criteria met
```

The `--sequential` flag auto-chains children with `depends_on` links so each step blocks the next.

### Linking

```bash
mote link <source> <type> <target>
mote unlink <source> <type> <target>
```

### Strata (Reference Knowledge)

Ingest external documents for BM25 search without creating motes:

```bash
mote strata add ./docs/*.md --corpus=project-docs
mote strata query "authentication patterns"
mote strata query "error handling" --corpus=project-docs --top-k=10
mote strata ls
mote strata update project-docs     # Re-index changed files
mote strata stats
```

Supported file types: `.md`, `.txt`, `.go`, `.py`, `.js`, `.ts`, `.rs`, `.sh`, `.rb`, `.java`, `.c`, `.cpp`, `.h`, `.css`, `.html`, `.yaml`, `.yml`, `.json`, `.toml`, `.xml`

Chunking strategies: `heading-aware` (markdown/text), `function-level` (code), `sliding-window` (fallback).

### Dream Cycle (Automated Maintenance)

The dream cycle runs headless LLM analysis over your knowledge graph:

```bash
mote dream                  # Run full cycle
mote dream --dry-run        # Preview what would be analyzed
mote dream --review         # Interactive review of pending visions
```

It detects: missing links, contradictions, stale motes, overloaded tags, compression candidates, constellation evolution, co-access patterns, and summarization clusters.

The cycle produces draft visions (from Sonnet batches) which are reconciled by Opus into finalized visions in `visions.jsonl`. Review finalized visions with `--review` and accept, edit, reject, or defer each one. See [docs/maintenance.md](docs/maintenance.md) for the full workflow and vision type reference.

**Self-consistency voting:** Set `batching.self_consistency_runs: 3` in `.memory/config.yaml` to invoke each batch 3 times and keep only majority-agreed visions. This reduces hallucinated suggestions at the cost of additional LLM calls. The agreement fraction feeds into confidence scoring. Disabled by default (`1`).

### Import/Export

```bash
mote export                                           # Export all motes as JSONL
mote export --type=task --tag=auth --status=completed  # Filtered export
mote export --output=backup.jsonl                      # Write to file
mote import backup.jsonl                               # Import with content-hash dedup
mote import backup.jsonl --dry-run                     # Preview without writing
```

### Soft Delete

```bash
mote delete <id>            # Soft-delete (moves to .memory/trash/)
mote trash list             # Show trashed motes
mote trash restore <id>     # Restore a trashed mote
mote trash purge            # Permanently remove trashed motes
```

### Feedback

```bash
mote feedback <id> useful      # Mark a mote as useful (boosts retrieval strength)
mote feedback <id> irrelevant  # Mark as irrelevant (reduces scoring)
```

### Maintenance

```bash
mote doctor                 # Graph integrity checks
mote stats                  # Health dashboard
mote stats --decay-preview  # Show recency decay impact
mote tags audit             # Tag health analysis
mote index rebuild          # Rebuild edge index from frontmatter
mote constellation list     # Tag frequency overview
mote crystallize <id>       # Convert completed work to permanent knowledge
mote promote <id>           # Copy mote to global ~/.claude/memory/
```

For guidance on when to run each command and a suggested maintenance schedule, see [docs/maintenance.md](docs/maintenance.md).

### Onboarding & Migration

```bash
mote onboard                        # Auto-detect and migrate beads/MEMORY.md
mote onboard --global               # Set up global cross-project memory
mote onboard --dry-run              # Preview without writing
mote migrate MEMORY.md              # Convert flat markdown to motes
mote migrate MEMORY.md --dry-run    # Preview without writing
```

## Storage Layout

```
.memory/
├── nodes/*.md              # One mote per file (YAML frontmatter + markdown body)
├── index.jsonl             # Edge index (rebuilt from motes, self-healing)
├── config.yaml             # All scoring, priming, dream, strata config
├── constellations.jsonl    # Constellation cluster records
├── .access_batch.jsonl     # Batched access updates (flushed at session-end)
├── trash/                 # Soft-deleted motes (restorable)
├── dream/
│   ├── log.jsonl           # Dream run history
│   ├── visions.jsonl       # Pending visions from dream analysis
│   ├── scan_state.json     # Content-hash cache for incremental prescanning
│   └── auto_applied.jsonl  # Auto-applied dream visions log
└── strata/<corpus>/
    ├── manifest.json       # Source paths, hashes, chunk count
    ├── chunks.jsonl        # Chunked document content
    └── bm25.json           # BM25 search index
```

### Mote File Format

```yaml
---
id: myproject-t1abc2def
type: task
status: active
title: Implement user authentication
tags: [auth, api, security]
weight: 0.7
origin: normal
size: l
created_at: 2026-03-01T10:00:00Z
depends_on: [myproject-t1xyz9876]
parent: myproject-t0parent1
acceptance:
  - Login endpoint returns signed JWT
  - Middleware validates token on protected routes
  - Refresh token rotation
acceptance_met: [false, false, false]
external_refs:
  - provider: github
    id: "42"
    url: https://github.com/org/repo/issues/42
---
JWT-based authentication for the REST API.
```

## Configuration

All configuration lives in `.memory/config.yaml`. See [docs/configuration.md](docs/configuration.md) for the full reference with sample configurations.

`mote init` generates a config with sensible defaults. Every field is optional — missing values fall back to defaults.

## Version History

- **v0.3.0** — Beads feature transfer: JSONL import/export, external refs, `--json` flags, scan cache, cluster summarization
- **v0.2.0** — Hierarchical planning: parent/child tasks, acceptance criteria, `plan`/`progress`/`check` commands
- **v0.1.x** — Core system: mote CRUD, graph linking, scoring, context/prime, dream cycle, strata, constellations

## Design Principles

- **Files are the database.** Markdown files with YAML frontmatter. No SQLite, no network services.
- **Atomic writes everywhere.** All file writes use write-to-temp-then-rename for POSIX atomicity.
- **Edge index is a cache.** Derived from mote frontmatter, self-healing via `mote index rebuild`.
- **Reads never write.** Access counts are batched in `.access_batch.jsonl` and flushed at session-end.
- **No embeddings.** Search uses BM25 (~150 LOC). No vector database, no API calls for retrieval.
- **LLM is optional.** Only the dream cycle requires an external LLM (via `claude` CLI). All other operations are pure computation.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `gopkg.in/yaml.v3` | YAML frontmatter parsing |

Everything else is Go stdlib.

## Development

```bash
make build      # Build binary
make test       # Run all tests
make vet        # Static analysis
make install    # Build and copy to ~/.local/bin/
make clean      # Remove binary
```
