# Maintenance Guide

This guide covers the three activities that keep your knowledge graph healthy: **dream cycles**, **knowledge capture**, and **graph hygiene**.

## Dream Cycle Workflow

The dream cycle runs headless LLM analysis over your knowledge graph, detecting missing links, contradictions, stale motes, and more. It's the deepest maintenance operation motes offers.

### Running a Dream Cycle

```bash
mote dream              # Run the full cycle
mote dream --dry-run    # Preview what would be analyzed (no LLM calls)
```

A dream run goes through four stages:

1. **PreScan** — Deterministic analysis finds candidates: orphan motes, stale content, overloaded tags, compression opportunities, co-access patterns
2. **Batch Reasoning** — Candidates are grouped into batches and sent to Claude Sonnet for analysis. Each batch produces draft visions written to `visions_draft.jsonl`
3. **Reconciliation** — All draft visions and the lucid log are sent to Claude Opus, which deduplicates, resolves conflicts, and produces finalized visions
4. **Write** — Finalized visions are written to `visions.jsonl` for review

### The Two Vision Files

The dream cycle produces two files in `.memory/dream/`:

| File | Contents | Lifecycle |
|------|----------|-----------|
| `visions_draft.jsonl` | Raw visions from Sonnet batches (pre-reconciliation) | Cleared at the start of each dream run |
| `visions.jsonl` | Finalized visions after Opus reconciliation | Persists until you review and apply them |

If reconciliation is disabled or fails, draft visions are promoted directly to `visions.jsonl`.

### Reviewing Visions

After a dream run, review the finalized visions:

```bash
mote dream --review
```

This starts an interactive review. For each vision you can **accept**, **edit**, **reject**, or **defer** (skip for now).

### Vision Types

Each vision type performs a specific action when accepted:

| Type | What It Detects | What Accept Does |
|------|----------------|------------------|
| `link_suggestion` | Missing semantic connection between motes | Creates a link via `mote link` and adds a wiki-link reference in the source mote's body |
| `staleness` | Mote not accessed in a long time, content outdated | Deprecates the mote (sets status to `deprecated`) |
| `contradiction` | Two motes making conflicting claims | Creates a `contradicts` link between them |
| `tag_refinement` | Mote has vague or overloaded tags | Replaces the mote's tags with refined ones |
| `compression` | Mote body is verbose or redundant | Replaces the mote's body with a compressed version |
| `constellation` | Cluster of motes sharing a theme but no hub | Creates a new constellation mote linking all members |
| `signal` | Recurring co-access pattern worth codifying | Adds a `co_access` signal to `config.yaml` for scoring |

### Tips

- **Review promptly.** Visions in `visions.jsonl` persist across dream runs. New runs overwrite the file, so unreviewed visions from a previous run are lost.
- **Use dry-run first.** `mote dream --dry-run` shows what the prescanner found without making LLM calls. Useful for checking if a dream run is worth the cost.
- **Check the run log.** `.memory/dream/log.jsonl` records each run's timestamp, batch count, and vision count.

## Knowledge Capture

Motes are most valuable when you capture knowledge as it happens. The key types for knowledge capture:

### When to Create Each Type

| Situation | Mote Type | Example |
|-----------|-----------|---------|
| Solved a hard bug | `lesson` | "Retry logic needed for flaky API" |
| Made an architectural choice | `decision` | "Use JWT over sessions — stateless, scales horizontally" |
| Investigated alternatives | `explore` | "Compared SQLite vs flat files for storage" |
| Established background info | `context` | "Auth service runs on port 8080 in dev" |
| Hit an open question | `question` | "Should we support multi-tenant isolation?" |

### Linking Into the Graph

New motes are most useful when connected to existing knowledge:

```bash
# A lesson learned while working on a task
mote link <lesson-id> caused_by <task-id>

# An exploration that informed a decision
mote link <decision-id> informed_by <explore-id>

# Two motes covering related ground
mote link <id1> relates_to <id2>

# A newer decision replacing an older one
mote link <new-id> supersedes <old-id>
```

Unlinked motes still appear in `mote prime` via tag matching and recency, but linked motes score higher through edge bonuses and graph traversal.

## Graph Hygiene

Periodic health checks catch problems before they accumulate.

### `mote doctor`

Runs integrity checks on the knowledge graph:

```bash
mote doctor
```

Reports: orphan links (pointing to deleted motes), missing frontmatter fields, malformed IDs, and index inconsistencies.

### `mote stats`

Dashboard view of graph health:

```bash
mote stats
mote stats --decay-preview    # Show recency decay impact on scores
```

Look for: high never-accessed count (motes that aren't surfacing), pending vision count, tag distribution skew.

### `mote tags audit`

Analyzes tag health:

```bash
mote tags audit
```

Flags: singleton tags (only one mote), overloaded tags (too many motes), and similar tags that could be merged.

### `mote index rebuild`

Rebuilds the edge index from mote frontmatter:

```bash
mote index rebuild
```

Use after: manual file edits, bulk imports, or if `mote doctor` reports index inconsistencies. The index is a cache — rebuilding it is always safe.

### `mote crystallize`

Converts completed task motes into permanent knowledge:

```bash
mote crystallize <id>
```

Prompts you to extract lessons and decisions from a finished task before archiving it.

## Suggested Schedule

| When | What |
|------|------|
| Every session start | `mote prime` |
| During work | Capture decisions, lessons, and findings with `mote add` |
| Every session end | `mote session-end` |
| Every few sessions | `mote dream` + `mote dream --review` |
| Weekly or monthly | `mote doctor`, `mote stats`, `mote tags audit` |
