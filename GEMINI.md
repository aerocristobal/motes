# GEMINI.md — Working with motes from Gemini CLI

Loaded into context at every Gemini CLI session start. The full working
agreements live in [AGENTS.md](AGENTS.md), imported below; this file
adds Gemini-CLI-specific tooling notes only.

@AGENTS.md

---

## Gemini CLI specifics

### Hooks

- Configured in `~/.gemini/settings.json` (user) or `.gemini/settings.json` (project).
- The repo ships a working `.gemini/settings.json` at the root with three hooks:
  - `SessionStart` (matchers `startup|resume|clear`) → `mote prime --hook --mode=startup`
  - `BeforeAgent` → `mote prompt-context` (Gemini's analog of Claude's `UserPromptSubmit`)
  - `SessionEnd` → `mote session-end --hook` with explicit **300000ms timeout**
- `mote onboard --gemini` installs the same three hooks at `~/.gemini/settings.json`. Auto-detected when `~/.gemini/` exists.
- **Timeouts are in milliseconds.** Gemini's default is 60000ms (60s); the `mote session-end --hook` flush regularly takes 2–3 minutes on large projects, so we set 300000ms explicitly. Without it the hook gets killed mid-flight.
- See [`docs/example-gemini-config.md`](docs/example-gemini-config.md) for the full reference.
- Manage with `/hooks panel`, `/hooks enable-all`, `/hooks disable <name>`.

### Skills

- Skills live at `.agents/skills/<name>/SKILL.md`. Gemini CLI scans this path at higher precedence than `.gemini/skills/`. **The same path Codex uses** — one install, both agents discover them.
- `mote onboard --gemini` (or `--codex`, or auto-detect of either tool) writes the four motes skills:
  - `mote-capture` — knowledge capture with tag suggestions
  - `mote-retrieve` — graph traversal vs full-text vs strata routing
  - `mote-plan` — multi-step parent + child task decomposition
  - `mote-subagent` — concise context-retrieval instructions for sub-agents
- Manage with `gemini skills list/install/disable` or the `/skills` slash command.
- Activation requires explicit user consent on the first call (Gemini shows a confirmation prompt with the skill's name, purpose, and folder path).

### `/memory` workflow

- `/memory show` — display the loaded GEMINI.md + AGENTS.md (after `@AGENTS.md` import expansion)
- `/memory reload` — re-scan after editing either file
- `/memory add <text>` — appends to your global `~/.gemini/GEMINI.md`

### `context.fileName` configuration

The repo's `.gemini/settings.json` configures `context.fileName: ["GEMINI.md", "AGENTS.md"]` so both files load. `mote onboard --gemini` writes the same setting at the user tier, preserving any user-defined entries.

Without this setting, Gemini CLI loads only `GEMINI.md` — but the `@AGENTS.md` import line at the top of this file inlines AGENTS.md anyway, so coverage is the same. The setting just ensures AGENTS.md is also picked up directly when nested AGENTS.md overrides exist in subdirectories (Codex-spec discovery).

### Hook environment

Mote's `--hook` flags read `cwd` from stdin and ignore the rest, so they work unchanged. For reference, Gemini CLI sets:

- `GEMINI_PROJECT_DIR`, `GEMINI_CWD`, `GEMINI_SESSION_ID`
- `CLAUDE_PROJECT_DIR` (alias, for cross-tool compat)

---

## Gemini as the dream-cycle backend

Separate concern from Gemini CLI driving you as the agent. The dream cycle (`mote dream`) can use Gemini's Vertex AI API regardless of which agent is writing code. Full setup at [`docs/providers.md` § gemini (Vertex AI ADC)](docs/providers.md#gemini-vertex-ai-adc).

Short version:

```yaml
# .memory/config.yaml
dream:
  provider:
    batch:
      backend: gemini
      auth: vertex-ai
      model: gemini-2.5-flash
      options:
        gcp_project: your-gcp-project
```

```bash
gcloud auth application-default login
mote dream
```

---

## Long-context tuning

Gemini's large context window (1M+ tokens for 2.5 Pro) is well-suited to motes' dream cycle. If you use Gemini as both the agent and the dream-cycle backend on a project with many small motes, you can comfortably increase the batch size:

```yaml
# .memory/config.yaml
dream:
  batching:
    max_motes_per_batch: 100   # default 50
    max_batches: 24            # default 12
```

The `internal/dream/gemini_invoker.go` invoker sets `responseMimeType: application/json` in `generationConfig`, so Gemini emits JSON natively and motes' parser sees no "no JSON found" warnings. If you do see them in `dream/failed_responses.jsonl`, the safety filter is the most likely cause — check `response_preview`.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Hooks don't fire | `mote` not on `$PATH` | `which mote` should succeed; install via `make install` |
| Hook killed before completing | Default 60s timeout exceeded | Set explicit `timeout` in ms — we use 300000ms for SessionEnd |
| Skill not in `/skills list` | Wrong path | Skills must be at `.agents/skills/<name>/SKILL.md` (or `.gemini/skills/`); `.agents` takes precedence |
| AGENTS.md not loaded | `context.fileName` doesn't include it | Add to `~/.gemini/settings.json` `context.fileName` array, or rely on the `@AGENTS.md` import in this file |
| `/memory show` shows stale content | File was edited mid-session | `/memory reload` |

---

## Pointers

- **Gemini CLI docs:** [hooks](https://geminicli.com/docs/hooks/), [skills](https://geminicli.com/docs/cli/skills/), [GEMINI.md](https://geminicli.com/docs/cli/gemini-md/)
- **Repo files:** [`.gemini/settings.json`](.gemini/settings.json), [`AGENTS.md`](AGENTS.md), [`docs/example-gemini-config.md`](docs/example-gemini-config.md)
- **Source of truth (Gemini settings install):** `ensureGeminiSettings` in `cmd/mote/cmd_onboard.go`
- **Dream-cycle backend (Vertex AI):** [`docs/providers.md`](docs/providers.md#gemini-vertex-ai-adc)
- **Other agent guides:** [`CLAUDE.md`](CLAUDE.md) (Claude Code), [`CODEX.md`](CODEX.md) (OpenAI Codex)

---

## License

This file, like the rest of the project, is AGPL-3.0-or-later.
