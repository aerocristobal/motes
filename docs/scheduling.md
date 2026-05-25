# Time-based scheduling

Motes carries two orthogonal temporal fields per task: **`due_at`** (when work
should be done by) and **`defer_until`** (when work should resurface in the
ready queue). They are independent — a mote may have both, neither, or just
one.

## Flags

| Surface | Flag | Effect |
|---|---|---|
| `mote add` | `--due=<spec>` | Set `due_at` at create. Past values allowed (back-dating). |
| `mote add` | `--defer=<spec>` | Set `defer_until` at create. Must be strictly in the future. |
| `mote update` | `--due=<spec>` | Set `due_at`. Empty (`--due=""`) clears the field. |
| `mote update` | `--defer=<spec>` | Set `defer_until` (must be future). Empty clears the field. |
| `mote ls` | `--overdue` | Show active/in_progress motes with `due_at < now`, sorted ascending. |
| `mote ls` | `--ready` | Already-existing flag; deferred motes are now hidden unless `--include-deferred`. |
| `mote ls` | `--include-deferred` | When combined with `--ready`, surface deferred motes anyway. |
| `mote ls` | `--due-before=<spec>` | Keep only motes with `due_at` strictly before this time. |
| `mote ls` | `--due-after=<spec>` | Keep only motes with `due_at` strictly after this time. |

`mote update --due` and `mote update --defer` are mutually exclusive with
`--claim`, matching the existing pattern for field-mutation flags.

## Time spec grammar

The same parser accepts every `<spec>` argument. It is an allowlist — anything
that isn't in this list is rejected with `invalid time`.

| Form | Example | Meaning |
|---|---|---|
| Relative future | `+30m`, `+6h`, `+2d`, `+1w` | now + duration. `m`=minutes, `h`=hours, `d`=days, `w`=weeks. |
| Natural | `now`, `tomorrow`, `next monday`, `next sunday` | Resolved against the host's local timezone. `tomorrow` and `next <weekday>` are 00:00 local. |
| Absolute date | `2026-12-01` | 00:00 in the host's local timezone. |
| Absolute RFC3339 | `2026-12-01T10:00:00Z` | Exact UTC instant. |

### Bounds and rejections

- Resolved times more than **10 years** in the future are rejected.
- Negative relative durations (`-1h`, `-1d`, …) are rejected.
- Shell metacharacters (`$`, `;`, `` ` ``, `|`, `&`, …) are rejected.
- Path-traversal sequences (`..`) are rejected.
- Unicode bidi controls are rejected.

The parser is fuzz-tested locally:

```bash
go test ./internal/core -run=^$ -fuzz=FuzzParseTimeSpec -fuzztime=30s
```

## Ready-queue semantics

`mote ls --ready` previously surfaced any `task` with status `active` and zero
unfinished blockers. With scheduling, deferred motes are filtered out:

```
ready = type==task ∧ status==active ∧ no live blockers ∧ (defer_until == nil ∨ defer_until ≤ now)
```

`--include-deferred` lifts the last clause so deferred motes surface anyway —
useful when an agent wants to inspect what is pending but hidden.

When a `defer_until` value is in the past, the mote is treated as not-deferred
for the purpose of the ready filter, but the field is **not auto-cleared**:
the value remains as a record of "I deferred this until X".

## Overdue surfacing

`mote ls --overdue` returns motes where:

```
due_at != nil ∧ due_at < now ∧ (status == active ∨ status == in_progress)
```

Results are sorted ascending by `due_at` so the most-overdue motes appear
first. Completed, archived, and deprecated motes are excluded.

## Audit log

Every change to `due_at` or `defer_until` appends an audit entry to
`.memory/audit.jsonl` with the changed field name in `fields_set`:

```json
{"operation":"update","mote_id":"motes-abc...","agent_id":"agent-A","timestamp":"...","fields_set":["due_at"]}
```

The audit schema records field names, not before/after values. The schema
itself is the canonical record of who set what when.

## Examples

```bash
# Back-date a missed deadline and surface it immediately
mote add --type=task --title="follow up on incident" --due=2026-01-01T00:00:00Z
mote ls --overdue --json   # includes the new mote

# Snooze a task until next Monday at 00:00 local
mote add --type=task --title="contact vendor" --defer="next monday"
mote ls --ready            # task does not appear
mote ls --ready --include-deferred   # task appears

# Clear a defer that's no longer relevant
mote update motes-abc... --defer=""

# Find everything past-due
mote ls --overdue

# Find everything due before tomorrow
mote ls --due-before=+1d --json
```
