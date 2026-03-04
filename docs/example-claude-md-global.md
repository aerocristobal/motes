# Example: Global CLAUDE.md

This shows what `~/.claude/CLAUDE.md` looks like when configured for motes. This replaces any previous bd/beads instructions.

---

```markdown
# User Instructions

## Session Workflow

### Session Start
Run `mote prime` at the start of every session.

### During Work
- Use `mote add` to capture decisions, lessons, and findings
- Use `mote ls --ready` to find available work
- Use `mote update <id> --status=completed` to close tasks

### Session End
Run `mote session-end` before ending a session. This flushes access counts and provides maintenance suggestions.

### Periodic Maintenance
Run `mote dream` periodically for automated knowledge graph maintenance. Review proposed changes with `mote dream --review`.

## Task Tracking

**Use motes for ALL task tracking.** Before starting non-trivial work, create a task mote:

    mote add --type=task --title="Summary" --tag=topic --body "What and why"

Close it when done:

    mote update <id> --status=completed

**Do NOT use** TodoWrite, TaskCreate, or markdown files as substitutes.

## Cross-Project Knowledge

Promote valuable motes to the global layer:

    mote promote <id>

Global motes are stored in `~/.claude/memory/` and surface during `mote prime` in any project.
```
