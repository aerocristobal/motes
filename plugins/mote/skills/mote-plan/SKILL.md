# /mote-plan

Multi-step planning and task decomposition. Use when breaking down complex work into trackable sub-tasks with dependencies.

## When to Trigger

- Starting a feature or epic that needs multiple steps
- Breaking a large task into smaller, ordered work items
- Need to track progress across dependent tasks

## Active Tasks
!mote pulse --compact

## Ready Work
!mote ls --ready --compact

## Workflow

### 1. Create Parent Task

```bash
mote add --type=task --title="Epic: Feature Name" --tag=topic --body "High-level goal and acceptance criteria"
```

### 2. Decompose into Child Tasks

```bash
# Add child tasks linked to parent
mote plan <parent-id> --child --title="Step 1: Description" --size=s
mote plan <parent-id> --child --title="Step 2: Description" --size=m

# Or create sequential (auto-linked) children
mote plan <parent-id> --sequential \
  --title="Step 1" --size=s \
  --title="Step 2" --size=m \
  --title="Step 3" --size=l
```

### 3. Set Acceptance Criteria

```bash
mote update <id> --accept "Given X, When Y, Then Z"
```

### 4. Track Progress

```bash
mote progress <parent-id>          # Completion status of parent and children
mote ls --ready                    # Tasks with no unfinished blockers
mote pulse                         # Active tasks sorted by weight
mote check <id>                    # Verify acceptance criteria
```

### 5. Complete Tasks

```bash
mote update <id> --status=completed
```

## Size Guide

| Size | Scope | Example |
|------|-------|---------|
| `xs` | < 15 min, single change | Fix a typo, add a flag |
| `s`  | 15-60 min, focused task | Add a command, write tests |
| `m`  | 1-3 hours, multiple files | New feature, refactor module |
| `l`  | Half day, cross-cutting | New subsystem, integration |
| `xl` | Full day+, decompose further | Epic-level, should be broken down |

## Tips

- **Keep parents as epics** — they track progress, children do the work
- **Use `--ready`** to find what's unblocked and can start now
- **Check `mote progress`** after compaction to re-orient
- **Link related tasks** with `mote link <id1> depends_on <id2>`
