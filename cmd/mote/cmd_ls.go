// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/jsonenv"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List motes with optional filters",
	RunE:  runLs,
}

// LsOutput is the JSON output structure for mote ls --json.
type LsOutput struct {
	Motes []LsMoteEntry `json:"motes"`
}

// LsMoteEntry represents a mote in ls JSON output. ReadyExplanation is
// populated only when `--ready --explain` is set (STORY-EXPLAIN-001) and is
// omitted otherwise so byte-for-byte legacy output is preserved.
type LsMoteEntry struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	Status           string                 `json:"status"`
	Weight           float64                `json:"weight"`
	Title            string                 `json:"title"`
	ReadyExplanation *core.ReadyExplanation `json:"ready_explanation,omitempty"`
}

var (
	lsType    string
	lsTag     string
	lsStatus  string
	lsStale   bool
	lsReady   bool
	lsCompact bool
	lsParent  string
	lsJSON    bool
	lsExplain bool

	lsOverdue         bool
	lsIncludeDeferred bool
	lsDueBefore       string
	lsDueAfter        string

	lsMetadataField  []string
	lsHasMetadataKey []string
)

func init() {
	lsCmd.Flags().StringVar(&lsType, "type", "", "Filter by mote type")
	lsCmd.Flags().StringVar(&lsTag, "tag", "", "Filter by tag")
	lsCmd.Flags().StringVar(&lsStatus, "status", "", "Filter by status")
	lsCmd.Flags().BoolVar(&lsStale, "stale", false, "Show motes with no access in 90+ days")
	lsCmd.Flags().BoolVar(&lsReady, "ready", false, "Show tasks with zero unfinished blockers")
	lsCmd.Flags().BoolVar(&lsCompact, "compact", false, "One-line-per-mote compact output: ID: Title")
	lsCmd.Flags().StringVar(&lsParent, "parent", "", "Filter by parent mote ID")
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "Output in JSON format")
	lsCmd.Flags().BoolVar(&lsExplain, "explain", false, "Surface per-mote justification for ready motes (requires --ready)")

	lsCmd.Flags().BoolVar(&lsOverdue, "overdue", false, "Show active/in_progress motes whose due_at has passed, sorted by due_at ascending")
	lsCmd.Flags().BoolVar(&lsIncludeDeferred, "include-deferred", false, "When combined with --ready, do not hide motes whose defer_until is still in the future")
	lsCmd.Flags().StringVar(&lsDueBefore, "due-before", "", "Filter to motes with due_at strictly before this time (accepts the same formats as --due)")
	lsCmd.Flags().StringVar(&lsDueAfter, "due-after", "", "Filter to motes with due_at strictly after this time (accepts the same formats as --due)")
	lsCmd.Flags().StringArrayVar(&lsMetadataField, "metadata-field", nil, "Filter by frontmatter key=value (repeatable; ANDs with other --metadata-field and --has-metadata-key flags)")
	lsCmd.Flags().StringArrayVar(&lsHasMetadataKey, "has-metadata-key", nil, "Filter to motes that have this frontmatter key present (repeatable; ANDs with --metadata-field)")
	rootCmd.AddCommand(lsCmd)
}

func runLs(cmd *cobra.Command, args []string) error {
	// STORY-EXPLAIN-001 Scenario 6: --explain is scoped to ready-mote
	// justification and has no meaning for other filters. Hard-error before
	// any store I/O so the user gets a clear "use this together" signal —
	// silently dropping the flag would hide intent and ship the wrong report.
	if lsExplain && !lsReady {
		return &exitCodeError{code: 2, err: errors.New("--explain requires --ready")}
	}

	// Parse + validate metadata flags first so a malformed query is rejected
	// before any store I/O. (STORY-MQRY-001 security boundary: the filter
	// operates on already-loaded motes, but we still reject bad input early
	// to keep error messages stable.)
	metaFields, hasKeys, err := resolveMetadataFlags(lsMetadataField, lsHasMetadataKey)
	if err != nil {
		return err
	}

	filters := core.ListFilters{
		Type:            lsType,
		Tag:             lsTag,
		Status:          lsStatus,
		Stale:           lsStale,
		Ready:           lsReady,
		Parent:          lsParent,
		Overdue:         lsOverdue,
		IncludeDeferred: lsIncludeDeferred,
		MetadataFields:  metaFields,
		HasMetadataKeys: hasKeys,
	}

	// Parse --due-before / --due-after eagerly so a bad time spec is
	// rejected before we touch the store. mustFindRoot in doLs() will
	// resolve the workspace; for parser-time `now` we use the wall clock
	// directly (test code can override via t.Setenv etc. if needed).
	if lsDueBefore != "" {
		t, perr := core.ParseTimeSpec(lsDueBefore, time.Now())
		if perr != nil {
			return perr
		}
		filters.DueBefore = &t
	}
	if lsDueAfter != "" {
		t, perr := core.ParseTimeSpec(lsDueAfter, time.Now())
		if perr != nil {
			return perr
		}
		filters.DueAfter = &t
	}

	return doLs(filters, false, lsCompact, lsJSON, lsExplain)
}

