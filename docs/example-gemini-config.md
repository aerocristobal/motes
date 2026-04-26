# Example: Gemini CLI configuration

This shows a complete `~/.gemini/settings.json` with all motes hooks installed and the `context.fileName` array configured to load both `GEMINI.md` and `AGENTS.md`. All commands use bare `mote` (no absolute paths) — ensure `mote` is on your `$PATH`.

Hooks and `context.fileName` are installed automatically by `mote onboard` when `~/.gemini/` is detected (or pass `--gemini` explicitly). You can also add them manually using the format below.

> **Important:** Gemini CLI hook timeouts are in **milliseconds** (default 60000 = 60s), unlike Codex (seconds, default 600). The `mote session-end --hook` flush regularly takes 2–3 minutes on projects with many motes; without an explicit `timeout`, the SessionEnd hook will be killed mid-flight.

---

## `~/.gemini/settings.json`

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear",
        "hooks": [
          {
            "name": "mote-prime",
            "type": "command",
            "command": "mote prime --hook --mode=startup",
            "timeout": 60000
          }
        ]
      }
    ],
    "BeforeAgent": [
      {
        "hooks": [
          {
            "name": "mote-prompt-context",
            "type": "command",
            "command": "mote prompt-context",
            "timeout": 30000
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "name": "mote-session-end",
            "type": "command",
            "command": "mote session-end --hook",
            "timeout": 300000
          }
        ]
      }
    ]
  },
  "context": {
    "fileName": ["GEMINI.md", "AGENTS.md"]
  }
}
```

Notes:

- `timeout` is in milliseconds. Common motes values: `60000` (1 min) for `SessionStart`, `30000` (30s) for `BeforeAgent` per-prompt context, **`300000` (5 min) for `SessionEnd`** — match this last one carefully.
- `matcher` for `BeforeAgent` and `SessionEnd` is omitted (Gemini CLI ignores `matcher` on these events).
- Gemini's `SessionStart` matchers are `startup`, `resume`, `clear` (no separate `compact` event — Gemini has `PreCompress` for that, which motes does not currently wire).
- `context.fileName` array determines which workspace files Gemini CLI loads as context. With `["GEMINI.md", "AGENTS.md"]`, both are read on every session start. Order doesn't matter; later files don't override earlier.

---

## Project-local `.gemini/settings.json`

The motes repo ships [`.gemini/settings.json`](../.gemini/settings.json) at the project root. The same hooks fire when Gemini CLI is opened in any cloned copy — once you grant project trust on first open. This is convenient for contributors who want the dream-cycle workflow live in their checkout.

The repo file is identical to the user-tier example above, except:

- It's loaded only when the project `.gemini/` layer is **trusted**. Gemini CLI prompts on first open.
- Project-level settings merge with user-tier `~/.gemini/settings.json`; project takes precedence on key conflicts.
- Gemini CLI fingerprints project hooks. If a hook's `name` or `command` changes (for example, via `git pull`), Gemini treats it as a new untrusted hook and asks again before running it.

---

## Hook descriptions

| Hook | Purpose |
|------|---------|
| **SessionStart** | Primes context at session start with scored mote selection. Combined `startup\|resume\|clear` matcher covers all three Gemini session sources. |
| **BeforeAgent** | Injects per-prompt context (active task progress, relevant motes). Gemini's analog of Claude's `UserPromptSubmit` — fires after the user submits a prompt but before the agent loop runs. |
| **SessionEnd** | Flushes the access batch, creates a session summary mote, runs co-access linking, and updates the global quality ledger. Fires when the session ends (`exit`, `clear`) — semantically better aligned with mote's heavy session-end work than Claude's per-turn `Stop`. |

---

## Hook output contract

All three hooks emit JSON-only output on stdout (Gemini CLI's "Golden Rule"). For SessionStart context injection, motes returns:

```json
{ "hookSpecificOutput": { "hookEventName": "SessionStart", "additionalContext": "..." } }
```

Gemini CLI accepts the same shape Claude Code uses, so `mote prime --hook` works in both ecosystems unchanged. Debug output goes to stderr (Gemini CLI captures stderr but never tries to parse it as JSON).

---

## Verification

After installing, confirm hooks are loaded:

```bash
gemini --version    # confirm CLI is on PATH

# In an interactive Gemini CLI session:
/hooks panel        # should show mote-prime, mote-prompt-context, mote-session-end
/memory show        # should display GEMINI.md (with @AGENTS.md inlined) + AGENTS.md
```

If hooks don't appear, the most common cause is the `~/.gemini/` directory not existing when you ran `mote onboard` (no auto-detect trigger). Re-run with the explicit flag:

```bash
mote onboard --gemini
```

---

## Related documents

- [`GEMINI.md`](../GEMINI.md) — Gemini CLI working agreements (loaded into context via `@AGENTS.md` import)
- [`docs/example-settings-json.md`](example-settings-json.md) — equivalent file for Claude Code
- [`docs/example-codex-config.md`](example-codex-config.md) — equivalent file for OpenAI Codex
- [`docs/providers.md`](providers.md#gemini-vertex-ai-adc) — configuring Gemini as the **dream-cycle backend** (separate from Gemini CLI driving as the agent)
- [Gemini CLI hooks docs](https://geminicli.com/docs/hooks/) · [skills docs](https://geminicli.com/docs/cli/skills/) · [GEMINI.md docs](https://geminicli.com/docs/cli/gemini-md/)
