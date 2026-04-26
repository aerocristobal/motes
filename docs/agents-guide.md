# Agents Guide — Extended Notes for AI Coding Agents

The short version of working agreements lives in `AGENTS.md` (top-level) — that file is loaded into the model context at every session, so it's deliberately tight. This guide is the longer-form companion: background, common pitfalls, the "agent-native" principle, and pointers to agent-specific files.

Read `AGENTS.md` first. Read this when you need more context than the prompt-budget version provides.

---

## What is motes?

Motes is an AI-native context and memory system written in Go. Knowledge is stored as atomic units ("motes") — markdown files with YAML frontmatter under `.memory/nodes/` — linked in two dimensions:

- **Dependency links** (`depends_on`, `blocks`) for planning and execution ordering.
- **Semantic links** (`relates_to`, `builds_on`, `contradicts`, `supersedes`, `caused_by`, `informed_by`) for thematic memory.

There is no database. There is no network for core operations. The CLI is `mote`, a single Go binary.

For a 60-second tour, read the [README.md "Concepts" section](../README.md#concepts).

---

## Common Pitfalls

### "Where do I put this?"

The layout table in `AGENTS.md` covers most cases. Beyond that:

| Symptom | Probable cause |
|---|---|
| Adding to `internal/core/` makes you nervous | `core` is foundational. New feature code usually belongs in a feature package (`scoring`, `strata`, `dream`) and pulls from `core`, not the other way around. |
| You're tempted to share state across packages via globals | Don't. Pass it as a struct field or function arg. |
| You want to add a new top-level command that "feels different" | Look at how the existing `cmd_*.go` files share helpers (`mustFindRoot`, `formatCost`). Match their shape. |

When in doubt, **open a `decision` mote** explaining the choice. It costs nothing now and is the system's only durable record of "why is this here?".

### "The test passes locally but I'm not sure it's testing the right thing"

- Tests use real filesystem state via `t.TempDir()`. Don't mock the filesystem.
- HTTP-based invokers (OpenAI, Gemini) use `httptest.NewServer` with a tighter retry policy injected for speed — see `internal/dream/openai_invoker_test.go` for the pattern.
- Tests that need external CLIs (`gcloud`, `claude`) skip with a clear message rather than failing on dev machines without them. See `internal/dream/gemini_invoker_test.go`.
- The `_test.go` files are the closest thing to API documentation. When in doubt, run them and read the assertions.

### "I'm tempted to add a YAML library / a new dependency / a config-file format"

Don't. The project's distribution model is "single static binary, zero config." The current dependency set (`cobra`, `yaml.v3`) was hard-fought; any addition needs justification in a `decision` mote linked from your task mote.

### "Comments in `config.yaml` keep disappearing"

`SaveConfig` writes through the `yaml.v3` Node API (`internal/core/config_yaml.go`). The struct→YAML round-trip strips comments; the Node tree preserves them. If you add a new user-facing field that should have a comment in the generated `config.yaml`, add a `HeadComment` decoration in `buildConfigNode`.

### "The stop hook seems slow / hung"

`mote session-end --hook` does substantial work on every session end (full `ReadAllParallel`, BM25 across all motes, optional concept enrichment, optional strata re-ingest, full edge-index rebuild). For a project with thousands of motes (including the global layer at `~/.claude/memory/`), it can take 2-3 minutes. Not stalled — actively working. Possible improvements (skip BM25 when no session motes were touched, separate fast/slow hooks, incremental rebuilds) are real but not currently scheduled.

### "I just made a multi-file edit and one file's tabs are gone"

`sed -i` and other line-based tools sometimes mangle Go's tab indentation. Run `gofmt -w <file>` after any sed-driven multi-line edit. The CI vet check will catch it but it's faster to fix locally.

### "I can't find the `MEMORY.md` index file"

Each agent installation has its own memory index:
- Project memory (this project's auto-memory): `~/.claude/projects/<encoded-cwd>/memory/MEMORY.md`
- Cross-project memory (motes' global layer): `~/.claude/memory/`
- Project mote storage: `<project>/.memory/`

`mote prime` surfaces the relevant subset; you rarely need to `cat` these directly.

---

## Agent-Native Principle

**Any action a human can take with motes, an agent can also take.** The CLI surface is intentionally complete:

- Every command supports `--json` for machine-readable output (where output structure matters)
- All state lives in plain text under `.memory/` so agents can `cat`/`grep` directly when needed
- No interactive-only operations — every flow has a non-interactive path (`--from=fresh`, `--dry-run`, `--quiet`, etc.)
- Hooks (`mote prime --hook`, `mote prompt-context`, `mote session-end --hook`) emit JSON shaped for in-context injection

If you find yourself unable to do something programmatically that's possible interactively, **that's a bug**. File a task mote.

---

## Agent-Specific Files

| File | Audience | Read when |
|------|----------|-----------|
| `AGENTS.md` | All agents (Codex spec) | Always — it's the prompt |
| `CLAUDE.md` | Claude Code | Working from Claude Code (it auto-loads this) |
| `CODEX.md` | OpenAI Codex | Hooks/skills setup; Codex-specific tooling |
| `GEMINI.md` | Google Gemini Code Assist | Vertex AI / Gemini setup; using Gemini as the dream-cycle backend |
| `docs/providers.md` | Any agent | Configuring the dream cycle's LLM backend |

When working in a project that *uses* motes as its memory system (rather than the motes source repo itself), the right approach is:

1. Each consuming project writes its own short `AGENTS.md` / `CLAUDE.md` describing its conventions
2. Those files reference the canonical motes workflow (e.g. "we use motes for task tracking — see `~/.claude/CLAUDE.md` for the full reference")
3. They do not duplicate motes workflow instructions

This keeps consuming-project instruction files small and the canonical motes workflow in one place.

---

## License

This file, like the rest of the project, is AGPL-3.0-or-later.
