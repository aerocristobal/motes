# Dream Cycle Provider Reference

Motes' dream cycle (`internal/dream/`) is the only LLM-using subsystem. As of v0.4.11 it dispatches through an `Invoker` interface to one of three backends, configured per stage in `.memory/config.yaml`.

This document covers setup, troubleshooting, cost guidance, and how to add a new provider. For the field-by-field config schema, see [configuration.md](configuration.md). For provider-agnostic agent conventions, see [`AGENTS.md`](../AGENTS.md). For Gemini-specific guidance, see [`GEMINI.md`](../GEMINI.md).

---

## At a Glance

| Backend | Auth Mechanism | Network Calls | Required Tools |
|---------|---------------|---------------|----------------|
| `claude-cli` (default) | Whatever `claude` CLI is configured with | `claude` shells out to Anthropic | `claude` binary on PATH |
| `openai` | API key (env var name or literal) | HTTPS to `api.openai.com` | None — uses Go stdlib `net/http` |
| `gemini` | Vertex AI ADC (gcloud OAuth token) | HTTPS to `*-aiplatform.googleapis.com` | `gcloud` on PATH; GCP project with Vertex AI enabled |
| `codex-cli` | Whatever `codex` CLI is logged in as (Sign in with ChatGPT or API key) | `codex` shells out to OpenAI | `codex` binary on PATH (`codex login` already run) |
| `gemini-cli` | Whatever `gemini` CLI is logged in as (Login with Google or API key) | `gemini` shells out to Google | `gemini` binary on PATH (already authenticated) |

All three satisfy the same `Invoker` contract:

```go
type Invoker interface {
    Invoke(prompt string, tier string) (InvokeResult, error)
    Model() string
}
```

The orchestrator constructs **two** invokers per dream run — one for batch reasoning, one for reconciliation — so backends and models can mix freely between stages.

---

## Backend Setup

### `claude-cli` (Default)

No additional configuration needed beyond having the `claude` CLI on `PATH`. The default `mote init` config uses this backend with `claude-sonnet-4-6` (batch) and `claude-opus-4-6` (reconciliation).

```yaml
dream:
  provider:
    batch:
      backend: claude-cli
      auth: oauth                   # placeholder — claude handles its own auth
      model: claude-sonnet-4-6
    reconciliation:
      backend: claude-cli
      auth: oauth
      model: claude-opus-4-6
```

**Implementation:** `internal/dream/invoker.go` (`ClaudeInvoker`). Shells out via `exec.CommandContext` with the JSON-only system prompt and 5-minute timeout.

### `openai`

```yaml
dream:
  provider:
    batch:
      backend: openai
      auth: OPENAI_API_KEY          # env var name; resolved at runtime
      model: gpt-4o-mini
    reconciliation:
      backend: openai
      auth: OPENAI_API_KEY
      model: gpt-4o
    rate_limit_rpm: 60
```

**Run:**
```bash
export OPENAI_API_KEY=sk-...
mote dream
```

**Auth resolution:** The `auth` field is interpreted as an environment variable name when `os.LookupEnv` finds it. Otherwise it's used as a literal credential. Names that look like env vars (`UPPERCASE_WITH_UNDERSCORES`) but aren't exported produce an explicit error rather than being silently sent as a credential — see `internal/dream/auth.go` for the heuristic.

**Implementation:** `internal/dream/openai_invoker.go`. Uses `net/http` only (no SDK dependency).

### `gemini` (Vertex AI ADC)

```yaml
dream:
  provider:
    batch:
      backend: gemini
      auth: vertex-ai               # sentinel value — uses Application Default Credentials
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
```

**Setup:**
```bash
gcloud auth application-default login
gcloud auth print-access-token | head -c 20    # confirm it works
mote dream
```

**Auth flow:** Per-request, the invoker shells out to `gcloud auth print-access-token` and uses the result as a Bearer token. Tokens last ~1 hour; gcloud caches and refreshes them transparently.

**Endpoint:** `https://{gcp_region}-aiplatform.googleapis.com/v1/projects/{gcp_project}/locations/{gcp_region}/publishers/google/models/{model}:generateContent`

**Safety:** All four harm categories (`HARASSMENT`, `HATE_SPEECH`, `SEXUALLY_EXPLICIT`, `DANGEROUS_CONTENT`) use the configured threshold. A `finishReason: SAFETY | RECITATION | PROHIBITED_CONTENT` response is treated as non-retryable — `RetryPolicy` will not waste attempts on a deterministic block.

**API-key path** (`generativelanguage.googleapis.com`) is intentionally not supported in v0.4.11. If you need it, see "Adding a New Provider" below or open an issue.

**Implementation:** `internal/dream/gemini_invoker.go`. See [`GEMINI.md`](../GEMINI.md) for Gemini-specific tuning advice.

