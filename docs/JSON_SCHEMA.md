# Mote JSON Schema

> **Status:** v1 (introduced in `mote 0.4.35`, STORY-JSCHEMA-001 / `docs/beads-recommendations.md` §23.7).
> **Stability:** every shape in §4 is a stable contract for callers of `mote <cmd> --json`. Renames, removals, and type changes bump `schema_version`. Additive changes (new optional field, new enum value, new top-level key) do **not** bump.

---

## 1. Envelope contract

Every JSON-emitting command branches on the `MOTE_JSON_ENVELOPE` env var:

| Env value | Mode | stdout (success) | stderr (failure) |
|-----------|------|------------------|------------------|
| `1`       | envelope | `{"schema_version": 1, "data": <payload>}` | `{"schema_version": 1, "error": "<message>", "code": "<STABLE_CODE>"}` |
| unset / `""` / `0` | legacy | raw payload as before, byte-for-byte | plain-text message (existing behaviour) |
| anything else | legacy + warning | raw payload as before | a one-line stderr warning naming `MOTE_JSON_ENVELOPE`, followed by the legacy behaviour |

In **envelope mode** the integer `schema_version` is always emitted first, before the (potentially large) `data` payload. Streaming parsers can branch on the version before consuming the body.

In **legacy mode** the JSON branch additionally writes a one-line deprecation notice to stderr exactly once per process. The notice names the env var and the rollout schedule. It does not pollute stdout, so it does not interfere with `mote ls --json | jq ...` pipelines.

---

## 2. Schema versioning rule

`schema_version` is **an umbrella integer**. It governs every shape in §4 collectively.

* **Bumps** on rename, removal, or type change of any documented field on any shape.
* **Does NOT bump** on:
  * adding a new optional field to a documented shape,
  * adding a new value to an enum (`status`, `type`, `execution_mode`, etc.),
  * adding a new top-level optional key.

When a shape changes incompatibly, document a new versioned name (e.g. `ls.list.v2`) alongside the old one and bump `schema_version` to `2`. Old versioned names remain documented for one release after removal, so consumers see what disappeared.

The per-shape version names (`ls.list.v1`, `show.object.v1`, …) live here for documentation only. The wire format carries only the umbrella integer — this matches beads' progression and keeps `jq` queries short.

---

## 3. Deprecation timeline

| Release | `MOTE_JSON_ENVELOPE` default | Legacy availability | Notes |
|---------|------------------------------|---------------------|-------|
| `v0.5.x` | legacy (notice on stderr each run) | always | opt in with `MOTE_JSON_ENVELOPE=1`. Notice fires exactly once per process. |
| `v0.6.x` | envelope | opt out with `MOTE_JSON_ENVELOPE=0` | legacy still available but emits a stronger notice. |
| `v0.7.x` | envelope (no legacy) | none | the `MOTE_JSON_ENVELOPE` variable becomes a no-op and the legacy shape is removed. |

Consumers running `mote ls --json | jq '.motes'` keep working through `v0.5.x`. At `v0.6.x` they need `jq '.data.motes'` or to opt back into legacy with `MOTE_JSON_ENVELOPE=0`. At `v0.7.x` only `.data.motes` works.

---

## 4. Shapes

Each subsection lists required vs optional fields, the source struct in the Go code, and a `jq` example for envelope mode.

### 4.1 `ls.list.v1` — `mote ls --json`

Source: `cmd/mote/cmd_ls.go::LsOutput`.

| Field          | Type     | Required | Notes |
|----------------|----------|----------|-------|
| `motes`        | `[]LsMoteEntry` | yes | Always an array — `[]` if no motes match, never `null`. |
| `motes[].id`   | string   | yes | Stable mote ID. |
| `motes[].type` | string   | yes | One of the enum values in `core.Mote.Type` (`task`, `decision`, `lesson`, `explore`, …). |
| `motes[].status` | string | yes | `active`, `in_progress`, `completed`, `deprecated`, `archived`. |
| `motes[].weight` | number | yes | Composite score (real). |
| `motes[].title` | string  | yes | Free text. |

