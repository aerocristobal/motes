# GEMINI.md — Working with Motes from Gemini Code Assist

This file is the entry point for **Google Gemini Code Assist** (and other Gemini-family agents — Gemini CLI, Vertex AI Agent Builder, etc.) when operating in this repository or in any project that uses motes as its memory system.

For agent-agnostic conventions, see [AGENTS.md](AGENTS.md). For Claude Code specifics, see [CLAUDE.md](CLAUDE.md).

---

## TL;DR

1. **Read [README.md](README.md), then [AGENTS.md](AGENTS.md).** Those define the workflow contract and project conventions.
2. **Use `mote` for all task and knowledge tracking.** No ad-hoc TODOs, no in-tree task lists.
3. **You can drive the dream cycle with Gemini itself** — see "Configuring Motes to Use Gemini" below.
4. **Tests must pass:** `go vet ./... && go test ./...`.
5. **Push your work** before declaring a session done.

---

## Project Overview (Motes-Specific)

Motes is an AI-native context and memory system written in Go. Knowledge is stored as atomic units (motes) — markdown files with YAML frontmatter under `.memory/nodes/`. The CLI is `mote`. There is no database. The only LLM-using subsystem is the **dream cycle** (`internal/dream/`), a headless background pipeline that performs semantic analysis, link inference, and constellation evolution.

The dream cycle dispatches through an `Invoker` interface (`internal/dream/invoker.go`) that supports three backends:

- `claude-cli` — shells out to Anthropic's `claude` CLI
- `openai` — HTTP client against OpenAI Chat Completions
- `gemini` — HTTP client against Vertex AI's `generateContent` API

Read [docs/internals.md](docs/internals.md) for the architectural overview, then [docs/providers.md](docs/providers.md) for backend setup.

---

## Configuring Motes to Use Gemini

The `gemini` backend uses **Vertex AI** with **Application Default Credentials** (ADC). API-key auth (`generativelanguage.googleapis.com`) is intentionally not supported in v0.4.11 — open an issue if you need it.

### Prerequisites

1. A Google Cloud project with the Vertex AI API enabled.
2. The `gcloud` CLI on `PATH`.
3. ADC configured:
   ```bash
   gcloud auth application-default login
   gcloud auth print-access-token | head -c 20    # confirm a token is retrievable
   ```

### `.memory/config.yaml`

```yaml
dream:
  provider:
    batch:
      backend: gemini
      auth: vertex-ai
      model: gemini-2.5-flash
      options:
        gcp_project: your-gcp-project
        gcp_region: us-central1                # default if omitted
        safety_threshold: BLOCK_ONLY_HIGH      # default if omitted
    reconciliation:
      backend: gemini
      auth: vertex-ai
      model: gemini-2.5-pro
      options:
        gcp_project: your-gcp-project
        gcp_region: us-central1
    rate_limit_rpm: 60                         # 0 = unlimited
```

Run it:

```bash
mote dream                  # full cycle
mote dream --dry-run        # preview without LLM calls
mote dream --review         # interactive vision review
```

### Tier-to-Model Mapping

The orchestrator now constructs a separate invoker per stage, so the legacy "tier" parameter is purely informational. The mapping you care about:

| Stage | Reads from | Recommended Gemini model |
|-------|-----------|--------------------------|
| Batch reasoning | `dream.provider.batch.model` | `gemini-2.5-flash` (cheap, fast, runs many times in parallel) |
| Reconciliation | `dream.provider.reconciliation.model` | `gemini-2.5-pro` (single high-capability pass) |

### Safety Settings

All four Vertex AI harm categories (HARASSMENT, HATE_SPEECH, SEXUALLY_EXPLICIT, DANGEROUS_CONTENT) are set to `safety_threshold` from your config. Default is `BLOCK_ONLY_HIGH` — permissive enough that technical content in mote bodies doesn't trigger false positives. Valid values:

- `BLOCK_NONE`
- `BLOCK_ONLY_HIGH` (default)
- `BLOCK_MEDIUM_AND_ABOVE`
- `BLOCK_LOW_AND_ABOVE`

A `finishReason: SAFETY | RECITATION | PROHIBITED_CONTENT` response is treated as a **non-retryable** error — `RetryPolicy` will not waste attempts on a deterministic block. The error string contains `"gemini response blocked"` so it shows up clearly in logs.

