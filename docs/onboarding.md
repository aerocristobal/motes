# Getting Started with Motes

This guide covers how to adopt motes in a new or existing project.

## Fresh Start

For a brand new project:

```bash
mote init
```

This creates:
- `.memory/` directory with `nodes/`, `dream/`, `strata/` subdirs
- `.memory/config.yaml` with sensible defaults
- `.memory/index.jsonl` (empty edge index)
- Appends a `## Motes` section to your `CLAUDE.md`

Start creating motes immediately:

```bash
mote add --type=task --title="Set up project" --tag=setup --body "Initial scaffolding"
```

## Transitioning from MEMORY.md

If your project has a `MEMORY.md` file with accumulated knowledge:

**Option A: Automatic (recommended)**

```bash
mote onboard
```

`onboard` auto-detects `MEMORY.md` in the project root, parses it into sections, infers mote types (context, lesson, decision), and creates individual motes. The original file is archived as `MEMORY.md.migrated.<date>`.

**Option B: Explicit**

```bash
mote migrate MEMORY.md
```

Same migration logic, but you specify the file path. Useful when your MEMORY.md is in a non-standard location.

**Preview first:**

```bash
mote onboard --dry-run
mote migrate MEMORY.md --dry-run
```

## Transitioning from Beads

If your project uses `.beads/` for issue tracking:

```bash
mote onboard
```

`onboard` detects `.beads/issues.jsonl` and imports open issues as task motes. Beads priorities map to mote weights (P0 = 1.0, P4 = 0.3). Bug-type issues become lesson motes with `failure` origin.

By default only open issues are imported. To include closed issues:

```bash
mote onboard --include-closed
```

### What gets auto-installed

`mote init` and `mote onboard` automatically:

- **Install Claude Code hooks** — `SessionStart` and `PreCompact` hooks for `mote prime` in `~/.claude/settings.json`
- **Install mote skills** — `mote-capture` and `mote-retrieve` skills to `~/.claude/skills/`
- **Migrate bd hooks** — Replaces `bd prime` → `mote prime` and `bd sync` → `mote session-end` in existing hooks

### Post-onboard cleanup

After verifying the import:

1. **Remove `.beads/`** — Use `mote onboard --cleanup` to auto-remove, or delete manually once satisfied

## Going Global

To set up global (cross-project) memory:

```bash
mote onboard --global
```

This creates `~/.claude/memory/` with the same structure as project memory. If `~/.beads/` exists, global beads issues are imported too.

Use `mote promote <id>` to copy project-local motes to the global layer for cross-project access.

## What Happens During Onboard

`mote onboard` runs these steps in order:

1. **Detect** — Scans for `.beads/issues.jsonl`, `MEMORY.md`, `.memory/`, and `CLAUDE.md`
2. **Report** — Prints what was found and what will happen
3. **Init** — Creates `.memory/` if it doesn't exist (same as `mote init`)
4. **Migrate MEMORY.md** — Parses sections, creates typed motes, archives original
5. **Import beads** — Converts open issues to task motes (idempotent — won't duplicate on re-run)
6. **Rebuild index** — Regenerates `index.jsonl` from all mote frontmatter
7. **Update CLAUDE.md** — Appends the `## Motes` section if not already present

## Workflow Cheat Sheet

### Session start

```bash
mote prime
```

Outputs scored, relevant context for the current work. Shows active tasks, recent decisions, and related knowledge.

### During work

```bash
# Create motes for decisions, lessons, and discoveries
mote add --type=decision --title="Use JWT" --tag=auth --body "Stateless, scales horizontally"
mote add --type=lesson --title="Retry logic needed" --tag=api --body "External API drops connections"

# Link related knowledge
mote link <id1> relates_to <id2>
mote link <story-id> depends_on <epic-id>

# Find available work
mote ls --ready
mote pulse

# Query context on a topic
mote context authentication
```

### Session end

```bash
mote session-end
```

Flushes batched access counts, suggests crystallization candidates, and provides maintenance hints.

Every few sessions, run the dream cycle for deeper automated maintenance:

```bash
mote dream              # Analyze graph, produce visions
mote dream --review     # Review and apply/reject each vision
```

The dream cycle detects missing links, stale content, contradictions, and more. Each finding becomes a "vision" that you review interactively. See [docs/maintenance.md](docs/maintenance.md) for the full maintenance workflow.

## Example CLAUDE.md Configurations

- [Project-level CLAUDE.md](example-claude-md-project.md) — What a project CLAUDE.md looks like with motes
- [Global CLAUDE.md](example-claude-md-global.md) — What `~/.claude/CLAUDE.md` looks like with motes
