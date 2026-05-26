---
user-invocable: false
description: Concise mote instructions for subagents — retrieve context, capture findings, attribute writes.
---

# Mote Subagent Instructions

You are running as a subagent. Use motes for context retrieval and knowledge capture.

## Retrieve Context

```bash
mote context <topic>      # Graph traversal — what do we know about X?
mote search <query>       # Full-text search (add --type, --tag, --status to filter)
mote show <id>            # Read a specific mote
mote strata query <topic> # Query reference docs
```

## Capture Findings

Before returning results, capture non-obvious findings:

```bash
mote add --type=decision --title="Summary" --tag=topic --body "Rationale"
mote add --type=lesson --title="Summary" --tag=topic --body "Details"
mote add --type=explore --title="Summary" --tag=topic --body "Findings"
```

After capturing, link related motes: `mote link <id1> relates_to <id2>`

## Agent Identity

Set `MOTE_AGENT_ID` to attribute your writes:

```bash
export MOTE_AGENT_ID=subagent-<purpose>
```

This tags mote frontmatter with `agent:` for traceability in multi-agent sessions.

## Rules

- Do NOT create task motes — only the parent agent manages tasks
- Do NOT run `mote prime` or `mote session-end` — the parent handles lifecycle
- Capture before returning — findings not captured are lost

## Execution metadata (read at dispatch)

Your model and reasoning effort were chosen from execution metadata at dispatch time and cannot be changed. The parent orchestrator read your dispatch mote's `execution_*` fields via `mote show <id> --execution-only` before launching you; that decision is final for this run.

If you believe the metadata is wrong, capture a `decision` mote explaining why and let the parent orchestrator re-dispatch — do not attempt to re-spawn yourself with different settings.

Full contract: `docs/agents-guide.md#read-execution-metadata-before-prose`.
