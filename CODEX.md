# CODEX.md — Working with Motes from OpenAI Codex

This file is the entry point for **OpenAI Codex** (CLI, IDE extension, Codex app) when operating in this repository or in any project that uses motes as its memory system.

For the project's working agreements (which Codex auto-loads as part of its system prompt), see [AGENTS.md](AGENTS.md). For agent-agnostic background, see [docs/agents-guide.md](docs/agents-guide.md). For Claude Code specifics, see [CLAUDE.md](CLAUDE.md). For Gemini Code Assist specifics, see [GEMINI.md](GEMINI.md).

---

## TL;DR

1. **Codex auto-loads `AGENTS.md`** at every session start. That file is the working contract — keep it short.
2. **Hooks live in `.codex/hooks.json`** (or `[hooks]` in `.codex/config.toml`). The repo ships a working `.codex/hooks.json` and `mote onboard --codex` writes the global one at `~/.codex/hooks.json`.
3. **Skills live at `.agents/skills/<name>/SKILL.md`**. `mote onboard --codex` installs the motes skills there alongside the existing `~/.claude/skills/` install.
4. **OpenAI as the dream-cycle backend** is configured separately — see [`docs/providers.md`](docs/providers.md#openai). It's independent of Codex driving you as the agent.
5. **Tests must pass:** `go vet ./... && go test ./...`. Push your work; a session is not done until `git push` succeeds.

---

## Project Overview

Motes is an AI-native context and memory system written in Go. Knowledge is stored as atomic units (motes) — markdown files with YAML frontmatter under `.memory/nodes/`. The CLI is `mote`. There is no database. The only LLM-using subsystem is the **dream cycle** (`internal/dream/`), a headless background pipeline that performs semantic analysis, link inference, and constellation evolution.

The dream cycle dispatches through an `Invoker` interface (`internal/dream/invoker.go`) that supports three backends: `claude-cli`, `openai`, `gemini` (Vertex AI). Read [docs/internals.md](docs/internals.md) for the architectural overview.

---

## Codex Hooks

Codex hooks let `mote prime` and `mote session-end` fire automatically at session start and end — exactly the same lifecycle integration that Claude Code gets via `~/.claude/settings.json`. The two systems use the same wire format (`additionalContext` JSON on stdout), so motes' existing `--hook` flags work in both ecosystems with no extra adapters.

### Enable the feature flag

Codex hooks are behind a feature flag in `~/.codex/config.toml`:

```toml
[features]
codex_hooks = true
```

Without this flag, Codex ignores `hooks.json` files entirely.

### Hook locations Codex reads

Codex looks for hooks in these layers (all matching layers run together):

| Layer | Path | Trust |
|-------|------|-------|
| Project (this repo) | `.codex/hooks.json` | Loads only when project trust is granted |
| Global user | `~/.codex/hooks.json` | Always loads |
| Inline TOML (project) | `.codex/config.toml` `[hooks]` block | Loads only when project trust is granted |
| Inline TOML (global) | `~/.codex/config.toml` `[hooks]` block | Always loads |
| Managed | per `requirements.toml` `managed_dir` | Enterprise / MDM |

Prefer one representation per layer (JSON or inline TOML, not both) — Codex warns at startup if both are present.

### What the repo ships

The motes repo includes [`.codex/hooks.json`](.codex/hooks.json) at the root. When you open the repo with Codex and grant project trust, these hooks fire automatically:

| Event | Matcher | Command | What it does |
|-------|---------|---------|--------------|
| `SessionStart` | `startup\|resume\|clear` | `mote prime --hook --mode=startup` | Loads scored mote context into the prompt |
| `UserPromptSubmit` | (any) | `mote prompt-context` | Injects per-prompt context (active task, relevant motes) |
| `Stop` | (any) | `mote session-end --hook` | Flushes access counts, creates session summary, runs co-access linking |

The `Stop` hook can take 1–3 minutes on projects with many motes (it does a full graph rebuild). Codex's default per-hook timeout is 600s — well above the observed worst case.

### Set up global hooks

Run `mote onboard --codex` from any motes-enabled project. It writes `~/.codex/hooks.json` with the same three hooks above and (if `~/.codex/config.toml` is fresh) sets the `codex_hooks = true` flag. If the file already exists, it merges new entries idempotently.

Auto-detect: if `~/.codex/` exists when you run plain `mote onboard`, the Codex install path runs automatically — you don't need the flag.

For the full reference and TOML equivalent, see [docs/example-codex-config.md](docs/example-codex-config.md).

---

## Codex Skills

Codex looks for skills in these locations (in order):

| Scope | Location | Use |
|-------|----------|-----|
| REPO | `<repo>/.agents/skills/` | Repo-specific skills |
| REPO | walked up the tree from CWD | Module-specific skills |
| USER | `~/.agents/skills/` | Personal skills available across repos |
| ADMIN | `/etc/codex/skills/` | Machine-wide skills |
| SYSTEM | bundled with Codex | OpenAI's defaults (skill-creator, plan, etc.) |

Skills follow the [open agent skills standard](https://github.com/openai/skills) — same `SKILL.md` frontmatter that Claude Code reads. Codex's only difference is the directory it scans.

### What the motes skills do

`mote onboard --codex` installs four motes skills at `~/.agents/skills/`:

| Skill | When Codex auto-invokes | What it does |
|-------|------------------------|--------------|
| `mote-capture` | When you mention "capture this" / "save a lesson" / "remember" | Guides knowledge capture with tag suggestions and link discovery |
| `mote-retrieve` | When you ask "what do we know about X" / "search for Y" | Guides retrieval choice (graph traversal vs full-text vs strata) |
| `mote-plan` | When breaking down multi-step work | Creates parent + child task motes with dependencies |
| `mote-subagent` | When spawning sub-agents | Concise context-retrieval instructions for sub-agents |

Codex's initial skills list is capped at ~2% of context window or 8 KB. Motes skills total roughly 4 KB of metadata, so they comfortably fit even alongside other installed skills.

### Explicit invocation

In addition to implicit triggering, you can invoke any skill directly:

```
$mote-plan
```

Or in CLI/IDE, use `/skills` to see the full list.

### Disable a skill without deleting it

Edit `~/.codex/config.toml`:

```toml
[[skills.config]]
path = "/home/you/.agents/skills/mote-subagent/SKILL.md"
enabled = false
```

Restart Codex after changing.

---

## Using OpenAI as the Dream-Cycle Backend

This is **separate** from Codex driving you as the agent. The dream cycle (`mote dream`) is a background pipeline that runs LLM analysis over your knowledge graph. You can configure it to use OpenAI's Chat Completions API (or Anthropic's Claude, or Google Gemini) regardless of which agent you're using to *write code*.

The full setup lives in [`docs/providers.md` § OpenAI Backend](docs/providers.md#openai). Short version:

```yaml
# .memory/config.yaml
dream:
  provider:
    batch:
      backend: openai
      auth: OPENAI_API_KEY
      model: gpt-4o-mini
    reconciliation:
      backend: openai
      auth: OPENAI_API_KEY
      model: gpt-4o
```

```bash
export OPENAI_API_KEY=sk-...
mote dream
```

Cost is reported in the `mote dream` summary line based on the model and `internal/dream/pricing.go`.

---

## Workflow for Codex Agents

Same as `AGENTS.md` workflow contract — Codex auto-loads it. The short version:

1. **Before coding**: `mote add --type=task --title="..." --tag=...`
2. **While coding**: capture lessons/decisions/explore notes as you find them
3. **On errors**: `mote search "<phrase>" --type=lesson` first
4. **After coding**: `go vet ./... && go test ./...`, close task motes, `git push`

A session is not done until `git push` succeeds.

---

## Codex-Specific Notes

### AGENTS.md is loaded into context

Codex reads `AGENTS.md` at every session start and includes it in the system prompt. The 32 KiB cap (`project_doc_max_bytes`) is generous, but each line still costs context — keep `AGENTS.md` lean and put long-form material in `docs/agents-guide.md`.

If you want temporary instructions without editing the file everyone uses, drop `AGENTS.override.md` next to it. Codex prefers the override.

### `apply_patch` matcher aliases

For `PreToolUse` / `PostToolUse` hooks targeting file edits, Codex accepts `Edit`, `Write`, or `apply_patch` as matchers. They're aliases. Use whichever reads more clearly.

### Hook stdin contract

Every hook command receives a JSON object on stdin. Common fields: `session_id`, `transcript_path`, `cwd`, `hook_event_name`, `model`. `SessionStart` adds `source` (one of `startup`, `resume`, `clear`). The motes hooks use only `cwd` (to find `.memory/`) — they ignore the rest.

### Hook stdout contract

For `SessionStart`, `UserPromptSubmit`, and `Stop`, plain text on stdout becomes additional developer context. JSON output supports `additionalContext`:

```json
{ "hookSpecificOutput": { "hookEventName": "SessionStart", "additionalContext": "..." } }
```

`mote prime --hook` and `mote session-end --hook` both emit this shape (the same shape Claude Code uses), so they work in both ecosystems unchanged.

### Repo-local hooks require trust

The `.codex/hooks.json` checked into this repo only loads when you grant project trust on first open. This is a Codex security boundary, not a motes thing.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Hooks don't fire | `codex_hooks` feature flag not set | Add `[features] codex_hooks = true` to `~/.codex/config.toml`, restart |
| Repo hooks ignored | Project trust not granted | Codex prompts on first open; re-grant via Codex settings |
| Skill not appearing in `/skills` list | Wrong path | Skills must be at `.agents/skills/<name>/SKILL.md` (note: `.agents`, not `.codex`) |
| Skill cut off in initial list | 2% / 8 KB context budget exceeded | Codex shortens descriptions first; if you have many skills, some are omitted |
| `AGENTS.md` truncated | Combined size exceeds `project_doc_max_bytes` (32 KiB default) | Raise the limit or split nested `AGENTS.md` files across directories |
| `Stop` hook seems hung | `mote session-end` is doing full graph rebuild on a large project | Wait — observed worst case is 2-3 minutes; well under the 600s default timeout |
| `additionalContext` not showing up | Wrong JSON shape | Must be `{"hookSpecificOutput": {"hookEventName": "...", "additionalContext": "..."}}` — see Codex hooks docs |

---

## Pointers

- **Codex docs:** [hooks](https://developers.openai.com/codex/hooks), [skills](https://developers.openai.com/codex/skills), [AGENTS.md](https://developers.openai.com/codex/guides/agents-md)
- **Repo files:** [`.codex/hooks.json`](.codex/hooks.json), [`AGENTS.md`](AGENTS.md), [`docs/example-codex-config.md`](docs/example-codex-config.md)
- **Source of truth (skills install):** `ensureMoteSkills` in `cmd/mote/cmd_onboard.go`
- **Source of truth (Codex hooks install):** `ensureCodexHooks` in `cmd/mote/cmd_onboard.go`

---

## License

This file, like the rest of the project, is AGPL-3.0-or-later.