### `codex-cli` (OpenAI Codex CLI delegation)

```yaml
dream:
  provider:
    batch:
      backend: codex-cli
      auth: oauth                   # placeholder — codex handles its own auth
      model: gpt-5-codex            # optional; empty falls through to codex's default
    reconciliation:
      backend: codex-cli
      auth: oauth
      model: gpt-5-codex
```

**Setup:**
```bash
npm install -g @openai/codex     # or brew install --cask codex
codex login                       # Sign in with ChatGPT (OAuth) or API key
which codex                       # confirm on PATH
mote dream
```

**Auth flow:** Motes never reads `~/.codex/auth.json` directly. Each invocation shells out to `codex exec --skip-git-repo-check --ephemeral --output-last-message <tmp> [-m <model>] -` with the prompt on stdin; the CLI handles OAuth (or API-key) auth and token refresh internally. The trade-off vs the `openai` backend is per-call subprocess startup (~50–200 ms); dream cycles run infrequently (default `schedule_hint_days: 2`) so this is rarely a hotspot.

**Implementation:** `internal/dream/codex_invoker.go`.

### `gemini-cli` (Gemini CLI delegation)

```yaml
dream:
  provider:
    batch:
      backend: gemini-cli
      auth: oauth                   # placeholder — gemini handles its own auth
      model: gemini-2.5-flash       # optional; empty falls through to gemini's default
    reconciliation:
      backend: gemini-cli
      auth: oauth
      model: gemini-2.5-pro
```

**Setup:**
```bash
# Install the Gemini CLI (see https://geminicli.com)
gemini                            # first run prompts Login with Google
which gemini                      # confirm on PATH
mote dream
```

**Auth flow:** Motes never reads `~/.gemini/oauth_creds.json` directly. Each invocation shells out to `gemini -p <prompt> -o text [-m <model>]`; the CLI handles OAuth (or API-key) auth and token refresh internally. The JSON-only system directive is prepended to the prompt body (Gemini CLI has no `--system-prompt` flag).

**When to choose `gemini-cli` vs `gemini`:** Use `gemini-cli` if you've already done Login with Google for the CLI and don't want a separate gcloud / Vertex AI setup. Use `gemini` (Vertex AI ADC) if you need fine-grained GCP project routing, billing controls, or higher rate limits.

**Implementation:** `internal/dream/gemini_cli_invoker.go`.

---

## Mixing Backends

Batch and reconciliation are independent. Common patterns:

### Cheap batch, capable recon

```yaml
dream:
  provider:
    batch:
      backend: openai
      auth: OPENAI_API_KEY
      model: gpt-4o-mini
    reconciliation:
      backend: claude-cli
      auth: oauth
      model: claude-opus-4-6
```

### Local default, paid escalation

```yaml
dream:
  provider:
    batch:
      backend: claude-cli            # uses your existing claude subscription
      auth: oauth
      model: claude-sonnet-4-6
    reconciliation:
      backend: gemini                # only the single recon call hits Vertex AI
      auth: vertex-ai
      model: gemini-2.5-pro
      options:
        gcp_project: your-gcp-project
```

---

## Cost Guidance

`mote dream` reports an estimated cost based on the configured model and the per-million-token rates table in `internal/dream/pricing.go`. As of 2026-04-25:

| Model | Input $/MTok | Output $/MTok |
|-------|--------------|---------------|
| `claude-sonnet-*` | 3.0 | 15.0 |
| `claude-opus-*` | 15.0 | 75.0 |
| `gpt-4o-mini` | 0.15 | 0.60 |
| `gpt-4o` | 2.50 | 10.0 |
| `o1-mini` | 3.0 | 12.0 |
| `o1` | 15.0 | 60.0 |
| `gemini-2.5-flash` | 0.30 | 2.50 |
| `gemini-2.5-pro` | 1.25 | 10.0 |

Unknown model names return `0` from `EstimateCost` — the cost line will read `~$0.0000`. That's a UI signal that pricing data is missing, not that the call was free.

**Refreshing rates:** Edit `pricingTable` in `internal/dream/pricing.go` and update the date comment. Run `go test ./internal/dream/ -run TestEstimateCost` to verify.

---

## Rate Limiting

`dream.provider.rate_limit_rpm` (shared between stages) configures a token-bucket rate limiter. `0` (default) means unlimited.

Set this when:
- Your project has many batches (`mote dream` reports something like "Batch 8/12 (clustered, 50 motes)...")
- You're on a tight provider quota (OpenAI free tier, Vertex AI default project quotas)

The limiter blocks before each `Invoke` call. It does not affect concurrent batch fan-out (which is governed separately by `dream.batching.max_concurrent`).

---

## Verification

### Configuration sanity

```bash
mote doctor
```

