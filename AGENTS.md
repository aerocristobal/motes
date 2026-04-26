# AGENTS.md — Working with Motes from any AI Coding Agent

This file is the canonical contract for AI coding agents (Claude Code, Cursor, Copilot, Aider, OpenAI Codex, Gemini Code Assist, etc.) working in the motes repository or using motes as a memory system inside other projects.

It complements `CLAUDE.md` (Claude Code specific) and `GEMINI.md` (Gemini Code Assist specific). When those files exist, they take precedence for their respective agents; this file is the fallback.

---

## TL;DR for Agents

1. **Read [README.md](README.md) and [docs/internals.md](docs/internals.md) first.** Don't guess at architecture.
2. **All planning, memory, and task tracking goes through `mote`.** Do not use ad-hoc TODO files, in-tree task lists, or external trackers.
3. **Before any non-trivial code change, create a task mote.** Close it on completion.
4. **Tests are mandatory.** `go vet ./... && go test ./...` must pass before you commit.
5. **`mote dream` works with any of three LLM backends** — `claude-cli`, `openai`, `gemini` (Vertex AI). See [docs/providers.md](docs/providers.md).
6. **Push your work.** A session is not done until `git push` succeeds.

---

## What is motes?

Motes is an AI-native context and memory system written in Go. Knowledge is stored as atomic units ("motes") — markdown files with YAML frontmatter — linked in two dimensions:

- **Dependency links** (`depends_on`, `blocks`) for planning and execution ordering
- **Semantic links** (`relates_to`, `builds_on`, `contradicts`, `supersedes`, `caused_by`, `informed_by`) for thematic memory

There is no database. There is no network for core operations. The CLI is `mote`, a single Go binary.

