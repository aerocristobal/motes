# /mote-retrieve

Intelligent context retrieval. Use when you need project knowledge beyond what `mote prime` surfaced.

## Active Tasks
!mote pulse --compact

## Available Strata
!mote strata ls --compact

## Auto-Query on Errors

When you encounter an unfamiliar error during work, search for prior lessons before debugging from scratch.

```bash
mote search "<error message or key phrase>" --type=lesson
```

### Trigger

- API returns unexpected error code or message → search the error code/message
- Build fails with non-obvious error → search the failing module or error text
- Configuration rejected or ignored at runtime → search the config key or symptom
- Dependency conflict or version mismatch → search the package name and version constraint

### Skip

- Syntax errors, typos, missing semicolons — fix directly
- Missing imports or undefined names — fix directly
- File-not-found for paths you just typed — fix the path
- Test assertion failures on code you just changed — read the diff

### Session De-duplication

Track which error patterns you have already queried in this session. If the same error class recurs (same error code, same failing module, same config key), use the results from your earlier query instead of searching again.

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
| Search past errors | `mote search "<error>" --type=lesson` |
| Give relevance feedback | `mote feedback <id> useful\|irrelevant` |
