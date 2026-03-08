# CLAUDE.md

## Motes

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in `.memory/`.

**Do NOT use** markdown files, TodoWrite, TaskCreate, or external issue trackers for tracking work.

### Session Start

***Run `mote prime` at the start of every session for scored, relevant context.***

Prime outputs: active tasks, recent decisions, lessons, explores, echoes (previously useful motes), and contradiction alerts. It auto-parses your git branch as keywords.

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

Use `context` for thematic exploration, `search` for specific keywords, `strata query` for ingested reference docs.

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

    # 1. Create parent task
    mote add --type=task --title="Goal" --size=l --tag=topic

    # 2. Decompose into subtasks (inherits parent tags)
    mote plan <parent-id> --child "Step 1" --child "Step 2" --sequential

    # 3. Add acceptance criteria
    mote update <child-id> --accept "criterion A" --accept "criterion B"

    # 4. Check progress
    mote progress <parent-id>

    # 5. Find next ready work
    mote ls --ready

    # 6. Mark criteria met
    mote check <id> <index>

    # 7. Complete subtask
    mote update <id> --status=completed

### Capturing Knowledge

Capture when you encounter:

| Trigger | Type | Command |
|---------|------|---------|
| Non-obvious choice made | decision | `mote add --type=decision --title="Summary" --tag=topic --body "Rationale"` |
| Gotcha or surprise discovered | lesson | `mote add --type=lesson --title="Summary" --tag=topic --body "Details"` |
| Researched alternatives | explore | `mote add --type=explore --title="Summary" --tag=topic --body "Findings"` |
| Quick thought | (auto) | `mote quick "your sentence here"` |
| Task done with learnings | (meta) | `mote crystallize <id>` |

**Always link after capturing.** Check motes already in context (primed, recently created, discussed this session) and link them: `mote link <id1> relates_to <id2>`. Use `[[mote-id]]` wikilinks in body text when referencing existing motes.
Give feedback on surfaced motes: `mote feedback <id> useful` or `mote feedback <id> irrelevant`

**Tag strategy:** Rare, specific tags beat generic ones. `bm25-scoring` > `search` > `code`.

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
- `docs/internals.md` — Architecture, storage layout, and design decisions

## Build & Development Commands

```bash
go build -o mote ./cmd/mote    # Build
go test ./...                   # Run all tests
go test ./internal/scoring      # Run tests for a single package
go test -run TestScoreEngine    # Run a specific test
go vet ./...                    # Lint
```

See [docs/internals.md](docs/internals.md) for architecture, storage layout, and design decisions.