func doLs(filters core.ListFilters, sortByWeight bool, compact bool, jsonOutput bool, explainMode bool) error {
	mode, err := outputMode(jsonOutput)
	if err != nil {
		return err
	}

	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	motes, err := mm.List(filters)
	if err != nil {
		return err
	}

	if len(motes) == 0 {
		switch mode {
		case ModeJSON:
			// Sprint-2 §23.16: empty must be `[]`, never `null` — agents poll
			// on the presence of `motes` and shape-check on the array type.
			// Legacy mode keeps the byte-exact compact form `{"motes":[]}` so
			// callers that grep on the literal stay green. Envelope mode goes
			// through the standard wrap helper.
			if jsonenv.Mode() == jsonenv.ModeEnvelope {
				return emitLsJSON(LsOutput{Motes: []LsMoteEntry{}})
			}
			jsonenv.EmitDeprecationNotice(os.Stderr)
			fmt.Println(`{"motes":[]}`)
			return nil
		case ModePlain:
			// Plain on empty list is silent — agents looping on `wc -l` see 0.
			return nil
		default:
			fmt.Println("No motes found.")
			return nil
		}
	}

	if sortByWeight {
		sort.Slice(motes, func(i, j int) bool {
			return motes[i].Weight > motes[j].Weight
		})
	}

	// STORY-EXPLAIN-001: build per-mote justification once, in parallel with
	// the filtered ready set, so every output mode renders from the same
	// source. We load the full graph (not just the ready set) because
	// blockers and parent epics will typically be in non-active states that
	// the readiness filter excludes.
	var explanations []*core.ReadyExplanation
	if explainMode {
		all, gerr := mm.ReadAllWithGlobal()
		if gerr != nil {
			return gerr
		}
		now := time.Now()
		explanations = make([]*core.ReadyExplanation, len(motes))
		for i, m := range motes {
			explanations[i] = core.BuildReadyExplanation(m, all, now)
		}
	}

	if mode == ModeJSON {
		entries := make([]LsMoteEntry, len(motes))
		for i, m := range motes {
			entry := LsMoteEntry{
				ID:     m.ID,
				Type:   m.Type,
				Status: m.Status,
				Weight: m.Weight,
				Title:  m.Title,
			}
			if explainMode {
				entry.ReadyExplanation = explanations[i]
			}
			entries[i] = entry
		}
		return emitLsJSON(LsOutput{Motes: entries})
	}

	if mode == ModePlain {
		if !explainMode {
			return emitLsPlain(motes)
		}
		// Interleave each row with its explanation block so a downstream
		// `grep -A 3 <mote-id>` pulls the right block (rather than all
		// rows followed by all blocks).
		for i, m := range motes {
			title := m.Title
			if m.Status == "deprecated" {
				title = "[deprecated] " + title
			}
			fmt.Printf("%s %s %s %.2f %s\n", m.ID, m.Type, m.Status, m.Weight, title)
			writeExplainLines(os.Stdout, explanations[i], false)
		}
		return nil
	}

	if compact {
		for i, m := range motes {
			fmt.Printf("%s: %s\n", m.ID, m.Title)
			if explainMode {
				writeExplainLines(os.Stdout, explanations[i], false)
			}
		}
		return nil
	}

	// STORY-HDRZ-001: each row is now a two-zone header (icon + ID + title
	// on the left, [icon STATUS w<weight>] on the right, right-aligned to
	// terminal width). The previous table column header (ID/TYPE/STATUS/
	// WEIGHT/TITLE) and the `---` divider are dropped — the rows are now
	// self-describing and consistent with `mote show`'s first line.
	useColor := useColorOutput()
	width := format.TerminalWidth()
	ascii := format.IconASCIIFromEnv()
	for i, m := range motes {
		// Preserve the historical `[deprecated] ` prefix on the title for
		// existing log scrapers (UI_PHILOSOPHY "Backward compatibility for
		// textual markers"). The mute styling is applied inside RenderHeader.
		title := m.Title
		if m.Status == "deprecated" {
			title = "[deprecated] " + title
		}
		fmt.Println(format.RenderHeader(format.HeaderInput{
			ID:     m.ID,
			Status: m.Status,
			Title:  title,
			Weight: m.Weight,
		}, width, format.HeaderPretty, ascii, useColor))
		if explainMode {
			writeExplainLines(os.Stdout, explanations[i], useColor)
		}
	}
	return nil
}