`runDoctorProviderAdvisories` in `cmd/mote/cmd_doctor.go` flags:
- Empty auth on `openai`/`gemini`
- Env-var-shaped `auth` value with the variable not exported
- Missing `gcp_project` for `gemini`
- Non-`vertex-ai` auth on `gemini`
- Unknown backend value (also blocked at config-load time)

All checks are local — no live API probe.

### Live smoke test

```bash
mote add --type=note --title="seed1" --body "test mote one"
mote add --type=note --title="seed2" --body "test mote two"
mote dream --dry-run    # asserts factory + scanner without LLM calls
mote dream              # real call; observe cost line in output
```

---

## Troubleshooting

### `unknown dream provider backend "..."`

Backend value isn't in the allowlist. Valid values: `claude-cli`, `openai`, `gemini`, `codex-cli`, `gemini-cli`, or empty (defaults to `claude-cli`).

### `codex-cli backend requires the codex CLI on PATH`

The `codex` binary isn't installed or isn't in `$PATH`. Install via `npm install -g @openai/codex` (or `brew install --cask codex`) and run `codex login` once.

### `gemini-cli backend requires the gemini CLI on PATH`

The `gemini` binary isn't installed or isn't in `$PATH`. Install per https://geminicli.com and run it once interactively to complete Login with Google.

### `codex invocation failed` / `gemini invocation failed`

The CLI exited non-zero. The error includes its stderr — most often this is an auth issue (run `codex login` / re-authenticate gemini) or a sandbox / approval prompt that motes can't answer non-interactively. Re-run with the CLI directly to see the underlying issue.

### `auth %q looks like an environment variable name but is not set`

You set `auth: OPENAI_API_KEY` but didn't export it. Either:
- `export OPENAI_API_KEY=sk-...` and re-run, or
- Use a literal value (not recommended for production).

### `openai HTTP 401: invalid_api_key`

Bad API key. Check `op://Personal/OpenAI/api_key` (or wherever you store it) matches the env var.

### `openai HTTP 429: rate_limit_exceeded`

Set `dream.provider.rate_limit_rpm` to your tier's limit (e.g. 500 for OpenAI Tier 2). The retry policy handles transient 429s but won't paper over a sustained quota breach.

### `gemini auth=vertex-ai requires gcloud CLI on PATH`

Install the Google Cloud SDK and ensure `which gcloud` succeeds.

### `gcloud auth print-access-token failed`

Run `gcloud auth application-default login` to set up ADC.

### `gemini HTTP 403: permission denied`

Either Vertex AI API isn't enabled on the project, or the principal lacks the `roles/aiplatform.user` role. From the GCP console:
- APIs & Services → Enable Vertex AI API
- IAM → grant your principal `Vertex AI User`

### `gemini response blocked: SAFETY`

The configured `safety_threshold` is too aggressive for your mote content. Set `safety_threshold: BLOCK_NONE` in the offending stage's `options`. This is non-retryable by design — RetryPolicy gives up immediately so you don't burn quota on a deterministic block.

### Empty visions / "no JSON found" in `dream/failed_responses.jsonl`

The model returned text rather than JSON. For OpenAI this usually means a rate-limit-flavored 200 response; for Gemini it can mean the safety filter partial-blocked the response. Inspect the `response_preview` field in the failed-responses log.

---

## Adding a New Provider

To add a fourth backend (e.g. local Ollama, AWS Bedrock, Azure OpenAI):

1. **Create `internal/dream/<name>_invoker.go`** implementing the `Invoker` interface:
   ```go
   type FooInvoker struct { /* ... */ }
   var _ Invoker = (*FooInvoker)(nil)
   func NewFooInvoker(entry core.ProviderEntry, rateLimitRPM int) (*FooInvoker, error) { ... }
   func (fi *FooInvoker) Invoke(prompt, tier string) (InvokeResult, error) { ... }
   func (fi *FooInvoker) Model() string { return fi.model }
   ```
2. **Wire it into the factory** in `internal/dream/invoker.go` `NewInvoker`:
   ```go
   case "foo":
       return NewFooInvoker(entry, rateLimitRPM)
   ```
3. **Add to the allowlist** `core.ValidProviderBackends` in `internal/core/config.go`.
4. **Add pricing rows** to `pricingTable` in `internal/dream/pricing.go`.
5. **Add tests** modeled on `openai_invoker_test.go` (httptest-based) or `gemini_invoker_test.go` (httptest + injected token source).
6. **Update `cmd/mote/cmd_doctor.go`** `providerStageAdvisories` with a case for the new backend.
7. **Document** in this file, [`configuration.md`](configuration.md), and the relevant agent file (`AGENTS.md` / `GEMINI.md` / new `<NAME>.md`).

The dream pipeline (`orchestrator.go`, prompts, parser, retry, rate-limit) does not change — that's the whole point of the abstraction.

---

## License

This file, like the rest of the project, is AGPL-3.0-or-later.
