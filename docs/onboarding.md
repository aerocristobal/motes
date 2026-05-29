# Getting Started with Motes

This guide covers how to adopt motes in a new or existing project.

## Fresh Start

For a brand new project:

```bash
mote init
```

This creates:
- `.memory/` directory with `nodes/`, `dream/`, `strata/` subdirs
- `.memory/config.yaml` with sensible defaults
- `.memory/index.jsonl` (empty edge index)
- Appends a `## Motes` section to your `CLAUDE.md`

Start creating motes immediately:

```bash
mote add --type=task --title="Set up project" --tag=setup --body "Initial scaffolding"
```

## Transitioning from MEMORY.md

If your project has a `MEMORY.md` file with accumulated knowledge:

**Option A: Automatic (recommended)**

```bash
mote onboard
```

`onboard` auto-detects `MEMORY.md` in the project root, parses it into sections, infers mote types (context, lesson, decision), and creates individual motes. The original file is archived as `MEMORY.md.migrated.<date>`.

**Option B: Explicit**

```bash
mote migrate MEMORY.md
```

Same migration logic, but you specify the file path. Useful when your MEMORY.md is in a non-standard location.

**Preview first:**

```bash
mote onboard --dry-run
mote migrate MEMORY.md --dry-run
```

## Transitioning from Beads

If your project uses `.beads/` for issue tracking:

```bash
mote onboard
```

`onboard` detects `.beads/issues.jsonl` and imports open issues as task motes. Beads priorities map to mote weights (P0 = 1.0, P4 = 0.3). Bug-type issues become lesson motes with `failure` origin.

By default only open issues are imported. To include closed issues:

```bash
mote onboard --include-closed
```

### Preferred: install via plugin marketplace (Claude Code)

If you use Claude Code, the fastest path is to install the bundled `mote` plugin from its marketplace registration. Inside Claude Code, run:

```
/plugin marketplace add aerocristobal/motes
/plugin install mote@motes
```

The plugin ships the four mote skills (`mote-capture`, `mote-retrieve`, `mote-plan`, `mote-subagent`) and the lifecycle hooks (SessionStart, PreCompact, UserPromptSubmit, Stop) — the same surface `mote onboard` would write to `~/.claude/skills/` and `~/.claude/settings.json`, but bundled with the binary so it refreshes on each release rather than going stale until you re-run `mote onboard`.

When you subsequently run `mote onboard` in a project, it detects the marketplace install and **skips** the dotfile shim path for Claude Code — the summary reports `Claude Code: integrated via plugin (skipped dotfile install)`. Codex is treated symmetrically once OpenAI's plugin marketplace ships; the artifact at `plugins/mote/.codex-plugin/plugin.json` is forward-looking. Gemini CLI has no marketplace path yet — the existing `~/.gemini/settings.json` + `~/.agents/skills/` dotfile install is still the only way to wire Gemini.

If you don't (yet) use Claude Code's marketplace, the dotfile install path below is unchanged and remains the fallback.

### What gets auto-installed

`mote init` and `mote onboard` automatically:

