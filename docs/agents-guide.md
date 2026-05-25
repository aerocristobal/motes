# Agents Guide — Extended Notes for AI Coding Agents

The short version of working agreements lives in `AGENTS.md` (top-level) — that file is loaded into the model context at every session, so it's deliberately tight. This guide is the longer-form companion: background, common pitfalls, the "agent-native" principle, and pointers to agent-specific files.

Read `AGENTS.md` first. Read this when you need more context than the prompt-budget version provides.

---

## What is motes?

Motes is an AI-native context and memory system written in Go. Knowledge is stored as atomic units ("motes") — markdown files with YAML frontmatter under `.memory/nodes/` — linked in two dimensions:

- **Dependency links** (`depends_on`, `blocks`) for planning and execution ordering.
- **Semantic links** (`relates_to`, `builds_on`, `contradicts`, `supersedes`, `caused_by`, `informed_by`) for thematic memory.
- **Provenance links** (`discovered_from`) — when working on mote A you discover a follow-up bug or task B, run `mote link B discovered_from A`. The link is directional and *non-blocking*: B stays ready even while A is still in flight, and A's frontmatter is untouched (the reverse `discovered_ref` lives in the index, surfaced by `mote show A` and `mote context A`).

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

## Empty-state contract (`--ready` / `--claim`)

Autonomous agents poll the workspace in a loop, waiting for claimable work. The CLI guarantees a stable contract so a polling shell loop can tell a quiet workspace apart from a real failure without parsing stderr:

| Command | Outcome | Exit code | Stdout |
|---|---|---|---|
| `mote ls --ready --json` | Nothing claimable (empty, all in flight, all blocked) | **0** | exactly `{"motes":[]}` |
| `mote ls --ready --json` | One or more ready motes | **0** | `{"motes":[ … ]}` (object wraps array) |
| `mote update <id> --claim` | Success | **0** | success line / JSON envelope |
| `mote update <id> --claim` | Lost the race (mote already claimed) | **2** | `{"claimed":false, …}` if `--json` |
| `mote update <id> --claim` | Other failure (bad ID, missing `MOTE_AGENT_ID`, blockers unfinished, terminal status) | **1** | stderr error |
| Any command | Unknown flag, malformed args | **non-zero** | (no JSON envelope) |

The exit-code split between **1** (real error) and **2** (contention) lets a shell script retry on contention while bailing on real errors:

```bash
while ! id=$(mote ls --ready --json | jq -er '.motes[0].id'); do sleep 30; done
if ! mote update "$id" --claim; then
  case $? in
    2) continue ;;   # someone else got it first — just keep polling
    *) exit 1 ;;     # real error — bail
  esac
fi
```

Two intentional details to be aware of:

- **JSON shape is `{"motes":[…]}`, not a bare `[…]`.** This is different from `bd ready --json` (beads), which emits a top-level array. Both shapes round-trip cleanly through `jq`. Do not change this without coordinating with consumers; a `--bare-array` mode could be added in a follow-up if needed.
- **`ls --ready` is design-robust against index drift.** It does not consult `.memory/index.jsonl`; it scans `nodes/` directly. A corrupt or out-of-date index does not break the polling loop. Malformed node files print a `warning: skipping <file>` line to stderr and are excluded from the result — they do not cause a non-zero exit. Use `mote doctor` to surface workspace inconsistencies; use `mote dream` (or `mote prime` on session start) to rebuild the index.

The Go test suite codifies this contract — see `cmd/mote/cmd_ls_empty_state_test.go`, `cmd/mote/cmd_ls_polling_test.go`, `cmd/mote/cmd_update_claim_test.go`, and `cmd/mote/cmd_update_claim_contract_test.go`. The Gherkin specification lives at `features/cli/empty-state-contract.feature` (living documentation).

---

## Show density (`mote show --short` / default / `--long`)

`mote show` has three text modes and three JSON shapes, each tuned for a different consumer.

| Mode | Use when | Stdout | Side effect |
|---|---|---|---|
| **default** | Inspecting one mote in detail | ~15-section rich text | Appends an access-batch entry (counts as a read) |
| `--short` | Iterating across many motes in a loop | One line: `<icon> <id> <weight> [<type>] <title>` | **None.** Loop-pure — does NOT increment `access_count`, so scanning 30 ready motes won't skew weight decay |
| `--long` | Debugging weight decay, prime injection, or deprecation chains | Default output + an `--- internal state ---` section (last_prime_at, audit_log entry count for this mote, promoted_to, strata_corpus, deprecated_by, status_changed_at, deprecation_chain) | Appends an access-batch entry |
| `--short --json` | Loop with JSON parsing | Exactly five keys: `id, status, type, weight, title` | None |
| `--long --json` | Forensic JSON; strict superset of `--json` | Every default-`--json` key plus `last_prime_at, audit_log_path, audit_log_entries_count, promoted_to, strata_corpus, deprecated_by, status_changed_at, deprecation_chain` | Appends an access-batch entry |
| `--short --long` | (rejected) | empty | Exits 1; stderr `--short and --long are mutually exclusive`. No mote read, no access-batch append |

**Status icons.** `--short` prefixes each line with a one-character lifecycle glyph: `○` active, `◐` in_progress, `✓` completed, `●` archived, `❄` deprecated. Set `NO_UNICODE=1` or pass `--ascii` to swap to the ASCII fallback (`o p x . -`).

**Default-mode byte-stability.** The default output is covered by a golden-file snapshot test (`cmd/mote/testdata/show_default.golden`) so renderer changes that drift the output produce a CI failure rather than a silent regression. Regenerate with `UPDATE_GOLDEN=1 go test ./cmd/mote/ -run TestShow_DefaultOutput_ByteStableAgainstSnapshot`.

