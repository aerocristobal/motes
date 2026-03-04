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

Run `mote session-end` for access flush and maintenance suggestions.

Run `mote dream` periodically for automated maintenance. Review with `mote dream --review`.
```
