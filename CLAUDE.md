# CLAUDE.md — Motes Project

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in `.memory/`.

**See `~/.claude/CLAUDE.md` for the full motes workflow** (task tracking, retrieval, capture, maintenance). That is the canonical reference — do not duplicate workflow instructions here.

## Landing the Plane (Session Completion)

> **The plane has not landed until `git push` succeeds.** Complete ALL eight
> steps, in order. Work is NOT complete until step 8 emits a handoff prompt.

1. **File issues** for remaining work
2. **Run quality gates** (if code changed): `go test ./...`, `go vet ./...`
3. **Update task status** — close finished work
4. **Push to remote** (MANDATORY):
   ```bash
   git pull --rebase && git push && git status
   ```
   If push fails, resolve and retry. **Resolve = `git pull --rebase`,
   NOT `git push --force`.**
5. **Verify** — all changes committed AND pushed:
   - `git status` → "nothing to commit, working tree clean"
   - `git log @{u}..HEAD` → empty (no unpushed commits)
   - `git stash list` → empty
   - `git diff --stat HEAD~1` → shows the just-pushed diff
6. **Clean local repo state**: `git stash clear`
7. **Clean remote refs**: `git remote prune origin`
8. **Hand off** — emit the follow-up-prompt for the next session
   (see template below)

### ⚠ Anti-patterns (the plane has not landed)

- Saying "ready to push when you are" — **YOU** must push, not the human
- Stopping after commit but before push
- Calling a feature done when tests are skipped without a `.test-skip` ref
- Leaving WIP in a stash instead of either committing or discarding it
- Closing a task whose acceptance criteria still have unmet checkboxes
- Using `git push --force` (or `--force-with-lease`) on shared branches to
  make a push "succeed" — **this is not landing, it is crashing**

### Hand-off follow-up prompt

> Continue work on mote-<id>: <title>. Done: <one line>. Next: <one line>.

Example:

> Continue work on mote-T1abc: Implement two-zone header format. Done:
> updated `cmd_show.go` to emit `<icon> <id> · <title> [w0.7 · ACTIVE]`.
> Next: extend the same renderer to `cmd_ls.go --ready` per STORY-HDRZ-001.

### Example session (copy verbatim)

```bash
# 1-3. Files, gates, statuses — task-specific
# 4. Push
git pull --rebase && git push && git status
# 5. Verify
git status
git log @{u}..HEAD
git stash list
git diff --stat HEAD~1
# 6-7. Clean
git stash clear
git remote prune origin
# 8. Hand off
echo "Continue work on mote-<task-id>: <title>. Done: <one line>. Next: <one line>."
```

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
- `docs/providers.md` — Multi-provider dream cycle setup (claude-cli, openai, gemini Vertex AI)
- `docs/configuration.md` — Full `.memory/config.yaml` field reference
- `docs/UI_PHILOSOPHY.md` — CLI design rules: icons, fix-op consolidation, command-count ceiling
- `docs/example-codex-config.md` — Codex `.codex/hooks.json` reference
- `docs/example-gemini-config.md` — Gemini CLI `.gemini/settings.json` reference (timeouts in ms!)
- `AGENTS.md` — Cross-agent contract for any AI coding agent working in this repo
- `CODEX.md` — OpenAI Codex specifics
- `GEMINI.md` — Gemini CLI specifics (imports AGENTS.md via `@AGENTS.md`)

## Build & Development Commands

```bash
go build -o mote ./cmd/mote    # Build
go test ./...                   # Run all tests
go test ./internal/scoring      # Run tests for a single package
go test -run TestScoreEngine    # Run a specific test
go vet ./...                    # Lint
```

See [docs/internals.md](docs/internals.md) for architecture, storage layout, and design decisions.

## Error Recovery

When debugging unfamiliar errors in motes itself, search for prior lessons first: `mote search "<error>" --type=lesson`. The full trigger/skip rules are in the `/mote-retrieve` skill.

## Dispatching subagents

Before spawning a subagent to enact a mote, read execution metadata before prose — see `docs/agents-guide.md#read-execution-metadata-before-prose`. A running subagent cannot change its model or reasoning effort after launch, so `mote show <id> --execution-only | jq .` decides dispatch.

## Motes

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in `.memory/`.

Lifecycle hooks automate `mote prime` (session start/resume/compaction) and `mote session-end` (session stop) — do not run these manually.

**See `~/.claude/CLAUDE.md` for the full motes workflow** (task tracking, retrieval, capture, maintenance).

**Do NOT use** markdown files, TodoWrite, TaskCreate, or external issue trackers for tracking work.
