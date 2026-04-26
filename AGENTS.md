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
5. A session is **not** done until `git push` succeeds.
   Never declare done without pushing.

Do not use ad-hoc TODO files, in-tree task lists, markdown checklists, or
external trackers as a substitute for task motes. The graph is the system
of record.

## Project conventions

- Go 1.25+. Stdlib + `github.com/spf13/cobra` + `gopkg.in/yaml.v3` only.
  Adding a dependency requires a `decision` mote first.
- AGPL-3.0-or-later SPDX header at the top of every `.go` file.
- Tabs for indentation. Verify with `gofmt`.
- All `.memory/` writes go through `core.AtomicWrite` (write-temp, rename).
  Reads never write — access counts are batched and flushed at session-end.
- Storage: markdown + YAML frontmatter under `.memory/nodes/`. No database.

## Layout — where things go

| Want to... | Look in / use... |
|---|---|
| Add a CLI command | `cmd/mote/cmd_<name>.go` + `init()` registers with `rootCmd` |
| Add a mote field | `internal/core/mote.go` (`Mote` struct) — also update index, search, frontmatter parser |
| Tune scoring | `internal/scoring/` + `ScoringConfig` in `internal/core/config.go` |
| Tune dream cycle | `internal/dream/` + `DreamConfig` in `internal/core/config.go` |
| Add a config field | `internal/core/config.go` struct + `DefaultConfig()` + `internal/core/config_yaml.go` for user-facing comments + `docs/configuration.md` row |
| Add a doctor check | `runDoctorChecks` (errors) or `runDoctorAdvisories` (warnings) in `cmd/mote/cmd_doctor.go` |
| Add an LLM provider | New `Invoker` impl in `internal/dream/<name>_invoker.go` + factory case in `invoker.go` + pricing row in `pricing.go` |

## LLM backend

The dream cycle is the only LLM-using subsystem. It supports three backends
configurable per stage in `.memory/config.yaml`:

| Backend | Auth | Notes |
|---------|------|-------|
| `claude-cli` | `oauth` placeholder | Default. Shells out to the `claude` binary. |
| `openai` | env var name (e.g. `OPENAI_API_KEY`) or literal | Calls `api.openai.com/v1/chat/completions`. |
| `gemini` | `vertex-ai` sentinel | Vertex AI ADC via `gcloud auth print-access-token`. Requires `gcp_project` in `options`. |

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
