# Example: OpenAI Codex hooks configuration

This shows complete `~/.codex/hooks.json` and equivalent `~/.codex/config.toml` files with all mote hooks installed. All commands use bare `mote` (no absolute paths) — ensure `mote` is on your `$PATH`.

Hooks are installed automatically by `mote onboard` when `~/.codex/` is detected (or pass `--codex` explicitly). You can also add them manually using either format below.

> **Required:** Codex hooks are behind a feature flag. Add this to `~/.codex/config.toml` (creating the file if it doesn't exist) before any hook will fire:
>
> ```toml
> [features]
> codex_hooks = true
> ```

---

## `~/.codex/hooks.json` (recommended)

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear",
        "hooks": [
          { "type": "command", "command": "mote prime --hook --mode=startup" }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          { "type": "command", "command": "mote prompt-context" }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "mote session-end --hook", "timeout": 600 }
        ]
      }
    ]
  }
}
```

Notes:

- `timeout` is in seconds. Codex defaults to `600` if omitted; we set it explicitly so future readers know the value matters.
- `matcher` for `UserPromptSubmit` and `Stop` is ignored by Codex — both events run on every occurrence regardless.
- Codex's `SessionStart` matcher accepts `startup`, `resume`, or `clear` (no separate `compact` event — Codex doesn't fire one).

---

## Inline `[hooks]` in `~/.codex/config.toml` (alternative)

If you prefer one file:

```toml
[features]
codex_hooks = true

[[hooks.SessionStart]]
matcher = "startup|resume|clear"

[[hooks.SessionStart.hooks]]
type = "command"
command = "mote prime --hook --mode=startup"

[[hooks.UserPromptSubmit]]

[[hooks.UserPromptSubmit.hooks]]
type = "command"
command = "mote prompt-context"

[[hooks.Stop]]

[[hooks.Stop.hooks]]
type = "command"
command = "mote session-end --hook"
timeout = 600
```

> Don't mix forms in a single layer. If both `hooks.json` and inline `[hooks]` exist in the same `~/.codex/`, Codex merges them and prints a warning at startup. Pick one.

---

## Repo-local `.codex/hooks.json`

The motes repo ships a working `.codex/hooks.json` at the project root. The same hooks fire when Codex is opened in any cloned copy — once you grant project trust on first open. This is convenient for contributors who want the dream-cycle workflow live in their checkout.

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear",
        "hooks": [
          { "type": "command", "command": "mote prime --hook --mode=startup" }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          { "type": "command", "command": "mote prompt-context" }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "mote session-end --hook", "timeout": 600 }
        ]
      }
    ]
  }
}
```

Project-local hooks load only when the project `.codex/` layer is **trusted**. In untrusted projects, Codex still loads user and system hooks from their own active config layers — so global hooks installed at `~/.codex/hooks.json` are unaffected.

---

## Hook descriptions

| Hook | Purpose |
|------|---------|
| **SessionStart** | Primes context at session start with scored mote selection. Same `additionalContext` JSON shape Claude Code uses. The combined `startup\|resume\|clear` matcher covers all three Codex session sources (Codex doesn't have a separate `compact` event). |
| **UserPromptSubmit** | Injects per-prompt context (active task progress, relevant motes). Runs on every user message. |
| **Stop** | Flushes the access batch, creates a session summary mote, runs co-access linking, and updates the global quality ledger. Runs on every conversation turn that ends. May take 1–3 minutes on large projects (full graph rebuild). |

---

## Verification

After installing, confirm hooks are loaded:

```bash
codex --ask-for-approval never "List the active hooks." 2>&1 | grep -i hook
```

If `mote prime --hook` doesn't appear, the most likely cause is a missing `codex_hooks = true` feature flag — see the box at the top.

---

## Related documents

- [`CODEX.md`](../CODEX.md) — Codex-specific tooling overview (hooks, skills, AGENTS.md)
- [`docs/example-settings-json.md`](example-settings-json.md) — equivalent file for Claude Code
- [`docs/providers.md`](providers.md) — configuring OpenAI as the dream-cycle backend (separate from Codex driving as the agent)
- [Codex hooks docs (OpenAI)](https://developers.openai.com/codex/hooks)
