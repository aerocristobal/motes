# UI Philosophy

Mote is an agent-facing tool first, terminal tool second. Output must be useful
to both readers — a `mote ls` that looks great in a terminal but breaks a shell
pipeline is a regression. This document codifies the design rules new commands,
flags, and renderers MUST follow. Reviewers (human or AI) cite specific Rule
sections to reject PRs that violate them.

The five Rules below are the spine. The **Operational details** appendix
preserves the renderer invariants the codebase already enforces in tests.

---

## Rule 1: No emoji-style status icons

Emoji glyphs (🔴 🟠 🟡 🔵 ⚪, 🚦, 🚀, ✅/❌ as status badges) are forbidden in
mote output. They render at variable widths, break column alignment, do not
survive `LANG=C` or POSIX-only TTYs, and are illegible to screen readers. Worse,
their meaning is conventional in one cultural context and noise in another — a
🟢 that means "good" to one reader means nothing to a script.

Status semantics belong in **text** (the literal status string) and in
**dim/normal weight** (see [Operational details → Closed items recede](#closed-items-recede)).
Iconography, when used at all, follows Rule 2.

### Accept

`internal/format/icon.go::StatusIcon` returns one of `○ ◐ ✓ ● ❄` for the five
mote statuses, with ASCII fallbacks `o p x . -` selected by an explicit flag.
The single live callsite, `cmd/mote/cmd_show.go:497`, passes the `ascii`
boolean through from the caller's mode decision. No emoji is reachable.

### Reject

Adding `case "blocked": return "🔴"` (or any of `🔴🟠🟡🔵⚪`) to a renderer.
Reviewer cites this Rule and the PR is sent back. If the underlying need is
"surface blocked state visually," the answer is a dim/bold weight on the text,
not a colored disk.

---

## Rule 2: Small Unicode semantic symbols (with ASCII fallback)

When a symbol IS used, it must come from a small, fixed, mono-width set of
semantically chosen Unicode glyphs (BMP code points, single column width). Each
symbol must have an ASCII fallback selected by an explicit signal — typically
the renderer's mode decision — so non-Unicode terminals and grep-pipelines
remain readable.

The fallback rule is not aesthetic; it is operational. Pipelines (`mote ls |
awk`), CI logs, and minimal containers cannot be assumed UTF-8. A Unicode-only
glyph with no fallback is a latent bug.

### Accept

`format.StatusIcon` has two parallel branches: the Unicode set `○ ◐ ✓ ● ❄` and
the ASCII set `o p x . -`. The `ascii` boolean selects one. The set is closed
(five symbols) and the mapping is documented at the call site.

### Reject

Adding `return "↻"` (or any new Unicode glyph) to a renderer with no
corresponding ASCII branch, or growing the symbol set ad-hoc per command so
that `mote ls` and `mote show` disagree on what an in-progress task looks like.
If a new symbol is needed, extend `StatusIcon` (or a sibling table in
`internal/format/`) so all renderers share the mapping.

---

## Header structure

<a id="header-structure"></a>

Header lines split into two zones:

- **Left** = status icon + Accent-colored ID + ` · ` + title.
- **Right** = `[<status-icon> <STATUS>` + (` w<weight>` if the mote is open) + `]`, right-aligned to the terminal width.

Closed motes (status in `{completed, archived, deprecated}`) mute the entire
line via `format.Muted` and drop the weight segment from the right zone —
weight is a triage signal, and triage is done. When the title is too long
for the available left-zone budget, it truncates with `…` (pretty) or `...`
(plain); the right zone is never truncated. Width detection lives in
`format.TerminalWidth()`, which honors the undocumented test-only
`MOTE_FORCE_WIDTH` env var.

In `--plain` mode the same content renders with ASCII icons, a literal
`  |  ` separator between zones, no right-alignment padding, and no ANSI.
This is the LLM-grep-friendly shape — `awk -F'|'` works.

The header is rendered by `format.RenderHeader` (internal/format/header.go),
which takes a `HeaderInput` struct of primitives so the format package does
not need to import `internal/core`. Two callsites today: `cmd_show.go`'s
default + plain rendering, and `cmd_ls.go`'s per-row loop. Other renderers
(`mote pulse`, `mote constellation`, `mote context`) intentionally do NOT
call it — a static scope guard in the test suite enforces that.

### Accept

The first line of `mote show T1abc7` on a 100-column TTY reads:

```
○ T1abc7 · Add login form                                                       [○ ACTIVE w0.6]
```

Each row of `mote ls --ready` has the same shape. The eye saccades to one
zone or the other depending on whether the reader is scanning for the mote
ID (left) or work state (right). In a `--plain` pipeline the same row is:

```
o T1abc7 - Add login form  |  [o ACTIVE w0.6]
```

### Reject

A PR that re-introduces the old flat `=== T1abc7 ===` header followed by
key/value `type: …`, `status: …`, `title: …`, `weight: …` lines for `mote
show` — that block is the redundancy the two-zone header replaced. A PR
that introduces a NEW renderer (say `mote audit`) emitting a one-line mote
summary should call `format.RenderHeader` instead of hand-rolling its own
layout, so all header-emitting commands stay consistent.

---

## Rule 3: Recovery/fix operations consolidate into `mote doctor --fix`

Mote will accrue many things that can drift, regress, or need repair —
git-hook templates today, config schema tomorrow, index integrity after that.
Each one MUST land as a check inside `runDoctorChecks` /
`runDoctorAdvisories` with a `--fix`-mode branch, NOT as a new top-level
command (`mote repair`, `mote recover`, `mote heal`, `mote fix-X`, …).

Reasons: (1) a single entry point is the contract users (and agents) memorise;
(2) `mote doctor` already aggregates diagnostics, so users see related issues
together; (3) each new top-level command spends from the budget in Rule 5;
(4) consolidation forces authors to think about idempotency and dry-run
semantics, which a one-off `mote heal` invariably skips.

### Accept

`cmd/mote/cmd_doctor.go:30` declares
`doctorCmd.Flags().Bool("fix", false, "Repair mote-managed git-hook drift in place (never touches user-authored hooks)")`.
The `--fix` flag is read once and threaded into the drift-check function. New
fix operations slot into the same flag by adding a new check; the user-facing
contract is unchanged.

### Reject

A PR adding `mote repair` (or `mote recover`, `mote fix-config`, etc.) as a
new top-level command. Reviewer cites this Rule. Documented remediation: add
a new check to `runDoctorChecks` that detects the drift, and extend
`--fix` to repair it. If the surface area is large enough to merit a
sub-command, prefer `mote doctor fix <subject>` (cobra subcommand on
`doctor`), NOT a new sibling of `doctor`.

---

## Rule 4: Prefer flags on existing commands over new top-level commands

New behavior is, by default, a flag on the nearest existing command. A new
top-level command is the last resort, justified only when the behavior is
genuinely orthogonal to every existing command's purpose.

Why: flags compose (`mote ls --ready --type=task --tag=docs`), share help and
discovery, and inherit the surrounding command's argument validation. A new
top-level command duplicates all of that and adds another row to
`mote --help`.

The test, before adding `mote <new-cmd>`: "Could this be
`mote <existing-cmd> --<new-behavior>`, where `<existing-cmd>` is the
command whose noun the new behavior is about?" If yes, the answer is the
flag.

### Accept

- `mote ls --ready` (cmd/mote/cmd_ls.go:67) — surfaces ready tasks as a
  filter on the existing list, not a `mote ready` command.
- `mote update --status=in_progress` (cmd/mote/cmd_update.go:57) — status
  transition as a field on `update`, not `mote transition` or `mote status`.
- `mote update --claim` (cmd/mote/cmd_update.go:67) — multi-agent claim
  workflow as a flag, not `mote claim`. Composes with `--status`.

### Reject

A PR adding `mote claim` as a sibling of `mote update`, duplicating
update's mutual-exclusivity matrix and concurrency model. Reviewer cites
this Rule plus the existing `update --claim` precedent.

---

## Rule 5: ~30 top-level commands is the discoverability ceiling

Cobra's `--help` output, agent autocomplete, and the contributor's mental
model all degrade past roughly 30 top-level commands. The ceiling is the
discoverability budget, not a hard SLO: at ~30 it is a yellow flag, past 30 it
is a serious design pressure that should be answered by consolidation, not
accommodation.

Today, mote is **over** the ceiling (46 top-level commands as of v0.4.42).
That is a known overshoot. The CI tooth to mechanically enforce a threshold
(warn-at-N, fail-at-N+k) lands in STORY-UIPHI-002 once Three Amigos confirms
the numbers in light of the current count. The Rule applies in PR review
regardless: a new top-level command needs an affirmative justification, not
silent inclusion.

### Accept

A PR adds the new behavior as a flag on an existing command (see Rule 4) and
the top-level count is unchanged. CI passes. The contributor's design memo
in the PR description either does not mention this Rule or notes "covered by
flag".

### Reject

A PR adds `rootCmd.AddCommand(fooCmd)` to `cmd/mote/cmd_foo.go` where the
behavior could be a flag on `bar`. Reviewer quotes this Rule. Documented
remediation: move the behavior to `bar --foo`. If the PR persists, ship the
flag; reject the new command.

---

## When NOT to color

Color is functional, not aesthetic. ANSI escapes are emitted ONLY when they
carry information the reader needs to act on. The renderer MUST NOT color:

- **Descriptions.** Field labels, mote bodies, frontmatter values, prose
  generally — color them and you've made them harder to copy, grep, and diff.
- **Examples in `--help` text.** `--help` is plain-text contract. ANSI in
  help output corrupts `mote ls --help | less` and degrades on dumb
  terminals.
- **Every list item.** Color must carry information, so it cannot apply to
  every row. If every row of `mote ls` were green, "green" would mean
  nothing. The existing precedent: only `format.IsClosed` rows are muted;
  live rows render plain. (See [Operational details → Closed items recede](#closed-items-recede).)
- **Pure decoration.** Brand colors, mood lighting, gradient banners, "make
  it look nice" — none of it ships. If the change would be invisible to a
  shell pipeline, the change is doing no work.

These prohibitions are independent of the palette. The palette itself —
semantic color tokens for status, severity, and category — is owned by the
sibling section below and filled in by STORY-COLOR-001.

---

## Semantic color palette

<a id="semantic-color-tokens"></a>

All ANSI styling in mote flows through six **semantic color tokens** defined
once in `internal/format/style.go`: `Pass`, `Warn`, `Fail`, `Accent`,
`Muted`, `Command`. Renderers call the token whose name answers the question
"what does this color mean?" — never a raw `\x1b[32m`. Reviewers grep
`\\x1b\[` and reject any hit outside `internal/format/style.go`.

Each token is an `AdaptiveColor{Light, Dark string}` — a pair of 24-bit hex
values. At render time mote calls `detectBackground()`, which reads the
`COLORFGBG` environment variable (a convention shared with rxvt, urxvt, and
lipgloss). When the trailing background field is in the high-color range
(8–15) mote emits the Light variant; otherwise — including absent or
unparseable values — mote emits the Dark variant. The default is dark
because over-darkening is recoverable; washing out an unintended light
variant on a dark terminal is not.

The palette:

| Token   | Use for                                                 | Light hex      | Dark hex       |
|---------|---------------------------------------------------------|----------------|----------------|
| Pass    | Successful checks, passing tests, healthy state         | #86b300        | #c2d94c        |
| Warn    | Drift detected, recoverable problems, deprecation       | #f2ae49        | #ffb454        |
| Fail    | Errors, failed checks, blocking conditions              | #f07171        | #f07178        |
| Accent  | Mote IDs, command examples, primary highlights          | #399ee6        | #59c2ff        |
| Muted   | Closed motes, secondary metadata                        | SGR dim (`2`)¹ | SGR dim (`2`)¹ |
| Command | Inline command names in help / error text               | #55b4d4        | #95e6cb        |

Pass / Warn / Fail are pinned from the Ayu palette as published in
beads-recommendations §23.13. Accent / Command are the Ayu Sublime / VS Code
values picked to match the Pass/Warn/Fail tonality during STORY-COLOR-001
Three Amigos.

¹ **Muted is implemented via the SGR dim attribute (`\x1b[2m`) rather than
a 24-bit hex color**, preserving byte-for-byte compatibility with
STORY-MUTED-001's existing snapshot tests. Dim composes multiplicatively
with the terminal's foreground, so it always reads as "secondary" against
any background — no light/dark variant needed. The `MutedColor`
`AdaptiveColor` constant in `internal/format/style.go` (`#828c99` /
`#5c6773`) documents what a hex-based Muted *would* look like, and is the
value future renderers should reach for if they need an explicit fill (a
header badge background, for example). It is not what `format.Muted` emits
today.

**Color is never the sole signal.** Every closed mote is also rendered as
muted *text*; every fail message also contains the word `FAIL` (or names the
failing category in plain English); every warning also carries the `⚠` (or
`!`) prefix. A grep of `mote doctor` output sees the same information
whether the terminal supports color or not. This is the redundancy required
by Section 508 and by WCAG 2.1's "color is not the only visual means" guideline
— a future formal audit is not in scope for this section, but the
single-source-of-truth palette is the hook for one.

The color decision (when to emit ANSI at all) and the suppression contracts
(`--no-color`, `NO_COLOR`, non-TTY, `--json`) are unchanged from the
[When NOT to color](#when-not-to-color) rules above and from
[Operational details → The color decision](#the-color-decision). Tokens are
*what* color to emit; those contracts are *whether*.

---

## Operational details

The Rules above govern WHAT goes in the output. The sections below codify HOW
the renderers implement them. These invariants are guarded by tests in
`internal/format/` and exercised by `MOTE_FORCE_TTY=1` in CI.

### Closed items recede

This is Tufte's principle, applied operationally: information that has been
resolved should fade so the eye lands on what still needs attention. In mote
that means rows whose status is `completed`, `archived`, or `deprecated`
render in a muted style (ANSI dim) on a TTY. Live statuses (`active`,
`in_progress`) render normally.

The closed set lives at `internal/format/style.go::IsClosed`. It is the
inverse of `core.IsLive`.

### The color decision

Three signals combine, in this precedence order:

1. **`--no-color` flag** (persistent on `rootCmd`) — explicit user intent.
2. **`NO_COLOR` env var** (any non-empty value) — the
   [no-color.org](https://no-color.org) standard.
3. **TTY detection** on `os.Stdout`.

`format.ShouldColor(isTTY, noColorFlag)` encodes the logic in one place; all
renderers call it the same way:

```go
useColor := format.ShouldColor(format.IsTTY(os.Stdout.Fd()), noColorFlag)
```

The internal helper `useColorOutput()` in `cmd/mote/main.go` wraps the call
so command files don't need to re-import `os` and `format` for the same
two-line idiom.

### The column-alignment invariant

ANSI escapes have zero visible width but non-zero byte width. A printf
format string like `%-24s` counts *bytes*, not display cells, so inserting an
escape sequence into a column **before** padding will visually drift the
adjacent column by the length of the escape.

**Rule:** pad the raw string first, then wrap the entire row in escapes.

```go
row := fmt.Sprintf("%-24s  %-14s  ...", id, typ, ...)   // padded raw
if format.IsClosed(status) {
    row = format.Muted(row, useColor)                    // wrap after
}
fmt.Println(row)
```

`TestMuted_ColumnAlignment` guards this invariant by asserting
`StripANSI(Muted(s, true)) == s`.

### Backward compatibility for textual markers

`mote ls` has historically prefixed deprecated titles with `[deprecated] `.
Existing scripts and log scrapers grep for that literal. The muting is
**additive**: the prefix stays, and the muted region wraps the prefix too,
so on a TTY a deprecated row reads dim *and* still contains the marker for
greppers.

### The test-only TTY override

`MOTE_FORCE_TTY=1` makes `format.IsTTY` return true regardless of the real
file descriptor. It exists so deterministic tests can exercise the colored
code path even though `go test` captures stdout into a pipe. It mirrors the
`MOTE_GLOBAL_ROOT` test-isolation convention in
`internal/core/safety.go`. **Do not document it in user-facing help.** It is
not a supported configuration knob.

### The `--plain` / `--pretty` / `--json` mode trio

Mote has two orthogonal output axes:

| Axis    | Flags                              | What it controls                                          |
|---------|------------------------------------|-----------------------------------------------------------|
| color   | `--no-color`, `NO_COLOR`           | Whether ANSI escapes are emitted                          |
| layout  | `--plain`, `--pretty`              | Whether Tufte chrome (headers, padded columns) is emitted |

- `--no-color` strips ANSI but **keeps** the pretty Tufte layout. Existing
  scripts that pass `--no-color` see no layout change.
- `--plain` strips ANSI **and** collapses the layout to one record per line
  (or `key: value` per field for object views).
- `--pretty` is the inverse of TTY detection: it forces ANSI + padded
  columns even on a pipe (useful for CI logs and capture sessions).
- `--json` is the agent contract. ANSI bytes break parsers; the color
  decision MUST come *after* the JSON branch returns.

`--json`, `--pretty`, and `--plain` are mutually exclusive. The default (no
mode flag) preserves the pre-v0.4.36 behaviour: TTY → styled, non-TTY → no
color but Tufte layout intact. `TestLs_NonTTY_ByteStableAgainstSnapshot`
guards that default against drift.

The mode decision lives in `cmd/mote/main.go::outputMode(jsonFlag bool)` — a
single function returning `ModeAuto` / `ModeJSON` / `ModePretty` / `ModePlain`
so renderers branch on one value, not three flags.

### Adding color to a new view

1. Compute `useColor := useColorOutput()` once at the top of the render.
2. For each renderable unit, build the padded raw string with `Sprintf`.
3. Conditionally wrap with `format.Muted(row, useColor)` based on
   `format.IsClosed(status)` (or your own predicate).
4. Add a test that proves the non-TTY path emits no `\x1b` bytes.
5. Add a test under `MOTE_FORCE_TTY=1` that proves the rule fires.

### Still out of scope

- A styled-output library (`lipgloss`, `tablewriter`, etc.).
- Markdown or HTML output modes.
- `--plain` on write commands (`add`, `update`, …) — their output is
  already minimal confirmation text.

---

## See also

- **STORY-COLOR-001 (§23.13)** — semantic color tokens (palette section above).
- **STORY-HDRZ-001 (§23.14)** — two-zone header convention.
- **STORY-UIPHI-002** — CI command-count check enforcing Rule 5.
