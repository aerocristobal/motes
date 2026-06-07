# AGENTS.md — Working with motes

This file is loaded into your context at session start. Keep it short.
For extended background, common pitfalls, and the agent-native principle,
read `docs/agents-guide.md`.

## Setup

```bash
make build       # go build -o mote ./cmd/mote
make test        # go test ./...
make vet         # go vet ./...
```

All three must pass before any commit. CI gates on the same commands.

## Workflow contract

1. Before any non-trivial code change:
   `mote add --type=task --title="..." --tag=topic --body "what and why"`
2. On completion:
   `mote update <id> --status=completed`
3. Capture knowledge as you find it, not after the fact:
   `mote add --type=lesson|decision|explore --title="..." --body "..."`
   Link findings into the graph: `mote link <id> caused_by <task-id>`
4. On unfamiliar errors, search prior lessons before debugging:
   `mote search "<phrase>" --type=lesson`
5. `mote ls --ready` hides motes with a future `defer_until`; pass
   `--include-deferred` to surface them, or `mote ls --overdue` for past-due
   work. Set with `mote add|update --due=<spec>` / `--defer=<spec>`.
6. A session is **not** done until `git push` succeeds.
   Never declare done without pushing.

Do not use ad-hoc TODO files, in-tree task lists, markdown checklists, or
external trackers as a substitute for task motes. The graph is the system
of record.

## Project conventions

- Go 1.25+. Stdlib + `github.com/spf13/cobra` + `gopkg.in/yaml.v3` only.
  Adding a dependency requires a `decision` mote first.
- MIT SPDX header at the top of every `.go` file.
- Tabs for indentation. Verify with `gofmt`.
- All `.memory/` writes go through `core.AtomicWrite` (write-temp, rename).
  Reads never write — access counts are batched and flushed at session-end.
- Storage: markdown + YAML frontmatter under `.memory/nodes/`. No database.

## Layout — where things go

| Want to... | Look in / use... |
|---|---|
| Add a CLI command | `cmd/mote/cmd_<name>.go` + `init()` registers with `rootCmd` |
| Add a mote field | `internal/core/mote.go` (`Mote` struct) — also update index, search, frontmatter parser |
| Set orchestration hints | `--execution-agent-type`, `--execution-suggested-model`, `--execution-reasoning-effort` (`low\|medium\|high`), `--execution-mode` (`local\|delegated\|parallel`), `--execution-parallel-group` on `mote add` / `mote update` |
| Dispatch a subagent | Read execution metadata before prose — see `docs/agents-guide.md#read-execution-metadata-before-prose` (`mote show <id> --execution-only`) |
| Tune scoring | `internal/scoring/` + `ScoringConfig` in `internal/core/config.go` |
| Tune dream cycle | `internal/dream/` + `DreamConfig` in `internal/core/config.go` |
| Add a config field | `internal/core/config.go` struct + `DefaultConfig()` + `internal/core/config_yaml.go` for user-facing comments + `docs/configuration.md` row |
| Add a doctor check | `runDoctorChecks` (errors) or `runDoctorAdvisories` (warnings) in `cmd/mote/cmd_doctor.go` |
| Add an LLM provider | New `Invoker` impl in `internal/dream/<name>_invoker.go` + factory case in `invoker.go` + pricing row in `pricing.go` |

## LLM backend

The dream cycle is the only LLM-using subsystem. It supports five backends
configurable per stage in `.memory/config.yaml`:

| Backend | Auth | Notes |
|---------|------|-------|
| `claude-cli` | `oauth` placeholder | Default. Shells out to the `claude` binary. |
| `openai` | env var name (e.g. `OPENAI_API_KEY`) or literal | Calls `api.openai.com/v1/chat/completions`. |
| `gemini` | `vertex-ai` sentinel | Vertex AI ADC via `gcloud auth print-access-token`. Requires `gcp_project` in `options`. |
| `codex-cli` | `oauth` placeholder | Shells out to `codex exec`; inherits whatever `codex login` set up. |
| `gemini-cli` | `oauth` placeholder | Shells out to `gemini -p`; inherits whatever the gemini CLI is logged in as. |

Full reference: `docs/providers.md`.

Agent-specific guides:
- `CLAUDE.md` — Claude Code (auto-loads its own file)
- `CODEX.md` — OpenAI Codex; AGENTS.md is auto-loaded by Codex per its spec
- `GEMINI.md` — Gemini CLI; imports this file via `@AGENTS.md` syntax

## Read also

- `README.md` — concepts, full CLI reference
- `docs/agents-guide.md` — extended pitfalls, common patterns, "agent-native" principle
- `docs/internals.md` — architecture, storage layout, design decisions
- `docs/configuration.md` — every `.memory/config.yaml` field explained
- `docs/UI_PHILOSOPHY.md` — CLI design rules: icons, fix-op consolidation, command-count ceiling

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

