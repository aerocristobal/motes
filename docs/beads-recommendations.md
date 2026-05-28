# Beads → Mote: Recommendations for Incorporation

Source: analysis of [`gastownhall/beads`](https://github.com/gastownhall/beads) at the
time of writing (commit graph as of 2026-05-06). Beads is a Dolt-backed,
agent-first issue tracker that has converged on many useful agent-ergonomics
patterns. This document distills the patterns from beads's hooks, workflows,
and instructions that would benefit `mote` and notes the ones that don't fit
mote's design.

Conventions used below:

- **Impact:** `H` high / `M` medium / `L` low — operator value to mote users.
- **Effort:** `S` small / `M` medium / `L` large — engineering work in mote.
- Each section ends with a short *why* and a concrete *adopt* note.

---

## 1. Claude Code PreToolUse safety hooks (Impact: H, Effort: S)

Beads ships two PreToolUse hooks wired in `.claude/settings.json`:

- `block-gh-watch.sh` — denies `gh run watch` (3-second polling burns the
  5000/hr GitHub API quota and has historically blocked all crew members for
  up to an hour during releases).
- `block-interactive-cmds.sh` — denies `cp`/`mv`/`rm` without `-f` (macOS
  shells frequently alias these to `-i`, which silently hangs an agent
  waiting for `y/n` input it cannot provide).

Both hooks read `tool_input.command` from stdin via `jq`, return a
`hookSpecificOutput.permissionDecision: "deny"` JSON envelope, and explain
the bypass (`command rm`, absolute paths, or `-f`) in
`permissionDecisionReason`.

**Why:** these are real footguns we have all hit. Mote agents run identical
shells on identical macOS hosts and benefit from the same guardrails.

**Adopt for mote:**

1. Add `.claude/hooks/block-interactive-cmds.sh` verbatim — it is
   project-agnostic.
2. Add a mote-specific `block-mote-rm.sh` that denies `rm`/`trash` against
   `.memory/nodes/**` (mote nodes are append-mostly; deletes should go through
   `mote trash`).
3. Optionally add a hook denying direct edits to `.memory/index.jsonl` and
   `.memory/mote_bm25.json` outside `mote` itself (these are derived).
4. Wire in `.claude/settings.json` under `hooks.PreToolUse[].matcher: "Bash"`.

---

## 2. Staged-only Go pre-commit hook (Impact: M, Effort: S)

`.githooks/pre-commit` formats and lints **only staged Go files**, then
re-stages auto-fixes:

```bash
staged_go_files=$(git diff --cached --name-only --diff-filter=ACM -- '*.go')
[ -z "$staged_go_files" ] && exit 0
echo "$staged_go_files" | xargs gofmt -w
echo "$staged_go_files" | xargs git add
CGO_ENABLED=0 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1 \
    run --new-from-rev=HEAD --fix || exit 1
echo "$staged_go_files" | xargs git add
```

Two details worth copying:

- `--new-from-rev=HEAD` — only block on lints introduced by this commit;
  baseline warnings stay tracked separately (beads has `docs/LINTING.md`).
- `CGO_ENABLED=0` for the lint pass avoids typecheck panics from
  CGO-only transitive deps.

The hooks dir is wired with `git config core.hooksPath .githooks` and
declared in `make install` so contributors don't have to opt in manually.

**Adopt for mote:** create `.githooks/pre-commit` with the same
staged-files-only pattern, then add `git config core.hooksPath .githooks`
to mote's `Makefile` `install` target.

---

## 3. First-class `mote hooks install` (Impact: H, Effort: M)

Beads embeds its hooks in the binary and exposes:

```
bd hooks install        # install git hooks (pre-commit, post-merge, etc.)
bd migrate hooks --dry-run
bd doctor --fix         # remediates hook drift among other things
```

Hooks are NOT external scripts checked into the consumer project — they ship
inside the bd binary. This is a meaningful upgrade over external script
templates because:

- Hooks update with the tool, not the project.
- New consumers get them via `bd init` / `bd hooks install`, no copy-paste.
- `bd doctor --fix` can re-install drifted or missing hooks.

**Adopt for mote:** mote already has `cmd_hook.go` and `cmd_onboard.go` —
extend them so `mote onboard` (or new `mote hooks install`) writes mote's
session-start/session-end hooks into `.git/hooks/` from binary-embedded
templates. Today these hooks live in user dotfiles, which means new
contributors clone the repo and miss them.

---

## 4. CLI design discipline: cognitive overload rules (Impact: M, Effort: S)

Beads's `AGENTS.md` and `AGENT_INSTRUCTIONS.md` codify CLI/UI design rules
explicitly:

- **No emoji-style icons** (🔴🟠🟡🔵⚪) — they cause cognitive overload.
- **Use small Unicode symbols** with semantic lipgloss colors:
  `○ ◐ ● ✓ ❄` for status, `● P0/P1` for priority.
- **Recovery/fix operations consolidate into `bd doctor --fix`** — never a
  separate `bd recover` / `bd repair`. Adding commands is a cognitive
  liability and is gated on "fundamentally different operation, not a
  convenience wrapper".
- **Prefer flags on existing commands** (`bd list --stale`) over new
  top-level commands (`bd stale`).
- **Run `bd --help` and count** — approaching 30 top-level commands signals
  a discoverability problem.

**Adopt for mote:** add a short *CLI Design Principles* section to
`docs/internals.md` (or a new `docs/UI_PHILOSOPHY.md`). Mote already has
~30 top-level commands; this rule would prompt some consolidation
(candidates: `mote crystallize`/`mote dream` could share root, `mote
constellation`/`mote stats`/`mote pulse` could collapse).

---

## 5. Time-based scheduling for tasks (Impact: H, Effort: M)

Beads added `--due` and `--defer` (GH#820) with rich query flags:

```bash
bd create "Task" --due=+6h
bd create "Task" --defer=tomorrow
bd update <id> --due=+2d
bd update <id> --defer=""        # clear defer
bd list --deferred
bd list --due-before=+2d
bd list --due-after="next monday"
bd list --overdue
bd ready --include-deferred
```

Defer hides items from the ready queue until a target time. Due dates surface
overdue work. Both accept relative (`+6h`, `+1w`) and natural-language
(`tomorrow`, `next monday`) inputs.

**Why:** agents need to "snooze" tasks ("retry this in 6h after rate limit
resets"; "wait for downstream dep until next week") without losing them.
Today motes lacks any temporal model.

**Adopt for mote:** add `due_at` and `defer_until` fields to mote
frontmatter; teach `mote ls --ready` to filter out `defer_until > now`; add
`mote ls --overdue` and `mote ls --include-deferred`. This composes nicely
with mote's existing `--status=ready` semantics.

---

## 6. Execution metadata as orchestration hints (Impact: H, Effort: M)

Beads introduced first-class structured metadata for execution:

```
execution_agent_type
execution_suggested_model
execution_reasoning_effort
execution_mode             # local | delegated | parallel
execution_parallel_group
```

The agent contract is explicit: *read execution metadata before prose* — it
is authoritative when present, and the parent agent must read these fields
before spawning subagents because **a subagent cannot change its model or
reasoning effort after launch**.

**Why:** this is the missing piece for agent swarms. Mote already
encourages multi-agent capture but provides no mechanism to say "this task
is a parallel-group delegated to a haiku subagent at low reasoning."

**Adopt for mote:** standardize five frontmatter keys (same names) under a
new `execution:` block, and have `mote show <id>` surface them prominently
ahead of body text. Document the contract in `docs/agents-guide.md`.

---

## 7. Atomic `--claim` for tasks (Impact: H, Effort: S)

```
bd update <id> --claim --json
```

Sets assignee + flips `status` to `in_progress` atomically (compare-and-swap
on prior status). Prevents two crew members from both grabbing the same
ready task.

**Adopt for mote:** add `mote update <id> --claim` that sets `status=in_progress`
only if currently `ready`/`open`, and stamps the agent ID
(`MOTE_AGENT_ID` env var). Returns non-zero if already claimed by another
agent — agents can then loop on `mote ls --ready` without races.

---

## 8. `discovered-from` link type (Impact: M, Effort: S)

When working on `bd-42` an agent may discover a follow-up bug. Beads
formalizes this with a `discovered-from` dependency type:

```bash
bd create "Found bug" --description="..." -p 1 --deps discovered-from:bd-42
```

It is a non-blocking link (does not affect the ready queue) but preserves
*provenance* — you can always trace why an issue exists.

**Adopt for mote:** mote has `mote link` with several types — add a
`discovered-from` semantic-link kind. Mote's graph already supports it; this
is mostly a labeling and convention change plus surfacing in
`mote context <id>`.

---

## 9. Hierarchical IDs for epics (Impact: M, Effort: M)

Beads supports hierarchical IDs like `bd-a3f8.1.1`:

- `bd-a3f8` — Epic
- `bd-a3f8.1` — Task within the epic
- `bd-a3f8.1.1` — Sub-task

These are addressable, sortable, and visually convey scope. Combined with
`bd dep tree` they replace separate epic/sub-issue tracking.

**Adopt for mote:** consider an opt-in hierarchical suffix when a parent
relation is set at create time. The current opaque IDs (`motes-Tdib70p5i3...`)
optimize for collision-resistance but lose readability for humans skimming a
plan. Could coexist by keeping the hash as a long form and adding a
short hierarchical alias.

---

## 10. Test isolation discipline (Impact: H, Effort: S)

Beads's instructions repeatedly enforce test isolation:

- `BEADS_DB=/tmp/test.db` env var for manual testing — points the entire CLI
  at a throwaway database.
- `t.TempDir()` for Go tests, building DB paths under it.
- Explicit warning in CLI when a "Test*" issue is created in production DB.
- `git config core.hooksPath .git/hooks` inside test fixtures because the
  developer's global `core.hooksPath` can leak into temp repos and produce
  flaky behavior.
- A `.test-skip` file lists known-broken tests with GH issue refs; the
  custom `./scripts/test.sh` wrapper honors it.

**Why:** mote tests touch real `.memory/` directories — same risk class as
beads writing to a real Dolt DB. Mote has commit `37ffe41 feat(global):
intake guardrails on promote/prime + test isolation` indicating awareness;
the formal patterns above would harden it further.

**Adopt for mote:**

- Document a `MOTE_DIR=/tmp/mote-test ./mote …` pattern for manual testing
  and add a CI assertion that prevents nodes whose title starts with `Test`
  in `.memory/nodes/` on `main`.
- Provide a `.test-skip` (or annotation) for tests waiting on upstream
  fixes, with required issue ref.
- Add the `core.hooksPath` reset inside any temp-repo helper.

---

## 11. PR preflight script for contributor protection (Impact: M, Effort: S)

`scripts/pr-preflight.sh` is a read-only, agent-callable gate:

```bash
scripts/pr-preflight.sh --search "<topic>" --repo gastownhall/beads
scripts/pr-preflight.sh <pr-number>     --repo gastownhall/beads
```

It checks for existing open PRs on the same topic before an agent
implements parallel work, and for an individual PR it surfaces
`isCrossRepository`, `isDraft`, mergeability, review state, large-diff
signals, `.beads` data changes, and missing tests for code changes. Output
is a `[block]` / `[pass]` / `[next]` style checklist.

The companion policy is in `CONTRIBUTING.md` and `PR_MAINTAINER_GUIDELINES.md`,
both invoked from `AGENTS.md` as required reading before any PR triage.

**Why:** mote will eventually take external contributions — and the
"agent silently rewrites a contributor's PR" failure mode is exactly the
class of mistake worth pre-empting.

**Adopt for mote:** port `pr-preflight.sh` largely verbatim (it is
repo-agnostic) and add a minimal `CONTRIBUTING.md` so the preflight has a
policy to enforce.

---

## 12. Doc-flag freshness check in CI (Impact: M, Effort: S)

`scripts/check-doc-flags.sh` runs in CI and:

1. Extracts every flag from `bd help --all`.
2. Greps the docs/website/skills tree for `bd <command> --<flag>` patterns.
3. Flags any reference that does not exist in the live CLI.
4. Separately greps for known-removed commands (`bd sync`, `bd import`) with
   an allowlist for `CHANGELOG`, `removed`, `deprecated` mentions.

This is the single highest-leverage piece of doc rot prevention I have
seen. Beads has many docs (`docs/` has 50+ files) and they stay accurate
because CI fails on drift.

**Adopt for mote:** add `scripts/check-doc-flags.sh` patterned on this.
Mote's `docs/` is also large enough (`prd.md`, `internals.md`,
`onboarding.md`, `providers.md`, `configuration.md`, etc.) that drift is
near certain.

---

## 13. Version-consistency check (Impact: M, Effort: S)

`scripts/check-versions.sh` runs in CI and asserts that every place a
version is hard-coded matches: `cmd/bd/version.go`, the npm package, the
MCP package, the Claude/Codex plugin manifests, the marketplace manifest,
the README, the PLUGIN.md. The companion `scripts/bump-version.sh <ver>
--commit` updates all of them atomically. Beads literally filed `bd-66`
when only `version.go` was bumped and the others drifted; this is the
prevention.

**Adopt for mote:** mote already publishes via Homebrew. Add a tiny
`scripts/check-versions.sh` that asserts `internal/version.go` matches
`README.md`, `Makefile` (if version is referenced), and any future package
manifest. Cheap insurance.

---

## 14. Release gate evidence files (Impact: M, Effort: S)

`release-gates/be-eqw-gate.md` is a per-release evidence document with:

- A criteria table (review PASS, acceptance criteria met, tests pass, no
  high-severity findings, branch clean, branch diverges cleanly).
- "Tests run on release branch" with command, result, runtime, notes.
- "Findings tracked from review" with severity and follow-up disposition.
- A final **Verdict** block.

These get checked in. Future regressions can audit *exactly* what was
verified at ship time.

**Why:** mote ships a CLI that other agents depend on. The "the deploy that
broke prod left no trail" failure mode is real.

**Adopt for mote:** add `release-gates/` and require a gate file before
tagging. The structure (criteria table + tests run + findings + verdict) is
generic and project-agnostic. Even one-line gates per release pay dividends
the first time something breaks.

---

## 15. Dual-audience instruction docs with divergence markers (Impact: L, Effort: S)

Beads splits its instructions across:

- `AGENTS.md` — short cross-tool quick-reference (any agent landing here
  knows where to look).
- `AGENT_INSTRUCTIONS.md` — detailed operational guide.
- `CLAUDE.md` — Claude-specific bits.
- `.github/copilot-instructions.md` — GitHub Copilot specifics.
- `GEMINI.md` / `CODEX.md` — per-tool variations.

Each top-level instruction file carries an HTML comment marker:

```html
<!-- bd-doctor-divergence: ok -->
```

`bd doctor` cross-references files that should agree and flags the marker
when divergence is intentional, so accidental drift is caught but
deliberate per-audience differences pass.

**Adopt for mote:** mote already has `AGENTS.md`, `CLAUDE.md`, `CODEX.md`,
`GEMINI.md` — add a `mote doctor` rule that reconciles them, and the
divergence-ok marker convention.

---

## 16. "Landing the plane" workflow, codified (Impact: M, Effort: S)

Mote already has this section in `CLAUDE.md`. Beads goes further:

- Spells out **non-negotiable** push step with an explicit anti-pattern
  ("NEVER say 'ready to push when you are' — YOU must push").
- Adds `git stash clear` and `git remote prune origin` cleanup.
- Requires the agent to provide a follow-up prompt format: `"Continue work
  on bd-X: [title]. [context done / next]"`.
- Codifies the example session bash commands so an agent can copy verbatim.

**Adopt for mote:** import beads's exact "Landing the Plane" anti-patterns
section. The framing ("the plane has not landed until `git push` succeeds")
is sticky and works.

---

## 17. CI hygiene patterns (Impact: M, Effort: S)

Beads's `.github/workflows/ci.yml` has a few patterns mote can copy directly:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event_name }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

— cancel stale runs when a PR is force-pushed, saving CI minutes.

```yaml
- uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
```

— pinned-by-SHA action versions with the human-readable tag in a comment
(supply-chain hygiene without losing readability).

A separate `detect-ci-tier` job decides whether the PR needs the full
embedded test matrix or just smoke tests, based on what changed. This keeps
fast-path PRs fast.

**Adopt for mote:** add `concurrency` cancel-in-progress, pin actions by
SHA, and consider a CI tier detector once mote's tests are slow enough to
justify it.

---

## 18. `--stealth` and `--contributor` modes (Impact: M, Effort: M)

Beads supports two off-the-beaten-path init modes:

- `bd init --stealth` — sets `no-git-ops: true`, doesn't install git hooks,
  the `.beads/` data is on disk but never gets committed. Lets a personal
  user adopt the tool inside a shared repo without imposing it on
  collaborators.
- `bd init --contributor` — for forked OSS repos: routes new issues to a
  separate planning DB (e.g. `~/.beads-planning`) so experimental task
  tracking does not leak into PRs.

Beads also auto-detects "maintainer" via SSH origin URLs.

**Why:** mote's adoption story today is *project-wide commitment*. Many
users want to try it on a personal branch without committing `.memory/`. A
`--stealth` mode lowers the friction.

**Adopt for mote:** mote already supports a `MOTE_DIR` env var by
<!-- doc-flags: ignore-next -->
convention; formalize `mote init --stealth` that writes
`.memory/config.yaml` with `no-git-ops: true` and adds `.memory/` to a local
(not committed) `.git/info/exclude`.

---

## 19. Embedded plugin packages for Claude/Codex (Impact: H, Effort: M)

Under `plugins/beads/` beads ships:

- `.claude-plugin/plugin.json`
- `.codex-plugin/plugin.json`
- `skills/beads/{SKILL.md, commands/, agents/, resources/, adr/}`

The plugin is *vendored alongside the source* and version-bumped together.
End users can install the plugin from the repo's marketplace registration
(`.claude-plugin/marketplace.json`) and instantly get all the agent
shims.

**Why:** today mote relies on dotfiles or per-user skill installation. A
bundled plugin would make `mote onboard` a one-shot for new users on Claude
Code, Codex, and Gemini.

**Adopt for mote:** add `plugins/mote/.claude-plugin/plugin.json` (and the
codex equivalent), move the existing `mote-capture`, `mote-plan`,
`mote-retrieve`, `mote-subagent` skills into the plugin, and register a
marketplace entry. Mote's `cmd_onboard.go` can detect the plugin and skip
the dotfile shims.

---

## 20. Messaging issue type for inter-agent comms (Impact: L, Effort: L)

Beads supports a `message` issue type with `--thread`, `replies_to`,
ephemeral lifecycle, and "mail delegation" — agents leave structured
messages for one another inside the same DB instead of free-form prose
captured nowhere.

**Why:** mote already has `feedback` motes. Promoting a similar lightweight
"mote with addressed-to and reply-to" pattern would let mote subagents send
structured findings back to the parent.

**Adopt for mote:** speculative — worth a design mote, not an immediate
implementation. The interesting subset is `replies_to` + a TTL field
("ephemeral"). Mote already has `mote trash` so the lifecycle pieces exist.

---

## 21. Pre-commit framework integration (Impact: L, Effort: S)

`.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/golangci/golangci-lint
    rev: v2.10.1
    hooks:
      - id: golangci-lint
        args: [--timeout=5m]
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v6.0.0
    hooks:
      - id: trailing-whitespace
      - id: end-of-file-fixer
      - id: check-yaml
      - id: check-added-large-files
```

Lets contributors who already use the python `pre-commit` framework get
mote's hooks for free without overriding `core.hooksPath`.

**Adopt for mote:** trivial copy — add the same file with golangci-lint
pinned and the four upstream hygiene hooks.

---

## 22. Test wrapper script with skip list (Impact: M, Effort: S)

`scripts/test.sh` wraps `go test`:

- Honors `.test-skip` (newline-delimited test names that get `-skip`).
- Default 3-minute timeout (configurable via `TEST_TIMEOUT`).
- `TEST_VERBOSE=1` env var maps to `-v`.
- `TEST_RUN=TestX` env var maps to `-run`.

The companion doc `.claude/test-strategy.md` instructs Claude to *always*
use the wrapper rather than `go test` directly, with concrete reasoning
("compilation 180s vs 3.8s actual test execution; running subsets of tests
doesn't save much time but is still better than seeing unrelated
failures").

**Adopt for mote:** mote's compile times are smaller, but the *consistency*
is the win — agents run identical commands locally and in CI, and a
`.test-skip` file with issue refs is the cleanest way to defer flaky tests.

---

## 23. Agent-facing context display patterns (Impact: H, Effort: M)

This section is a closer read of *how* beads talks to agents during the
critical workflow stages — `bd prime`, `bd ready`, `bd show`, `bd memories`
— and is worth treating as its own discipline. Mote already has analogous
commands (`mote prime`, `mote ls --ready`, `mote show`); the patterns below
are the rough edges beads has found that mote can pre-empt.

### 23.1 Adaptive context size: MCP mode vs CLI mode

`bd prime` detects whether the agent is running with the bd MCP server
active and emits two different payloads:

- **MCP mode** — ~50 tokens. Brief workflow reminders. Assumes the agent
  can call MCP tools to fetch detail on demand.
- **CLI mode** — ~1–2k tokens. Full command reference, session-close
  protocol, persistent memories.
- **`--memories-only`** — strips everything but the persistent-memory
  block. Used in compact hook contexts (`PreCompact`).
- **`--full` / `--mcp` flags** force the mode regardless of detection.

Detection logic looks at `~/.claude/settings.json` for an `mcpServers` key
containing `"beads"`.

**Adopt for mote:** `mote prime` today emits a fixed payload. Add MCP-mode
detection (mote will eventually have an MCP wrapper, but the *flag-based*
override is useful now), `--memories-only`, and document the size budget
(~50 tokens compact / ~1–2k tokens full) so the SessionStart hook stays
under the host's preview threshold.

### 23.2 Truncation directive prepended to prime output

Every prime output starts with a literal instruction:

```
[bd prime] If this output is truncated by your host, read the full
persisted hook output before continuing; it may contain project memories
and session rules not visible in the preview.
```

This is a small but high-signal pattern. Many hosts (Claude Code,
Cursor, Codex) truncate hook output for display, but the file is on disk.
The directive teaches the agent to look past the preview.

**Adopt for mote:** prepend the same literal sentence (with mote naming)
to every `mote prime` output. One-line code change, large robustness gain.

### 23.3 Customizable `PRIME.md` override with three-tier resolution

`bd prime` checks for a custom `PRIME.md` at three locations (in priority
order) and emits its contents verbatim if found:

1. `./.beads/PRIME.md` — clone-specific override (per-developer).
2. `<resolved-workspace>/PRIME.md` — shared override (per-project).
3. `~/.config/beads/PRIME.md` — user-global override.

`bd prime --export` dumps the default content as a starting point for
customization. This means a project can teach its agents project-specific
rules without forking bd, and a user can teach all their projects the same
rules in one place.

**Adopt for mote:** mote's prime context is currently baked into the
binary. Add the same three-tier resolution against `MOTE_PRIME.md` (or
<!-- doc-flags: ignore-next -->
`.memory/PRIME.md`). Mote already has `mote prime --export`-equivalent
plumbing in its skill system; making the override file-based is simpler.

### 23.4 Silent failure model for `mote prime`

`bd prime` is engineered to be safe to wire into a hook in any environment:

```go
// CRITICAL: No stderr output, exit 0
// This enables cross-platform hook integration
os.Exit(0)
```

If beads is not present, not a beads project, the DB is corrupt, the user
is on Windows in a sandbox — all the same: silent exit 0, no stderr. The
hook continues. The agent gets no context but no error either.

**Adopt for mote:** mote's prime hook already runs on session start, but
`mote prime` failing today emits a stderr message and a non-zero exit.
That breaks well-formed hook chains where mote is one of several setup
steps. Switch to silent-success-on-error semantics for prime specifically
(other commands keep their normal error model).

### 23.5 `bd remember / memories / forget / recall` as first-class verbs

Beads exposes persistent memory through four small CLI verbs rather than a
config-file convention:

```bash
bd remember "always run tests with -race flag"
bd remember "auth uses JWT not sessions" --key auth-jwt
bd memories                  # list all
bd memories dolt             # search by substring
bd recall <key>              # get one
bd forget <key>              # remove one
```

Keys are auto-slugified from the content unless `--key` is given. Memories
live in the kv store and are **injected at `bd prime` time** — no manual
loading. Memories are also exported as part of `bd export --json`.

This is meaningfully different from mote's current model where lessons are
captured as `--type=lesson` motes and surface only via `mote search`. The
"verb = intent" framing is sticky for agents (`remember` / `forget` is what
they want to *do*).

**Adopt for mote:** add `mote remember <text>`, `mote forget <key>`,
`mote recall <key>`, `mote memories [search]` as thin wrappers over a
new `kind=memory` mote type (or over an internal kv store). Make `mote
prime` inject the full set in compact form by default and `--memories-only`
just-the-memories. This collapses an entire workflow ("how do I save a
lesson? how does the next session see it?") into two CLI verbs.

### 23.6 Auto-refresh stale upstream data before prime

Before emitting context, `bd prime` calls `maybePullStaleLinearData`,
which:

1. Checks if the project has a Linear API key configured.
2. Checks if the local Linear cache is older than the staleness threshold.
3. If both yes, shells out to `bd linear sync --pull --json`.
4. Reports the count of pulled updates to *stderr* (not stdout — stdout is
   the agent-consumable context).

The agent thus starts every session with fresh upstream data without the
user having to remember to sync.

**Adopt for mote:** when mote grows external integrations (GitHub issue
import is already in `cmd_github_import.go`), apply the same pattern:
auto-pull on prime if the cache is stale. Keep the staleness threshold
configurable.

### 23.7 JSON envelope contract with `schema_version`

Beads is migrating from "raw JSON output" to an envelope:

```bash
export BD_JSON_ENVELOPE=1
```

```json
{
  "schema_version": 1,
  "data": { "id": "beads-abc", "title": "Example issue", ... }
}
```

Key contract details:

- `schema_version` is an **integer** that bumps only on **breaking** changes
  (rename, removal, type change). Additive changes do *not* bump.
- Errors emit JSON to **stderr** with `{schema_version, error, code}`.
- List commands and object commands have separate documented schemas in
  `docs/JSON_SCHEMA.md` with required vs optional fields.
- `--json` is the stable contract; `--format json` is for human-readable
  variants and is *not* a stable interface.
- A deprecation period: legacy default → opt-in envelope → v2.0 default
  envelope → v3.0 envelope only. Stderr deprecation notice during the
  transition.

**Adopt for mote:** mote emits JSON in a few commands today but does not
have a documented contract. Add a `docs/JSON_SCHEMA.md`, a `schema_version`
field, the same `MOTE_JSON_ENVELOPE=1` opt-in, and a deprecation timeline.
Future MCP integration is much easier when the JSON contract is explicit.

### 23.8 Three output modes per command: `--json` / `--pretty` / `--plain`

`bd ready` (and many others) accept three orthogonal output flags:

- `--json` — machine-readable, stable contract.
- `--pretty` — colored, Tufte-styled human output (default for TTY).
- `--plain` — colorless, minimal markup, line-oriented (for `grep`,
  `awk`, copy-paste into chat clients that mangle ANSI).

The three modes serve three audiences (agent, human-at-terminal,
human-at-pipeline). They are not redundant.

**Adopt for mote:** add `--plain` to mote's read commands (`ls`, `pulse`,
`stats`, `show`, `context`). Today mote falls back to colored or no-color
based on TTY detection, which is a coarser axis.

### 23.9 `--explain` flag on `ready`: surface *why* an issue is ready

`bd ready --explain` runs the dependency-aware ready calculation but adds
a per-issue justification:

```
bd-a3f8 — Add login form (P1)
  ready because: 2 of 2 blocking deps closed (bd-a3f7, bd-a3f5)
  parent epic: bd-a300 (open, in_progress)
```

For an agent deciding *which* ready issue to claim, this surfaces context
the agent would otherwise have to derive by walking the graph itself.

**Adopt for mote:** mote's `mote ls --ready` emits a flat list. Add `mote
ls --ready --explain` that surfaces blocker history, parent context, and
freshness ("not touched in 12d"). Agents waste turns walking the graph
when this could be one query.

### 23.10 Progressive disclosure: `--short` / default / `--long`

`bd show` has three density modes:

- `--short` — one line: `STATUS_ICON ID Priority [Type] Title`.
- *default* — header + metadata + description + dependencies + comments.
- `--long` — adds: extended timestamps, closure details, compaction
  history, gate fields, wisp metadata, source system, sender, ephemeral
  flags, mol-type, work-type, estimated minutes.

Default mode is a deliberate cut: the fields most likely to drive an
agent's next action. `--long` is opt-in for forensic work.

**Adopt for mote:** `mote show` today shows the full mote frontmatter and
body. Split into `--short` / default / `--long` so:

- `--short` is loop-friendly (`for id in $(mote ls --ready); do mote show
  $id --short; done`),
- default is the agent's "decide what to do" view,
- `--long` reveals seldom-needed fields like history pointers, embed
  vector status, last-prime timestamp.

### 23.11 "Read execution metadata before prose" rule

This is the operational counterpart to §6 (execution metadata schema). The
agent contract from `AGENT_INSTRUCTIONS.md`:

> When enacting a bd issue, inspect the structured metadata before using
> description or notes to choose execution mode, delegation, model,
> reasoning level, or parallel group:
>
> ```bash
> bd show <id> --json | jq '.[0] | {id,title,metadata,description,notes}'
> ```
>
> The execution metadata keys are authoritative when present. Use
> `description` for the work scope and `notes` for rationale or fallback
> context. Parent/orchestrator agents must read these fields before
> spawning subagents because a running subagent cannot change its model or
> reasoning effort after launch.

Two things to note:

1. The contract is enforced by *documentation*, not code — agents are
   *told* to read metadata first.
2. The reasoning is sharp: a running subagent cannot retroactively change
   its model. The orchestrator's first read decides everything.

**Adopt for mote:** when mote ships execution metadata (§6), copy this
exact contract into `docs/agents-guide.md` and into the mote-subagent
skill. The bash one-liner is portable.

### 23.12 Visual decay: closed items mute the entire row

Throughout the bd output formatters (`formatShortIssue`,
`formatDependencyLine`), there is a consistent pattern:

```go
if issue.Status == types.StatusClosed {
    return fmt.Sprintf("...",
        ui.RenderMuted(issue.ID),
        ui.RenderMuted(issue.Title),
        ui.RenderMuted(...))
}
// Active items: full color
```

Closed work *recedes* visually so the eye lands on actionable items.
Combined with the small-Unicode status icons (`○ ◐ ● ✓ ❄`), `bd dep tree`
on a partially-completed epic instantly shows what is left.

**Adopt for mote:** `mote ls`, `mote pulse`, and `mote context --planning`
should mute closed/done motes the same way. This is purely a renderer
change; no schema impact. Pairs well with §4.

### 23.13 Tufte semantic color tokens with AdaptiveColor

Beads's `docs/UI_PHILOSOPHY.md` codifies six tokens: `Pass / Warn / Fail /
Accent / Muted / Command`. Each is a `lipgloss.AdaptiveColor` with separate
light- and dark-terminal hex codes, sourced from the Ayu palette:

```go
ColorPass = AdaptiveColor{Light: "#86b300", Dark: "#c2d94c"}  // Green
ColorWarn = AdaptiveColor{Light: "#f2ae49", Dark: "#ffb454"}  // Yellow
ColorFail = AdaptiveColor{Light: "#f07171", Dark: "#f07178"}  // Red
```

The "When NOT to color" rules are equally useful: descriptions, examples
in help text, every list item, decoration. Color is functional, not
aesthetic.

**Adopt for mote:** mote's coloring today is ad-hoc per command. Centralize
in `internal/ui/styles.go` (mirroring beads), pick a palette with light/dark
variants, and document the "when not to color" rules in a new
`docs/UI_PHILOSOPHY.md`.

### 23.14 Header structure: identifier left, state right

`formatIssueHeader` produces:

```
○ bd-a3f8 · Add login form   [● P1 · OPEN]
```

Two visual zones with consistent semantics:

- **Left zone:** status icon, ID (accent-colored), title.
- **Right zone:** priority chip + state badge in `[…]` brackets.

Eye saccades naturally to one or the other. The pattern is consistent
across `bd show`, `bd list`, and `bd ready`.

**Adopt for mote:** `mote show` and `mote ls` today use a flat
"key: value" layout. The two-zone pattern reads faster, especially in
long ready-queue listings.

### 23.15 Queryable structured metadata: `--metadata-field` and `--has-metadata-key`

Beads's `metadata` JSON field on every issue is *queryable* directly from
the CLI:

```bash
bd ready --metadata-field execution_mode=parallel --json
bd ready --has-metadata-key execution_parallel_group --json
```

The storage layer validates the metadata key shape (no nested traversal,
no SQL injection). This makes the execution metadata of §6 actually
*useful* — orchestrators can ask "show me everything in parallel-group A"
in one call.

**Adopt for mote:** add `--metadata-field key=value` and
`--has-metadata-key key` filters to `mote ls`, `mote ls --ready`, and
`mote search`. With §6's execution metadata, this becomes the swarm
dispatcher's ready-queue.

### 23.16 Empty-state semantics for `--claim`

`bd ready --claim --json` returns `[]` (empty array) when nothing is
claimable, not an error. Agents can poll in a loop:

```bash
while ! claimed=$(bd ready --claim --json | jq -e '.[0]'); do sleep 30; done
```

The non-zero exit is reserved for *real* errors (DB unreachable, bad
flag). "Nothing to claim" is a normal outcome.

**Adopt for mote:** when adding `mote update --claim` (§7), make the
"nothing to claim" path return empty + exit 0, not a "no work found"
error. Let agents poll cleanly.

---

## What does NOT translate to mote

A few beads patterns are tightly coupled to the Dolt backend or to bd's
specific ergonomics and are noted only to forestall accidental copy:

- **`bd dolt push` / `bd dolt pull` and Dolt cell-level merge** — mote uses
  flat markdown files in git; standard git merge already handles this.
  Don't import the "JSONL is legacy" framing — mote's storage *is* the
  filesystem.
- **Dolt server mode (`bd init --server`)** — mote has no equivalent
  multi-writer scenario to solve.
- **`gms_pure_go` build-tag enforcement** — solves a Dolt-specific CGO
  linkage problem mote doesn't have.
- **`check-build-tags.sh`** — same, mote has no required build tags.
- **`bd compaction` of closed tasks for "memory decay"** — mote's `dream`
  and `crystallize` already do the equivalent. Don't import the naming.
- **MCP server as a Python package** — mote's surface is small enough to
  expose via direct CLI; an MCP wrapper is later-stage work and should
  follow Anthropic's reference patterns rather than beads's.
- **Explicit warning when creating "Test*" issues in prod DB** — useful in
  beads where the prod DB is committed; mote nodes are markdown so the same
  hazard is lower.

---

## Suggested adoption order

A pragmatic sequence by leverage-per-hour:

1. **§1, §2, §21, §23.2, §23.4** — Claude Code safety hooks, staged-only
   pre-commit, pre-commit framework config, prime truncation directive,
   silent-failure prime. One PR, wide blast radius — all small, high signal.
2. **§7, §8, §23.16** — atomic `--claim` (with empty-state semantics) and
   `discovered-from` link type. Tiny, unlocks multi-agent workflows.
3. **§23.5, §23.10, §23.12** — first-class `mote remember/memories/forget`,
   `--short`/`--long` progressive disclosure on `show`, mute-closed-rows.
   Reshapes the everyday agent workflow.
4. **§5, §6, §23.11, §23.15** — `--due`/`--defer`, `execution_*` metadata,
   the "read metadata before prose" contract, queryable `--metadata-field`
   filters. Schema additions; design first, then ship as a coherent block.
5. **§23.7, §23.8, §23.9** — JSON envelope contract, `--plain` mode,
   `--explain` flag on `ready`. Stabilizes agent-facing output for the
   long term.
6. **§12, §13, §17** — CI hygiene: doc-flag freshness, version
   consistency, concurrency cancel-in-progress + SHA-pinned actions.
7. **§3, §19, §23.1, §23.3** — first-class `mote hooks install`, bundled
   Claude/Codex plugin packages, adaptive MCP/CLI prime, customizable
   `PRIME.md` override. Adoption-friction reductions.
8. **§4, §15, §16, §22, §23.13, §23.14** — CLI design discipline doc with
   `docs/UI_PHILOSOPHY.md`, divergence markers, "Landing the Plane"
   anti-patterns, test wrapper, semantic color tokens with AdaptiveColor,
   two-zone header format. Documentation/process.
9. **§9, §11, §14, §18, §23.6** — hierarchical IDs, PR preflight,
   release-gates evidence files, stealth/contributor modes, auto-refresh
   stale upstream data on prime. Larger or more product-flavored changes.
10. **§20** — messaging issue type. Speculative; design mote first.

---

*Generated 2026-05-06 from analysis of `gastownhall/beads` `main`. Cross-checked
against the current motes tree (`cmd/mote/`, `internal/`, `docs/`, `skills/`,
`.memory/`) so as not to duplicate features mote already ships.*
