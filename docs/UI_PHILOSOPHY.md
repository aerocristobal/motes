# UI Philosophy

Mote is an agent-facing tool first, terminal tool second. Its output has to be
useful to both readers — a `mote ls` that looks great in a terminal but breaks
a shell pipeline is a regression. This page captures the rules the renderer
follows so future format work stays consistent.

## 1. Closed items recede

This is Tufte's principle, applied operationally: information that has been
resolved should fade so the eye lands on what still needs attention. In mote
that means rows whose status is `completed`, `archived`, or `deprecated`
render in a muted style (ANSI dim) on a TTY. Live statuses (`active`,
`in_progress`) render normally.

The closed set lives at `internal/format/style.go::IsClosed`. It is the
inverse of `core.IsLive`.

## 2. When NOT to color

Coloring is for *secondary* information that frames the data. Never color:

- **JSON output.** `--json` is the agent contract. ANSI bytes break parsers
  and downstream tooling. The color decision must come *after* the JSON
  branch returns.
- **Data values themselves.** ID strings, weights, titles, error messages,
  log lines — color them and you've made them harder to copy, grep, and diff.
- **Non-TTY output.** Pipes, redirects, CI logs, and `mote ls | grep ...`
  must be byte-stable.
- **Error messages.** Errors go to stderr; they must be readable without a
  color-capable terminal.

## 3. The color decision

Three signals combine, in this precedence order:

1. **`--no-color` flag** (persistent on `rootCmd`) — explicit user intent.
2. **`NO_COLOR` env var** (any non-empty value) — the [no-color.org](https://no-color.org)
   standard.
3. **TTY detection** on `os.Stdout`.

`format.ShouldColor(isTTY, noColorFlag)` encodes the logic in one place; all
renderers call it the same way:

```go
useColor := format.ShouldColor(format.IsTTY(os.Stdout.Fd()), noColorFlag)
```

The internal helper `useColorOutput()` in `cmd/mote/main.go` wraps the call
so command files don't need to re-import `os` and `format` for the same
two-line idiom.

## 4. The column-alignment invariant

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

## 5. Backward compatibility for textual markers

`mote ls` has historically prefixed deprecated titles with `[deprecated] `.
Existing scripts and log scrapers grep for that literal. The new muting is
**additive**: the prefix stays, and the muted region wraps the prefix too,
so on a TTY a deprecated row reads dim *and* still contains the marker for
greppers.

## 6. The test-only TTY override

`MOTE_FORCE_TTY=1` makes `format.IsTTY` return true regardless of the real
file descriptor. It exists so deterministic tests can exercise the colored
code path even though `go test` captures stdout into a pipe. It mirrors the
`MOTE_GLOBAL_ROOT` test-isolation convention in
`internal/core/safety.go`. **Do not document it in user-facing help.** It is
not a supported configuration knob.

## 7. What this isn't (yet)

We deliberately did not adopt a styled-output library (e.g. `lipgloss`) or
introduce semantic color tokens. The current toolbox is the `Muted` helper,
the `IsClosed` decision, `NO_COLOR` / `--no-color` (color axis), and as of
`v0.4.36` `--plain` / `--pretty` (layout axis — see below).

If a future story needs adaptive color, status palettes, or multi-style
rendering, `format.Muted` can become a one-line delegation to a heavier
library without touching any renderer.

### What landed in v0.4.36 (STORY-PLAIN-001)

`mote` now has two orthogonal output axes:

| Axis    | Flags                              | What it controls                                          |
|---------|------------------------------------|-----------------------------------------------------------|
| color   | `--no-color`, `NO_COLOR`           | Whether ANSI escapes are emitted                          |
| layout  | `--plain`, `--pretty`              | Whether Tufte chrome (headers, padded columns) is emitted |

The two are independent:

- `--no-color` strips ANSI but **keeps** the pretty Tufte layout. Existing
  scripts that pass `--no-color` see no layout change.
- `--plain` strips ANSI **and** collapses the layout to one record per line
  (or `key: value` per field for object views).
- `--pretty` is the inverse of TTY detection: it forces ANSI + padded
  columns even on a pipe (useful for CI logs and capture sessions).

`--json`, `--pretty`, and `--plain` are mutually exclusive. Default (no
mode flag) preserves the pre-`v0.4.36` behaviour: TTY → styled, non-TTY →
no color but Tufte layout intact. A `TestLs_NonTTY_ByteStableAgainstSnapshot`
guards that default against drift.

The mode decision lives in `cmd/mote/main.go::outputMode(jsonFlag bool)`
— a single function returning `ModeAuto` / `ModeJSON` / `ModePretty` /
`ModePlain` so renderers branch on one value, not three flags.

### Still out of scope

- Adaptive color / semantic color tokens (status = green/yellow/red, etc.).
- A styled-output library (`lipgloss`, `tablewriter`, etc.).
- Markdown or HTML output modes.
- `--plain` on write commands (`add`, `update`, …) — their output is
  already minimal confirmation text.

## 8. Adding color to a new view

1. Compute `useColor := useColorOutput()` once at the top of the render.
2. For each renderable unit, build the padded raw string with `Sprintf`.
3. Conditionally wrap with `format.Muted(row, useColor)` based on
   `format.IsClosed(status)` (or your own predicate).
4. Add a test that proves the non-TTY path emits no `\x1b` bytes.
5. Add a test under `MOTE_FORCE_TTY=1` that proves the rule fires.