Envelope-mode example:
```bash
MOTE_JSON_ENVELOPE=1 mote ls --json | jq '.data.motes[] | {id, status}'
```

### 4.2 `pulse.list.v1` — `mote pulse --json`

Identical to `ls.list.v1`. `mote pulse` is a convenience alias for `mote ls --status=active --type=task` sorted by weight, and routes through the same `doLs` JSON branch. Documented separately so adding a `pulse`-specific field later does not silently leak through the `ls` contract.

### 4.3 `stats.object.v1` — `mote stats --json`

Source: `cmd/mote/cmd_stats.go::StatsOutput`. A single flat object with required core counts and a long tail of optional metrics.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `total_motes` | int | yes | |
| `status_counts` | `map[string]int` | yes | Keys mirror the status enum. |
| `accessed_7d`, `accessed_30d`, `accessed_90d`, `never_accessed` | int | yes | |
| `total_tags`, `overloaded_tags`, `singleton_tags` | int | yes | |
| `contradictions`, `pending_visions` | int | yes | |
| `dream_runs`, `dream_input_tokens`, `dream_output_tokens`, `dream_estimated_cost`, `dream_total_visions`, `dream_total_applied`, `dream_total_deferred`, `dream_acceptance_rate`, `dream_cost_per_accepted` | int / number | no | Present when dream telemetry exists. |
| `prime_hit_rate`, `prime_sessions` | number / int | no | |
| `created_7d` … `net_growth_90d` | int | no | Flow stats when motes carry `status_changed_at`. |
| `graph_decisions`, `graph_lessons`, `graph_explorations`, `graph_knowledge_count`, `graph_avg_links`, `graph_cross_session_motes`, `graph_age_days` | int / number | no | |

### 4.4 `show.object.v1` — `mote show <id> --json`

Source: `cmd/mote/cmd_show.go::ShowOutput`. The full record. Required fields are `id`, `type`, `status`, `title`, `tags`, `weight`, `origin`, `created_at`, `access_count`, `body`. Every other field is optional via `omitempty`. Execution metadata (`execution_agent_type`, `execution_suggested_model`, `execution_reasoning_effort`, `execution_mode`, `execution_parallel_group`) is positioned **before** `body` so orchestrators dispatching subagents read dispatch hints first.

### 4.5 `show.short.v1` — `mote show <id> --short --json`

Source: `cmd/mote/cmd_show.go::ShowShortOutput`. Tight loop-iteration shape — exactly five fields, all required: `id`, `status`, `type`, `weight`, `title`. Consumers can depend on this set not growing.

### 4.6 `show.long.v1` — `mote show <id> --long --json`

Source: `cmd/mote/cmd_show.go::ShowLongOutput`. Superset of `show.object.v1`: every default-mode field is promoted to the top level, plus forensic extension fields (`last_prime_at`, `audit_log_path`, `audit_log_entries_count`, `promoted_to`, `strata_corpus`, `strata_query_hint`, `strata_query_count`, `strata_last_queried`, `deprecated_by`, `status_changed_at`, `deprecation_chain`). Forensic fields are `omitempty`.

### 4.7 `show.execution-only.v1` — `mote show <id> --execution-only`

Source: `cmd/mote/cmd_show.go::ShowExecutionOnlyOutput`. Strictly six fields and no body:

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `id`                          | string | yes | |
| `execution_agent_type`        | string | no | omitempty |
| `execution_suggested_model`   | string | no | omitempty |
| `execution_reasoning_effort`  | string | no | omitempty |
| `execution_mode`              | string | no | omitempty |
| `execution_parallel_group`    | string | no | omitempty |

`--execution-only` emits JSON **without** `--json`. The two flags are mutually exclusive. In envelope mode this shape is also wrapped under `data`.

A mote with no execution metadata serializes as `{"id":"motes-xyz"}` (the empty-state contract — missing metadata is not an error, per sprint-2 §23.16).

### 4.8 `context.list.v1` — `mote context <topic...> --json`

Source: `cmd/mote/cmd_context.go::ContextOutput`.

