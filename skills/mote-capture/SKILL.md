# /mote-capture

Interactive knowledge capture. Use when you detect a capture-worthy moment during work, or invoke directly.

## When to Trigger

- A non-obvious design choice was just made → `decision`
- Something unexpected happened (gotcha, surprise) → `lesson`
- Alternatives were researched or evaluated → `explore`
- A quick thought worth preserving → `mote quick`
- A completed task with reusable learnings → `mote crystallize`

## Current Tags
!mote tags --compact

## Recent Active Tasks (for linking)
!mote ls --type=task --status=active --compact

## Workflow

1. **Identify type** from context: decision, lesson, or explore
2. **Draft title** — concise, searchable summary
3. **Suggest tags** — prefer tags from Current Tags above; use rare/specific over generic
4. **Suggest links** — find related motes with `mote search "<keywords>"` and propose links
5. **Create the mote:**

```bash
mote add --type=<type> --title="<title>" --tag=<tag1> --tag=<tag2> --body "<body>"
```

6. **Link to related motes** (if found in step 4):

```bash
mote link <new-id> relates_to <related-id>
mote link <new-id> builds_on <prior-id>
```

7. **Extract learnings from completed tasks** (when a task has reusable insights):

```bash
mote crystallize <id>                    # Extract learnings from a completed task
mote crystallize --candidates            # Find tasks with unextracted learnings
mote crystallize <id> --origin=failure   # Tag the origin (failure/discovery/revert/hotfix)
```

## Tag Guidelines

- Use 1-3 tags per mote
- Specific beats generic: `bm25-scoring` > `search` > `code`
- Reuse existing tags when they fit; only create new ones for genuinely new topics

## Type Selection Guide

| Signal | Type | Example |
|--------|------|---------|
| "We chose X because..." | decision | Architecture choice, library selection, API design |
| "Watch out for..." / "TIL..." | lesson | Unexpected behavior, debugging insight, gotcha |
| "I looked into..." | explore | Compared approaches, researched options, benchmarked |