### Cost Estimation

`mote dream` prints an estimated cost line that uses the configured model and the per-million-token rates in `internal/dream/pricing.go`. As of 2026-04-25:

| Model | Input $/MTok | Output $/MTok |
|-------|--------------|----------------|
| `gemini-2.5-flash` | 0.30 | 2.50 |
| `gemini-2.5-pro` | 1.25 | 10.0 |

These are sourced from `cloud.google.com/vertex-ai/pricing`. If they drift, update the table and the date in `pricing.go`.

### Verifying Setup

```bash
mote doctor                 # advisories: missing gcp_project, wrong auth value, etc.
```

Expected silence on a correctly configured machine; clear actionable messages otherwise.

---

## Workflow for Gemini Agents

### 1. Before Coding — Create a Task Mote

```bash
mote add --type=task --title="Short summary" --tag=topic --body "What and why"
```

For multi-step work, use `mote plan <parent-id> --child "..." --child "..."` to create a hierarchy.

### 2. While Coding — Capture Knowledge

When you discover something non-obvious:

```bash
mote add --type=lesson --title="..." --body "..." --tag=...
mote link <new-id> caused_by <task-id>
```

### 3. On Errors — Search First

```bash
mote search "<error fragment>" --type=lesson
```

### 4. After Coding — Quality Gates + Push

```bash
go vet ./...
go test ./...
mote update <task-id> --status=completed
git add <files>
git commit -m "..."
git push
```

A session is **not done** until `git push` succeeds.

---

## Gemini-Specific Notes

### Long-Context Strengths

Gemini's large context window (1M+ tokens) is well-suited to motes' dream cycle, which constructs prompts containing:

- The lucid log (cross-batch context, capped at `dream.journal.max_tokens` — default 2000)
- A batch of motes (up to 50 by default — see `dream.batching.max_motes_per_batch`)
- The pre-scan candidates (link suggestions, contradictions, stale flags)

You can comfortably increase batch size when running on Gemini if your project has many small motes:

```yaml
dream:
  batching:
    max_motes_per_batch: 100   # default 50
    max_batches: 24            # default 12
```

### JSON-Only Output Contract

The `internal/dream/parser.go` expects a single JSON object response. The Gemini invoker sets:

```json
"generationConfig": { "responseMimeType": "application/json" }
```

— which makes Gemini emit JSON natively. If you see "no JSON found" warnings in `dream/failed_responses.jsonl`, check that:

- Your model supports `responseMimeType` (most 2.5+ models do)
- No safety filter is partial-blocking the response

### Function Calling / Tool Use

Motes does **not** use Gemini's function-calling features. The dream cycle is single-turn JSON generation. There is no agentic loop inside the cycle itself.

If you want to add tool-use, that's a substantial architectural change — open a `decision` mote first.

---

## Common Gemini-Specific Pitfalls

| Symptom | Likely Cause | Fix |
|---------|--------------|-----|
| `gemini auth=vertex-ai requires gcloud CLI on PATH` | gcloud not installed | Install Google Cloud SDK |
| `gcloud auth print-access-token failed` | ADC not configured | `gcloud auth application-default login` |
| `gemini HTTP 403 ... permission denied` | Vertex AI API not enabled or service account lacks `aiplatform.user` role | Enable API and grant role |
| `gemini response blocked: SAFETY` | Safety filter triggered on mote body content | Lower the threshold (e.g. `BLOCK_NONE` for technical-only projects), or rephrase the offending mote |
| `gemini HTTP 429` | Quota exceeded | Lower `rate_limit_rpm`; request quota increase |
| Empty visions output | `responseMimeType` not honored, or model returned a refusal that isn't a hard safety block | Inspect `dream/failed_responses.jsonl` |

---

## Pointers

- **Setup & full reference:** [docs/providers.md](docs/providers.md), [docs/configuration.md](docs/configuration.md)
- **Source of truth (Gemini invoker):** `internal/dream/gemini_invoker.go`
- **Test patterns:** `internal/dream/gemini_invoker_test.go` — uses `httptest` + injected `tokenSource` so tests run without `gcloud` installed
- **Doctor advisories:** `runDoctorProviderAdvisories` in `cmd/mote/cmd_doctor.go`

---

## License

This file, like the rest of the project, is AGPL-3.0-or-later.