| Field         | Type     | Required | Notes |
|---------------|----------|----------|-------|
| `topic`       | string   | yes | The space-joined topic string actually queried. |
| `results`     | `[]MoteEntry` | yes | Scored results in descending order. Empty list is `[]`, not `null`. |
| `results[].id`, `results[].title`, `results[].type`, `results[].status`, `results[].score` | various | yes | See `cmd/mote/cmd_search.go` (the `MoteEntry` type is shared). |

### 4.9 `error.v1` — every error from a JSON-emitting command in envelope mode

Source: `internal/jsonenv.errorEnvelope`. Always written to **stderr**, never stdout. Exactly three keys, all required:

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `schema_version` | int | yes | Mirrors the success envelope. |
| `error` | string | yes | Human-readable message. Stable wording is not guaranteed; treat as display text. |
| `code` | string | yes | Stable identifier in `UPPER_SNAKE_CASE`. Match on this, not on `error`. |

Codes registered for v1:

| `code` | Emitted from | Meaning |
|--------|--------------|---------|
| `MOTE_NOT_FOUND` | `mote show <id> --json` | The requested mote ID does not exist (and the failure is a real `os.IsNotExist`, not an unrelated read error). |
| `MOTE_INVALID_FLAG` | `mote show` | Mutually-exclusive flag combinations: `--short` + `--long`, or `--execution-only` + `--json`. |

Adding a new error site:
1. Pick a code in `UPPER_SNAKE_CASE`. Prefer specificity over reuse — a parser can branch on a code it doesn't recognise more safely than on overloaded ones.
2. Add it to this table with the originating command and meaning.
3. In the command's `RunE`, use `jsonEnvErr(jsonFlag, "YOUR_CODE", exit, "msg ...", args)`.

`jsonenv.WrapError` **panics** if invoked with an empty `code`. This is deliberate — silent malformed envelopes would be worse than a crash in development.

---

## 5. `--format json` is documented as non-stable

`mote` does not currently expose a `--format json` flag. If one is added later, it MUST be documented as a non-stable, human-readable variant that does not carry the envelope. The only stable, schema-versioned JSON interface is `--json`. This rule exists to prevent ambiguity if `--format` is introduced for human output later (e.g. `--format pretty` / `--format table`).

---

## 6. Out of scope

* **`mote compliance` JSON output.** Governed by the OSCAL spec from NIST; its `schema_version` is the OSCAL version (`1.2.1`, etc.), not this envelope. Compliance JSON is unaffected by `MOTE_JSON_ENVELOPE`.
* **Sibling `--json` paths.** `mote search`, `mote tags`, `mote update --claim`, `mote prime`, `mote remember`, `mote memories`, `mote dream --review`, `mote hook` keep their legacy shapes through `v0.5.x`. They are not in `internal/jsonenv.RegisteredShapes()` and are out of scope for STORY-JSCHEMA-001. The same envelope helpers will be applied to them in a follow-up story.
* **MCP wrapper.** The envelope is a prerequisite for it, but the wrapper itself is a separate epic.

---

## 7. Empty-state guarantee

`data.motes`, `data.results`, and any other documented list field are emitted as `[]` (not `null`), preserving the sprint-2 §23.16 contract for agent polling loops. The envelope branch does NOT collapse empty arrays into the absence of a field.

```bash
$ MOTE_JSON_ENVELOPE=1 mote ls --status=does-not-exist --json
{
  "schema_version": 1,
  "data": {
    "motes": []
  }
}
```

---

## 8. Drift detection

`mote doctor` cross-checks the list returned by `internal/jsonenv.RegisteredShapes()` against this file. A registered shape that is not mentioned here becomes an `undocumented_json_shape` integrity issue and pushes doctor's exit code to `1`. A missing `docs/JSON_SCHEMA.md` becomes a `json_schema_doc_missing` issue. Run `mote doctor` after adding a new `--json` emitter.

---

## 9. Filtering does not change shape

Filter flags introduced by STORY-MQRY-001 (`--metadata-field`, `--has-metadata-key` on `ls` and `search`) restrict which motes appear in `data.motes` / `data.results`. They do not introduce new fields and they do not bump `schema_version`. A filtered list is the same shape as an unfiltered list with fewer entries.
