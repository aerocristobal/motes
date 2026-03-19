# CLAUDE.md — Motes Project

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in `.memory/`.

**See `~/.claude/CLAUDE.md` for the full motes workflow** (task tracking, retrieval, capture, maintenance). That is the canonical reference — do not duplicate workflow instructions here.

## Landing the Plane (Session Completion)

**When ending a work session**, complete ALL steps. Work is NOT complete until `git push` succeeds.

1. **File issues** for remaining work
2. **Run quality gates** (if code changed): `go test ./...`, `go vet ./...`
3. **Update task status** — close finished work
4. **Push to remote** (MANDATORY):
   ```bash
   git pull --rebase && git push && git status
   ```
5. **Verify** — all changes committed AND pushed
6. **Hand off** — provide context for next session

**CRITICAL:** Never stop before pushing. Never say "ready to push when you are" — YOU must push. If push fails, resolve and retry.

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

## Motes

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in `.memory/`.

Lifecycle hooks automate `mote prime` (session start/resume/compaction) and `mote session-end` (session stop) — do not run these manually.

**See `~/.claude/CLAUDE.md` for the full motes workflow** (task tracking, retrieval, capture, maintenance).

**Do NOT use** markdown files, TodoWrite, TaskCreate, or external issue trackers for tracking work.