- **Install Claude Code hooks** — `SessionStart`, `PreCompact`, `UserPromptSubmit`, and `Stop` hooks in `~/.claude/settings.json`
- **Install PreToolUse safety hooks** — `block-interactive-cmds.sh` (denies bare `rm`/`cp`/`mv` that may hang on macOS `-i` aliases; bypass with `-f`, `/bin/rm`, or `command rm`), `block-gh-watch.sh` (denies `gh run watch`, which burns the GitHub API quota; use `gh run view --log` or `gh run list` instead), and `block-mote-rm.sh` (denies raw deletes of `.memory/nodes/**` — use `mote delete` — and direct writes to `.memory/index.jsonl` / `.memory/mote_bm25.json` — use `mote index rebuild`). Scripts land in `~/.claude/hooks/` and are wired under `hooks.PreToolUse[].matcher: "Bash"`.
- **Install mote skills** — `mote-capture`, `mote-retrieve`, `mote-plan`, `mote-subagent` to `~/.claude/skills/`
- **Migrate bd hooks** — Replaces `bd prime` → `mote prime` and `bd sync` → `mote session-end` in existing hooks
- **Install Codex hooks** (when `~/.codex/` is detected, or `--codex` is passed) — `SessionStart`, `UserPromptSubmit`, `Stop` in `~/.codex/hooks.json`; sets the `codex_hooks = true` feature flag in `~/.codex/config.toml`; mote skills also written to `~/.agents/skills/`. See [CODEX.md](../CODEX.md).
- **Install Gemini CLI settings** (when `~/.gemini/` is detected, or `--gemini` is passed) — `SessionStart`, `BeforeAgent`, `SessionEnd` in `~/.gemini/settings.json` (with `300000ms` timeout on `SessionEnd` because Gemini's 60s default would kill the heavy flush); `context.fileName` configured to load both `GEMINI.md` and `AGENTS.md`; mote skills also written to `~/.agents/skills/`. See [GEMINI.md](../GEMINI.md).
- **Install per-project git hooks** (when the project is a git working tree) — `post-checkout` runs `mote prime --hook --mode=resume` so agent context refreshes after a branch switch, and `pre-commit` refuses commits that stage edits to derived files (`.memory/index.jsonl`, `.memory/mote_bm25.json`). Templates are embedded in the `mote` binary and update with it — re-running `mote onboard` (or `mote githooks install`) after upgrading repairs drift; `mote doctor --fix` does the same. Each hook carries a `# managed-by: mote githooks install` sentinel so re-runs never clobber user-authored scripts (use `mote githooks install --force` to override an intentional conflict).

The `~/.agents/skills/` install is shared between Codex and Gemini CLI — both honor that path at higher precedence than their tool-specific defaults, so one install reaches both.

> **Migration note (v0.4.39):** Earlier mote versions installed a soft-warning `pre-commit` hook ("no active task found"). That hook lacks the new `# managed-by: mote githooks install` sentinel, so `mote githooks install` will flag it as a conflict. Run `mote githooks install --force` once to replace it with the current template, or delete `.git/hooks/pre-commit` manually if you prefer.

### Post-onboard cleanup

After verifying the import:

1. **Remove `.beads/`** — Use `mote onboard --cleanup` to auto-remove, or delete manually once satisfied

## Going Global

To set up global (cross-project) memory:

```bash
mote onboard --global
```

This creates `~/.motes/` with the same structure as project memory. If `~/.beads/` exists, global beads issues are imported too. Existing users with motes content under the legacy `~/.claude/memory/` are migrated automatically on the first `mote` command — only motes-owned files move; Claude's auto-memory (`MEMORY.md` and top-level `*.md`) is left in place.

Use `mote promote <id>` to copy project-local motes to the global layer for cross-project access.

## What Happens During Onboard

`mote onboard` runs these steps in order:

1. **Detect** — Scans for `.beads/issues.jsonl`, `MEMORY.md`, `.memory/`, and `CLAUDE.md`
2. **Report** — Prints what was found and what will happen
3. **Init** — Creates `.memory/` if it doesn't exist (same as `mote init`)
4. **Migrate MEMORY.md** — Parses sections, creates typed motes, archives original
5. **Import beads** — Converts open issues to task motes (idempotent — won't duplicate on re-run)
6. **Rebuild index** — Regenerates `index.jsonl` from all mote frontmatter
7. **Update CLAUDE.md** — Appends the `## Motes` section if not already present

## Workflow Cheat Sheet

### Session start

```bash
mote prime
```

Outputs scored, relevant context for the current work. Shows active tasks, recent decisions, and related knowledge.

### During work

```bash
# Create motes for decisions, lessons, and discoveries
mote add --type=decision --title="Use JWT" --tag=auth --body "Stateless, scales horizontally"
mote add --type=lesson --title="Retry logic needed" --tag=api --body "External API drops connections"

# Link related knowledge
mote link <id1> relates_to <id2>
mote link <story-id> depends_on <epic-id>

# Find available work
mote ls --ready
mote pulse

# Query context on a topic
mote context authentication
```

### Session end

```bash
mote session-end
```

Flushes batched access counts, suggests crystallization candidates, and provides maintenance hints.

Every few sessions, run the dream cycle for deeper automated maintenance:

```bash
mote dream              # Analyze graph, produce visions
mote dream --review     # Review and apply/reject each vision
```

The dream cycle detects missing links, stale content, contradictions, and more. Each finding becomes a "vision" that you review interactively. See [docs/maintenance.md](docs/maintenance.md) for the full maintenance workflow.

## Example CLAUDE.md Configurations

- [Project-level CLAUDE.md](example-claude-md-project.md) — What a project CLAUDE.md looks like with motes
- [Global CLAUDE.md](example-claude-md-global.md) — What `~/.claude/CLAUDE.md` looks like with motes

## Contributing to motes

If you are working on the motes repo itself (rather than adopting motes in your own project), run `make install` (or `make install-hooks`) after cloning. This wires `.githooks/pre-commit` via `git config core.hooksPath`, giving you `gofmt` and `--new-from-rev=HEAD` linting on every commit. See [LINTING.md](LINTING.md) for the policy.

Contributors who already use the python [`pre-commit`](https://pre-commit.com) framework across their repos can instead run `pre-commit install` from the repo root to wire the same hooks via `.pre-commit-config.yaml`. The two paths are equivalent — pick one. Don't run both: `make install-hooks` sets `core.hooksPath=.githooks/`, which overrides any `pre-commit install` you've already done.
