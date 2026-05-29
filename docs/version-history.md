# Version History

The head of this list (the first `- **vX.Y.Z** —` bullet) is the canonical user-facing version statement for mote and is enforced equal to `internal/version.Value` by `scripts/check-versions.sh`. Add a new bullet here for every release (this is what `scripts/bump-version.sh X.Y.Z` does automatically). Older entries below are not policed by CI.

## Version History

- **v0.4.40** — Adaptive `mote prime` MCP-mode detection across the three host settings files (`~/.claude`, `~/.codex`, `~/.gemini`); explicit `--mcp` / `--full` overrides with mutual-exclusivity; documented size budget in `docs/agents-guide.md` (MCP ~75 tokens, CLI ~2500 tokens); JSON envelope additively gains `mode` and `mode_source` (STORY-ADAPRIME-001).
- **v0.4.39** — First-class `mote githooks install` from binary-embedded templates (post-checkout + pre-commit); `mote onboard` installs them automatically; `mote doctor --fix` repairs mote-managed drift (STORY-HOOKINST-001).
- **v0.4.38** — CI hygiene: `check-doc-flags` subcommand and `doc-flags` CI job fail the build on stale flag references in docs (STORY-DOCFLAGS-001).
- **v0.4.37** — `mote ls --ready --explain` surfaces per-mote justification for why each mote is ready (dep status, claim availability, score components).
- **v0.4.36** — `--plain` and `--pretty` layout modes for read commands; orthogonal to `--no-color`; mutually exclusive with each other and with `--json` (STORY-PLAIN-001).
- **v0.4.35** — Versioned JSON envelope contract with `schema_version` field on every machine-readable output (STORY-JSCHEMA-001).
- **v0.4.34** — `--metadata-field` and `--has-metadata-key` filters on `ls` and `search` for querying motes by frontmatter metadata.
- **v0.4.33** — `--execution-only` flag prints only orchestration-hint metadata; agent contract documents the read-execution-before-prose pattern (STORY-EREAD-001).
- **v0.4.32** — Time-based scheduling: `due_at`/`defer_until` metadata and `ls --overdue` filter.
- **v0.4.31** — Orchestration-hint metadata on motes (preferred model, reasoning effort, parallelism budget).
- **v0.4.30** — Visual decay for closed motes: completed/deprecated motes render dimmed to reduce visual weight in lists.
- **v0.4.29** — Progressive disclosure for `mote show`: `--short` / default / `--long` to control the level of detail.
- **v0.4.28** — First-class memory verbs: `remember`, `memories`, `recall`, `forget` mirror the lightweight note-taking workflow.
- **v0.4.27** — Empty-state tests codify the `ls --ready` / `update --claim` contract.
- **v0.4.26** — `discovered_from` non-blocking provenance link type (STORY-DISC-001).
- **v0.4.25** — Atomic `update --claim` primitive for multi-agent coordination (lock acquisition with optimistic concurrency).
- **v0.4.24** — `mote prime` silent-failure model: hook failures no longer block sessions; opt in to verbose output with `--debug`.
- **v0.4.23** — `mote prime --json` emits pure JSON (no banner/log lines) to satisfy BR-23-2 Scenario 2.
- **v0.4.22** — `mote prime` truncation directive and `last_prime.txt` persistence so subsequent runs can pick up where the previous left off.
- **v0.4.21** — Opt-in pre-commit framework configuration (`.pre-commit-config.yaml`).
- **v0.4.20** — Staged-only Go pre-commit hook with re-stage of formatted files.
- **v0.4.19** — Claude Code PreToolUse safety hooks block destructive writes to `.memory/`.
- **v0.4.18** — Strata: fix basename-collision bug that grew `chunks.jsonl` to multiple gigabytes.
- **v0.4.17** — Dream: salvage advisory link types and tag_refinement visions that were being dropped by reconciliation.
- **v0.4.16** — Global motes: intake guardrails on `promote` and `prime`; test isolation for global-scope operations.
- **v0.4.15** — Dream cycle backends: add `codex-cli` and `gemini-cli` (OAuth via existing CLI installs).
- **v0.4.14** — Tool-neutral global memory; multi-agent shims; layered provider configuration.
- **v0.4.13** — Gemini CLI onboarding: refactored `GEMINI.md` to a tight Gemini-CLI prompt (~110 lines, with `@AGENTS.md` import). Vertex AI dream-cycle backend material consolidated in `docs/providers.md`. Added `docs/example-gemini-config.md` and a working `.gemini/settings.json` at the repo root with `context.fileName: ["GEMINI.md", "AGENTS.md"]` and an explicit 300000ms `SessionEnd` timeout (Gemini's default 60s would kill the heavy `mote session-end --hook` flush mid-flight). `mote onboard` auto-detects `~/.gemini/` and installs the same settings; pass `--gemini` to opt in explicitly. Skills now install at `~/.agents/skills/` when either Codex or Gemini CLI is enabled (shared condition).
- **v0.4.12** — Codex (OpenAI) onboarding: tightened `AGENTS.md` to a Codex-friendly prompt (~80 lines), added `CODEX.md`, `docs/example-codex-config.md`, and a working `.codex/hooks.json` at the repo root. `mote onboard` auto-detects `~/.codex/` and installs hooks at `~/.codex/hooks.json` plus motes skills at `~/.agents/skills/` (alongside the existing `~/.claude/skills/`). Pass `--codex` to opt in explicitly.
- **v0.4.11** — Multi-provider dream cycle: `Invoker` interface with backend dispatch (`claude-cli`, `openai`, `gemini` Vertex AI ADC). Per-stage provider configuration so batch and reconciliation can use different backends. `mote doctor` provider advisories. `config.yaml` now generated with backend hint comments via `yaml.v3` Node API.
- **v0.4.10** — Larger batches (50 motes), tighter cap (12 batches); refreshed model IDs.
- **v0.4.9** — Dream cycle token consumption reduced ~60%.
- **v0.4.8** — `in_progress` status to distinguish queued from in-flight work.
- **v0.4.7** — Lens mode: N parallel LLM runs with distinct mental model lenses (structural, survivorship bias, feedback loops, etc.) instead of redundant self-consistency voting. `CrossLensAgreement` confidence signal. `dream --quality --lens` per-lens breakdown. Vision provenance display in `--review`.
- **v0.4.6** — Graph integrity: cross-project ref detection (`--cross-project`), `clean-links` command, doctor advisories (link density, chain depth, tag fragmentation).
- **v0.4.5** — Second-order impact awareness: vision scoring shows downstream impact in `dream --review`.
- **v0.4.4** — Stocks and flows: inflow/outflow metrics in `stats`, bloat detection in `doctor`.
- **v0.4.2** — Global dream quality ledger: per-cycle metrics across projects; `dream --quality` and `dream --compare`.
- **v0.4.0** — Secret detection: security scanning of mote body content for embedded credentials.
- **v0.3.19** — Search filters (`--type`, `--tag`, `--status`); doctor complexity checks; context weighting.
- **v0.3.18** — Strata code artifact connections.
- **v0.3.15** — Concept vocabulary layer (wiki-links in body auto-create concept edges).
- **v0.3.14** — BM25 auto-link on mote creation.
- **v0.3.13** — Global-by-default knowledge: decision/lesson/explore/context/question stored in `~/.claude/memory/` by default.
- **v0.3.0** — Beads feature transfer: JSONL import/export, external refs, `--json` flags, scan cache, cluster summarization.
- **v0.2.0** — Hierarchical planning: parent/child tasks, acceptance criteria, `plan`/`progress`/`check` commands.
- **v0.1.x** — Core system: mote CRUD, graph linking, scoring, context/prime, dream cycle, strata, constellations.
