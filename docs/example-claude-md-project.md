# Example: Project-level CLAUDE.md

This shows what a project `CLAUDE.md` looks like after running `mote init` or `mote onboard`. The `## Motes` section is auto-generated. Everything else is project-specific.

---

```markdown
# CLAUDE.md

## Project Overview

MyProject is a REST API for managing widgets. Built with Go, PostgreSQL, and Redis.

## Build & Development

\`\`\`bash
go build -o myproject ./cmd/myproject
go test ./...
go vet ./...
\`\`\`

## Key Conventions

- All handlers go in `internal/handlers/`
- Database queries use sqlc-generated code in `internal/db/`
- Config loaded from environment variables via `internal/config/`

## Motes

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in `.memory/`.

**Do NOT use** markdown files, TodoWrite, TaskCreate, or external issue trackers for tracking work.

### Session Start

Run `mote prime` at the start of every session for scored, relevant context.

Prime outputs: active tasks, recent decisions, lessons, explores, echoes, and contradiction alerts. It auto-parses your git branch as keywords.

Focus priming on a topic: `mote prime <topic>`
Inspect a surfaced mote: `mote show <id>`

### Mid-Session Retrieval

When you need context beyond what prime surfaced:

| Need | Command | Example |
|------|---------|---------|
| Graph traversal — "What do we know about X?" | `mote context <topic>` | `mote context authentication` |
| Full-text search — "Where did we mention Y?" | `mote search <query>` | `mote search "retry logic"` |
| Reference docs — "What does the spec say about Z?" | `mote strata query <topic>` | `mote strata query scoring` |
| Dependency chain view | `mote context --planning <id>` | `mote context --planning proj-t1abc` |

### Task Tracking & Planning

Find available work:

    mote ls --ready           # Tasks with no unfinished blockers
    mote pulse                # Active tasks sorted by weight

Create tasks with dependency links:

    mote add --type=task --title="Summary" --tag=topic --body "What and why"
    mote link <story-id> depends_on <epic-id>
    mote update <id> --status=completed

### Planning Workflow

For multi-step work, use hierarchical task decomposition:

    mote add --type=task --title="Goal" --size=l --tag=topic
    mote plan <parent-id> --child "Step 1" --child "Step 2" --sequential
    mote update <child-id> --accept "criterion A"
    mote progress <parent-id>
    mote check <id> <index>
    mote update <id> --status=completed

### Capturing Knowledge

Capture when you encounter:

| Trigger | Type | Command |
|---------|------|---------|
| Non-obvious choice made | decision | `mote add --type=decision --title="Summary" --tag=topic --body "Rationale"` |
| Gotcha or surprise discovered | lesson | `mote add --type=lesson --title="Summary" --tag=topic --body "Details"` |
| Researched alternatives | explore | `mote add --type=explore --title="Summary" --tag=topic --body "Findings"` |
| Quick thought | (auto) | `mote quick "your sentence here"` |

After capturing, link related motes: `mote link <id1> relates_to <id2>`
Give feedback on surfaced motes: `mote feedback <id> useful` or `mote feedback <id> irrelevant`

**Tag strategy:** Rare, specific tags beat generic ones.

### Session End

Run `mote session-end` for access flush and maintenance suggestions.

Run `mote dream` periodically for automated maintenance. Review with `mote dream --review`.

### Multi-Agent Support

Motes supports concurrent access from multiple agents. Hooks are auto-installed by `mote onboard` (including a Stop hook for guaranteed session cleanup). Re-run `mote onboard` after upgrading to install new hooks.
```