For a 60-second tour, read the [README.md "Concepts" section](README.md#concepts).

---

## Project Conventions

### Language & Style

- **Go 1.25+.** Stdlib + `github.com/spf13/cobra` + `gopkg.in/yaml.v3`. **Do not add dependencies** without strong justification — the project's distribution model is "single static binary."
- **AGPL-3.0-or-later** SPDX header at the top of every `.go` file.
- **Tabs for indentation** (Go default). The project's tests assert this; sed-driven multi-line edits frequently get this wrong — verify with `gofmt`.
- **Atomic file writes.** All state changes go through `core.AtomicWrite` (write-to-temp-then-rename). Never `os.WriteFile` directly for anything in `.memory/`.
- **Reads never write.** Access counts are batched in `.access_batch.jsonl` and flushed at session-end. Don't sneak writes into read paths.

### Build & Test

```bash
make build       # go build -o mote ./cmd/mote
make test        # go test ./...
make vet         # go vet ./...
make install     # build + copy to ~/.local/bin/
```

CI gating runs the same three commands. Don't merge or push without all three clean.

### File Layout

```
cmd/mote/                # CLI entry point + cobra commands (cmd_*.go)
internal/core/           # Mote types, config, index, atomic writes
internal/dream/          # Dream cycle: scanner, batcher, invokers, parser
internal/scoring/        # Hot-path scoring engine
internal/strata/         # BM25 reference knowledge layer
internal/security/       # Body content secret detection
docs/                    # PRD, architecture, configuration reference
```

When you're not sure where something belongs, **ask in a task mote** instead of guessing — the boundary between `core` and feature packages is meaningful.

---

## Workflow Contract

### 1. Task Tracking is Mandatory

Before changing code, create a task mote:

```bash
mote add --type=task --title="Short summary" --tag=topic --body "What and why"
```

When done:

```bash
mote update <id> --status=completed
```

For multi-step work, use `mote plan <parent-id> --child "..." --child "..."` to create a hierarchy. Use `mote progress <parent-id>` to track completion.

**Do NOT** use markdown checklists in PRs, TODO comments in code, or external issue trackers as a substitute. The graph is the system of record.

### 2. Knowledge Capture

When you discover something non-obvious (a bug's root cause, an architectural constraint, a tricky API contract):

```bash
mote add --type=lesson --title="..." --tag=... --body "..."
mote add --type=decision --title="..." --tag=... --body "..."
mote add --type=explore --title="..." --tag=... --body "..."
```

Link findings into the graph with `mote link`:

```bash
mote link <new-mote-id> caused_by <bug-task-id>
mote link <decision-id> relates_to <existing-decision-id>
```

Wikilinks (`[[mote-id]]`) in body text are auto-resolved during search and traversal.

### 3. Error Recovery

When you hit an unfamiliar error, search prior lessons before debugging from scratch:

```bash
mote search "<error message or key phrase>" --type=lesson
```

Skip this for trivial errors (typos, syntax, missing imports). Don't re-query the same error class within a session.

### 4. Session Completion ("Landing the Plane")

Per [CLAUDE.md](CLAUDE.md), a session is **not** complete until:

1. All quality gates pass (`go vet ./...`, `go test ./...`)
2. Task motes are closed
3. Commits exist and `git push` succeeds

Never declare done without pushing. If push fails, fix and retry — do not punt to the user.

---

## Multi-Provider Dream Cycle

Motes runs its dream cycle (the only LLM-using subsystem) against any of three backends, configured per stage in `.memory/config.yaml`:

| Backend | Auth | Notes |
|---------|------|-------|
| `claude-cli` | `oauth` (literal placeholder) | Default. Shells out to the `claude` binary. |
| `openai` | env var name (e.g. `OPENAI_API_KEY`) or literal key | Calls `api.openai.com/v1/chat/completions`. |
| `gemini` | `vertex-ai` (sentinel) | Vertex AI ADC via `gcloud auth print-access-token`. Requires `gcp_project` in `options`. |

Different agents will naturally prefer different providers — see [GEMINI.md](GEMINI.md) for the Gemini-specific recipe and [docs/providers.md](docs/providers.md) for the full reference.

The dream cycle pipeline (`internal/dream/`) does not change based on backend. Adding a new provider means implementing the `Invoker` interface in a new file and adding a case to the `NewInvoker` factory in `internal/dream/invoker.go` — see how `OpenAIInvoker` and `GeminiInvoker` are structured.

---

## Common Pitfalls

### "Where do I put this?"

| You want to... | Look in / use... |
|---|---|
| Add a CLI command | `cmd/mote/cmd_<name>.go` + `init()` registers with `rootCmd` |
| Add a mote field | `internal/core/mote.go` (`Mote` struct) — also update index, search, frontmatter parser |
| Tune scoring | `internal/scoring/` + `ScoringConfig` in `internal/core/config.go` |
| Tune dream cycle | `internal/dream/` + `DreamConfig` in `internal/core/config.go` |
| Add a config field | `internal/core/config.go` struct + default in `DefaultConfig()` + comment in `internal/core/config_yaml.go` if user-facing + entry in `docs/configuration.md` |
| Add a doctor check | `runDoctorChecks` (integrity errors) or `runDoctorAdvisories` (warnings) in `cmd/mote/cmd_doctor.go` |

### "The test passes locally but I'm not sure it's testing the right thing"

- Tests use real filesystem state via `t.TempDir()`. Don't mock the filesystem.
- HTTP-based invokers (OpenAI, Gemini) use `httptest.NewServer` with a tighter retry policy injected for speed — see `internal/dream/openai_invoker_test.go` for the pattern.
- Tests that need `gcloud` skip with a clear message rather than failing on dev machines without it.

### "I'm tempted to add a YAML library / a new dependency / a config-file format"

Don't. The project is deliberately minimal. Open a `decision` mote explaining why the existing tools fall short, link it from your task mote, and discuss before importing.

### "Comments in config.yaml keep disappearing"

`SaveConfig` now writes through the `yaml.v3` Node API (`internal/core/config_yaml.go`). If you add a new user-facing field that should have a comment in the generated `config.yaml`, add a `HeadComment` decoration in `buildConfigNode`.

---

## Agent-Specific Files

- **[CLAUDE.md](CLAUDE.md)** — Claude Code specifics, including hook automation and the Landing the Plane checklist.
- **[GEMINI.md](GEMINI.md)** — Google Gemini Code Assist specifics, including how to configure `mote dream` to use Gemini itself for the dream cycle.
- **AGENTS.md** (this file) — applies to any agent without a more specific file.

When working in a project that *uses* motes as a memory system (rather than the motes source repo itself), this file is the right level of generality. Each consuming project should write its own short `CLAUDE.md` / `AGENTS.md` describing its own conventions, then defer to motes' canonical workflow.

---

## Agent-Native Principle

**Any action a human can take with motes, an agent can also take.** The CLI surface is intentionally complete:

- Every command supports `--json` for machine-readable output (where output structure matters)
- All state lives in plain text under `.memory/` so agents can `cat`/`grep` directly when needed
- No interactive-only operations — every flow has a non-interactive path

If you find yourself unable to do something programmatically that's possible interactively, **that's a bug**. File a task mote.

---

## License

This file, like the rest of the project, is AGPL-3.0-or-later.
