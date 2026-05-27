// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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

// LsMoteEntry represents a mote in ls JSON output.
type LsMoteEntry struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Status string  `json:"status"`
	Weight float64 `json:"weight"`
	Title  string  `json:"title"`
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

	lsCmd.Flags().BoolVar(&lsOverdue, "overdue", false, "Show active/in_progress motes whose due_at has passed, sorted by due_at ascending")
	lsCmd.Flags().BoolVar(&lsIncludeDeferred, "include-deferred", false, "When combined with --ready, do not hide motes whose defer_until is still in the future")
	lsCmd.Flags().StringVar(&lsDueBefore, "due-before", "", "Filter to motes with due_at strictly before this time (accepts the same formats as --due)")
	lsCmd.Flags().StringVar(&lsDueAfter, "due-after", "", "Filter to motes with due_at strictly after this time (accepts the same formats as --due)")
	lsCmd.Flags().StringArrayVar(&lsMetadataField, "metadata-field", nil, "Filter by frontmatter key=value (repeatable; ANDs with other --metadata-field and --has-metadata-key flags)")
	lsCmd.Flags().StringArrayVar(&lsHasMetadataKey, "has-metadata-key", nil, "Filter to motes that have this frontmatter key present (repeatable; ANDs with --metadata-field)")
	rootCmd.AddCommand(lsCmd)
}

func runLs(cmd *cobra.Command, args []string) error {
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

	return doLs(filters, false, lsCompact, lsJSON)
}

func doLs(filters core.ListFilters, sortByWeight bool, compact bool, jsonOutput bool) error {
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

	if mode == ModeJSON {
		entries := make([]LsMoteEntry, len(motes))
		for i, m := range motes {
			entries[i] = LsMoteEntry{
				ID:     m.ID,
				Type:   m.Type,
				Status: m.Status,
				Weight: m.Weight,
				Title:  m.Title,
			}
		}
		return emitLsJSON(LsOutput{Motes: entries})
	}

	if mode == ModePlain {
		return emitLsPlain(motes)
	}

	if compact {
		for _, m := range motes {
			fmt.Printf("%s: %s\n", m.ID, m.Title)
		}
		return nil
	}

	fmt.Printf("%-24s  %-14s  %-12s  %-8s  %s\n",
		"ID", "TYPE", "STATUS", "WEIGHT", "TITLE")
	fmt.Println(strings.Repeat("-", 80))

	useColor := useColorOutput()
	for _, m := range motes {
		title := format.Truncate(m.Title, 40)
		if m.Status == "deprecated" {
			title = "[deprecated] " + title
		}
		// Pad/format the raw row first, then wrap in ANSI so column edges align.
		row := fmt.Sprintf("%-24s  %-14s  %-12s  %-8.2f  %s",
			m.ID, m.Type, m.Status, m.Weight, title)
		if format.IsClosed(m.Status) {
			row = format.Muted(row, useColor)
		}
		fmt.Println(row)
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