**Loop pattern (the design driver):**

```bash
# 30 ready motes in 30 lines (was: 30 ready motes in ~600 lines of default output)
for id in $(mote ls --ready --json | jq -r '.motes[].id'); do
  mote show "$id" --short
done
```

The Go test suite codifies these modes — see `cmd/mote/cmd_show_short_test.go`, `cmd/mote/cmd_show_long_test.go`, `cmd/mote/cmd_show_flags_test.go`, `cmd/mote/cmd_show_snapshot_test.go`, and `internal/format/icon_test.go`. The Gherkin specification lives at `features/agent_context/show_density.feature`.

---

## Memory verbs (`remember` / `memories` / `recall` / `forget`)

For short durable rules — "always run tests with -race", "auth uses JWT not sessions" — use the memory verbs instead of `mote add --type=lesson`:

| Verb | Purpose |
|---|---|
| `mote remember "<text>" [--key K] [--force] [--no-clobber] [--json]` | Save a memory. Key auto-derived from text via slugify if `--key` omitted. |
| `mote memories [substring] [--json]` | List all memories, optionally filtered (case-insensitive, matches key OR body). Sorted ascending by key. |
| `mote recall <key>` | Print one memory's body. Exits 2 if not found. |
| `mote forget <key>` | Delete a memory. Exits 2 if not found. Writes a `memory.delete` audit entry. |

Memories surface automatically in every `mote prime` output under a `## Persistent memories` heading positioned **before** every other section, so they survive bottom-up truncation by agent hosts. Use `mote prime --memories-only` in compact hook contexts to skip every other section.

### Memories vs lessons (when to use which)

**Memories** are flat key/body pairs stored at `.memory/memory.json`. They are **outside** the mote graph: no scoring, no edges, no contradiction detection, no concept index, no global promotion. They exist to seed `mote prime` output with terse rules.

**Lessons** (`mote add --type=lesson`) are full motes in the graph: they have weight, decay, can be linked with `[[wikilinks]]`, participate in dream cycles, get suggested via `mote search`, and can be promoted across projects. Use lessons for narrative knowledge with context and relationships.

A useful split: if the next session needs to **see it on every prime**, it's a memory. If the next session needs to **find it by topic**, it's a lesson.

### Auto-slugification rules

When no `--key` is given, `mote remember` derives one from the body:

- ASCII letters (lowercased) and digits are preserved.
- Whitespace and punctuation collapse to a single hyphen.
- Non-ASCII characters are dropped (no Unicode normalization).
- Runs of hyphens collapse to one; leading/trailing hyphens are trimmed.
- Truncated to 50 characters.
- On collision, a `-2`, `-3`, … suffix is appended.

`"always run tests with -race flag"` → `always-run-tests-with-race-flag`.

### Defaults and flags

- **Duplicate `--key` overwrites by default** (verb-as-intent: `remember` means "make this true now"). Pass `--no-clobber` to fail instead.
- **Empty or whitespace-only bodies are rejected** (`mote remember ""` exits non-zero).
- **Body length is capped at 1000 bytes** — longer durable notes belong in `mote add --type=lesson`.
- **Security scan parity with `mote add`**: bodies are scanned for credentials and private keys before persistence. Hits block the write; `--force` bypasses and is recorded in the audit log as `security_override`.
- **Exit codes**: `0` success, `1` real error, `2` "memory not found" (mirrors `mote update --claim`'s contention exit). The split lets a polling shell distinguish a missing key from an I/O failure.

### Export and import round-trip

`mote export` now emits a **JSON envelope**:

```json
{
  "motes":    [ { …mote… }, … ],
  "memories": [ { "key": "…", "body": "…", "created_at": "…", "updated_at": "…" }, … ]
}
```

This is a breaking change from the prior JSONL stream. `mote import` accepts both shapes — envelope inputs round-trip memories too; legacy JSONL inputs import motes only.

---

## Agent-Specific Files

| File | Audience | Read when |
|------|----------|-----------|
| `AGENTS.md` | All agents (Codex spec; Gemini CLI via `@AGENTS.md` import) | Always — it's the prompt |
| `CLAUDE.md` | Claude Code | Working from Claude Code (it auto-loads this) |
| `CODEX.md` | OpenAI Codex | Hooks/skills setup; Codex-specific tooling |
| `GEMINI.md` | Gemini CLI | Hooks/skills setup; `/memory` workflow; `context.fileName` config |
| `docs/providers.md` | Any agent | Configuring the dream cycle's LLM backend (claude-cli, openai, gemini Vertex AI) |

**Gemini CLI vs Gemini Code Assist:** these are two different products. Gemini CLI is a standalone command-line agent ([geminicli.com](https://geminicli.com/)); Gemini Code Assist is the IDE plugin. `GEMINI.md` in this repo targets Gemini CLI. The Vertex AI dream-cycle backend in `docs/providers.md` works with both — it's about using Google's API, not about which agent is driving you as a developer.

When working in a project that *uses* motes as its memory system (rather than the motes source repo itself), the right approach is:

1. Each consuming project writes its own short `AGENTS.md` / `CLAUDE.md` describing its conventions
2. Those files reference the canonical motes workflow (e.g. "we use motes for task tracking — see `~/.claude/CLAUDE.md` for the full reference")
3. They do not duplicate motes workflow instructions

This keeps consuming-project instruction files small and the canonical motes workflow in one place.

---

## License

This file, like the rest of the project, is MIT-licensed.
