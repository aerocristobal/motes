# /mote-retrieve

Intelligent context retrieval. Use when you need project knowledge beyond what `mote prime` surfaced.

## Active Tasks
!mote pulse --compact

## Available Strata
!mote strata ls --compact

## Decision Tree

### "What do we know about X?" → Graph Traversal

```bash
mote context <topic>
```

Walks the knowledge graph from topic-matching motes, following semantic links. Best for thematic exploration — shows related decisions, lessons, and explores connected by links.

### "Where did we mention Y?" → Full-Text Search

```bash
mote search <query>                              # All motes
mote search <query> --type=lesson                # Filter by type
mote search <query> --exclude-status=deprecated  # Exclude deprecated
```

BM25 keyword search across all mote titles and bodies. Supports `--type`, `--tag`, `--status`, and `--exclude-status` filters. Best for finding specific mentions, exact phrases, or keywords you remember.

### "What does the spec say about Z?" → Reference Docs

```bash
mote strata query <topic>
```

Searches ingested reference documents (PRDs, architecture docs, specs). Best for looking up requirements, design specs, or external documentation.

### "What depends on this task?" → Planning View

```bash
mote context --planning <id>
```

Shows the dependency chain for a task — what blocks it, what it blocks, and execution order.

### "What changed on this mote?" → Change History

```bash
mote diff <id>
```

Shows the change history for a mote — edits, status transitions, and metadata updates.

## Follow-Up Actions

After retrieving context:

- **Inspect a mote:** `mote show <id>` — full content with metadata
- **Mark as useful:** `mote feedback <id> useful` — boosts future scoring
- **Mark as irrelevant:** `mote feedback <id> irrelevant` — suppresses future scoring
- **Link motes together:** `mote link <id1> relates_to <id2>`
- **Validate graph integrity:** `mote doctor` — check for broken links, orphans, inconsistencies
- **View retrieval stats:** `mote stats` — graph metrics and health dashboard

## Quick Reference

| I want to... | Command |
|--------------|---------|
| Explore a topic | `mote context <topic>` |
| Find a keyword | `mote search <query>` |
| Check reference docs | `mote strata query <topic>` |
| See task dependencies | `mote context --planning <id>` |
| Read full mote | `mote show <id>` |
| See mote change history | `mote diff <id>` |
| Check graph health | `mote doctor` |
| View graph metrics | `mote stats` |
| Give relevance feedback | `mote feedback <id> useful\|irrelevant` |