// emitLsPlain writes one mote per line in colorless, line-oriented form for
// STORY-PLAIN-001. Each line is `<id> <type> <status> <weight> <title>` with
// single-space separators. Deprecated rows keep the textual `[deprecated]`
// prefix on the title (sprint-2 UI-PHIL backward-compat marker), positioned
// AFTER the id so the mote id remains the first whitespace-delimited token —
// Scenario 6 requires `awk '{print $1}'` to extract the id reliably.
func emitLsPlain(motes []*core.Mote) error {
	for _, m := range motes {
		title := m.Title
		if m.Status == "deprecated" {
			title = "[deprecated] " + title
		}
		fmt.Printf("%s %s %s %.2f %s\n", m.ID, m.Type, m.Status, m.Weight, title)
	}
	return nil
}

// writeExplainLines emits the three justification lines (`ready because:`,
// `parent epic:`, `freshness:`) under the current mote row. Shared by plain,
// compact, and pretty/auto modes — the only difference is whether ANSI
// muting is applied (useColor=true in pretty mode when stdout is a TTY).
//
// Output shape per STORY-EXPLAIN-001 §2:
//
//	ready because: no blockers
//	parent epic: epic-foo (in_progress)
//	freshness: 2d (fresh)
//
// Closed-parent rendering uses Scenario 8's "CLOSED — completed" highlight.
// Never-accessed motes render `freshness: never accessed`.
func writeExplainLines(w *os.File, exp *core.ReadyExplanation, useColor bool) {
	if exp == nil {
		return
	}
	lines := []string{
		"  ready because: " + exp.Reason,
	}
	if exp.Parent != nil {
		if exp.Parent.IsClosed {
			lines = append(lines, fmt.Sprintf("  parent epic: %s (CLOSED — %s)", exp.Parent.ID, exp.Parent.Status))
		} else {
			lines = append(lines, fmt.Sprintf("  parent epic: %s (%s)", exp.Parent.ID, exp.Parent.Status))
		}
	}
	lines = append(lines, "  freshness: "+freshnessLine(exp.Freshness))

	for _, line := range lines {
		if useColor {
			line = format.Muted(line, true)
		}
		_, _ = fmt.Fprintln(w, line)
	}
}

// freshnessLine renders a FreshnessRef for human display. Three shapes:
//
//	never accessed            — when NeverAccessed=true
//	Nd (stale — not touched in 14d)  — when Stale=true and recently-but-too-long accessed
//	Nd (fresh)                — when neither
func freshnessLine(f *core.FreshnessRef) string {
	if f == nil {
		return "unknown"
	}
	if f.NeverAccessed {
		return "never accessed"
	}
	rel := format.RelativeTime(time.Duration(f.SecondsSinceLastAccess) * time.Second)
	if f.Stale {
		return fmt.Sprintf("%s (stale — not touched in %s)", rel, format.RelativeTime(core.DefaultFreshnessThreshold))
	}
	return rel + " (fresh)"
}

// emitLsJSON serializes an LsOutput in either envelope or legacy mode.
// Centralised here so the empty-list shortcut and the populated path share the
// same envelope branch. Used by `mote ls --json` and `mote pulse --json`
// (the latter routes through doLs).
func emitLsJSON(out LsOutput) error {
	var (
		data []byte
		err  error
	)
	if jsonenv.Mode() == jsonenv.ModeEnvelope {
		data, err = json.MarshalIndent(jsonenv.Wrap(out), "", "  ")
	} else {
		jsonenv.EmitDeprecationNotice(os.Stderr)
		data, err = json.MarshalIndent(out, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
