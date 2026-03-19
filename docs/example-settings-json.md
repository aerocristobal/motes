# Example: Claude Code settings.json

This shows a complete `~/.claude/settings.json` with all mote hooks installed. All commands use bare `mote` (no absolute paths) — ensure `mote` is on your `$PATH`.

Hooks are installed automatically by `mote onboard`. You can also add them manually.

---

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [{ "type": "command", "command": "mote prime --hook --mode=startup" }]
      },
      {
        "matcher": "resume",
        "hooks": [{ "type": "command", "command": "mote prime --hook --mode=resume" }]
      },
      {
        "matcher": "compact",
        "hooks": [{ "type": "command", "command": "mote prime --hook --mode=compact" }]
      },
      {
        "matcher": "clear",
        "hooks": [{ "type": "command", "command": "mote prime --hook --mode=startup" }]
      }
    ],
    "PreCompact": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "mote prime --hook --mode=compact" }]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "mote prompt-context" }]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "mote session-end --hook" }]
      }
    ]
  }
}
```

## Hook Descriptions

| Hook | Purpose |
|------|---------|
| **SessionStart** | Primes context at session start. Differentiated matchers provide startup (full), resume (abbreviated), compact (full + snippets), and clear (full) modes. |
| **PreCompact** | Re-primes context before context window compaction so important motes survive. |
| **UserPromptSubmit** | Injects per-prompt context (active task progress, relevant motes). |
| **Stop** | Flushes access batch and creates session summary on session end. Fires automatically — no need for Claude to remember to call `mote session-end`. |
