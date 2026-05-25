// SPDX-License-Identifier: MIT
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
)

// ShowOutput is the JSON output structure for mote show --json.
type ShowOutput struct {
	ID             string             `json:"id"`
	Type           string             `json:"type"`
	Status         string             `json:"status"`
	Title          string             `json:"title"`
	Tags           []string           `json:"tags"`
	Weight         float64            `json:"weight"`
	Origin         string             `json:"origin"`
	Size           string             `json:"size,omitempty"`
	Parent         string             `json:"parent,omitempty"`
	CreatedAt      string             `json:"created_at"`
	LastAccessed   string             `json:"last_accessed,omitempty"`
	AccessCount    int                `json:"access_count"`
	ExternalRefs   []core.ExternalRef `json:"external_refs,omitempty"`
	DependsOn      []string           `json:"depends_on,omitempty"`
	Blocks         []string           `json:"blocks,omitempty"`
	RelatesTo      []string           `json:"relates_to,omitempty"`
	BuildsOn       []string           `json:"builds_on,omitempty"`
	Contradicts    []string           `json:"contradicts,omitempty"`
	Supersedes     []string           `json:"supersedes,omitempty"`
	CausedBy       []string           `json:"caused_by,omitempty"`
	InformedBy     []string           `json:"informed_by,omitempty"`
	DiscoveredFrom []string           `json:"discovered_from,omitempty"`
	Discovered     []string           `json:"discovered,omitempty"`
	Acceptance     []string           `json:"acceptance,omitempty"`
	AcceptanceMet  []bool             `json:"acceptance_met,omitempty"`
	Action         string             `json:"action,omitempty"`
	Body           string             `json:"body"`
	BodyLinks      []BodyLinkEntry    `json:"body_links,omitempty"`
	Concepts       []ConceptEntry     `json:"concepts,omitempty"`
}

// ShowShortOutput is the JSON output structure for `mote show --short --json`.
// The key set is intentionally minimal so loop consumers can rely on a tight,
// stable shape; see STORY-SHOW-001 Scenario 6.
type ShowShortOutput struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Type   string  `json:"type"`
	Weight float64 `json:"weight"`
	Title  string  `json:"title"`
}

// ShowLongOutput is the JSON output structure for `mote show --long --json`.
// It embeds ShowOutput so every default-mode key is promoted to the top level
// (Scenario 7 superset guarantee) and adds forensic internal-state fields.
type ShowLongOutput struct {
	ShowOutput
	LastPrimeAt          string   `json:"last_prime_at,omitempty"`
	AuditLogPath         string   `json:"audit_log_path"`
	AuditLogEntriesCount int      `json:"audit_log_entries_count"`
	PromotedTo           string   `json:"promoted_to,omitempty"`
	StrataCorpus         string   `json:"strata_corpus,omitempty"`
	StrataQueryHint      string   `json:"strata_query_hint,omitempty"`
	StrataQueryCount     int      `json:"strata_query_count,omitempty"`
	StrataLastQueried    string   `json:"strata_last_queried,omitempty"`
	DeprecatedBy         string   `json:"deprecated_by,omitempty"`
	StatusChangedAt      string   `json:"status_changed_at,omitempty"`
	DeprecationChain     []string `json:"deprecation_chain,omitempty"`
}

// BodyLinkEntry represents a resolved wiki-link target.
type BodyLinkEntry struct {
	ID    string `json:"id"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
}

// ConceptEntry represents a concept term associated with a mote.
type ConceptEntry struct {
	Term        string  `json:"term"`
	Frequency   int     `json:"frequency"`
	IDF         float64 `json:"idf"`
	Distinctive bool    `json:"distinctive"`
}

var (
	showJSON  bool
	showShort bool
	showLong  bool
	showASCII bool
)

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Display a mote's content and links",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "Output in JSON format")
	showCmd.Flags().BoolVar(&showShort, "short", false, "One-line dense output for loop iteration")
	showCmd.Flags().BoolVar(&showLong, "long", false, "Verbose forensic output with internal-state section")
	showCmd.Flags().BoolVar(&showASCII, "ascii", false, "Use ASCII status icons (also honors NO_UNICODE)")
	// Silence cobra's default error/usage rendering so main()'s handling of
	// *exitCodeError is the sole source of stderr output on failure — preserves
	// byte-stable stderr text for scripts and tests.
	showCmd.SilenceErrors = true
	showCmd.SilenceUsage = true
	rootCmd.AddCommand(showCmd)
}

func getConceptEntries(moteID string, idx *core.EdgeIndex) []ConceptEntry {
	if idx == nil {
		return nil
	}
	edges := idx.Neighbors(moteID, map[string]bool{"concept_ref": true})
	if len(edges) == 0 {
		return nil
	}
	var entries []ConceptEntry
	for _, e := range edges {
		term := e.Target
		freq := idx.ConceptStats[term]
		idf := 0.0
		if freq > 0 {
			idf = 1.0 / math.Log2(float64(freq)+2)
		}
		entries = append(entries, ConceptEntry{
			Term:        term,
			Frequency:   freq,
			IDF:         idf,
			Distinctive: idf > 0.6,
		})
	}
	return entries
}

func runShow(cmd *cobra.Command, args []string) error {
	// Mutex check runs BEFORE any side effect (no mote read, no access-batch
	// append) — Scenario 5 requires the rejection to be a pure validation error.
	if showShort && showLong {
		return &exitCodeError{code: 1, err: fmt.Errorf("--short and --long are mutually exclusive")}
	}

	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	m, err := mm.Read(args[0])
	if err != nil {
		if os.IsNotExist(err) {
			return &exitCodeError{code: 1, err: fmt.Errorf("mote not found: %s", args[0])}
		}
		return err
	}

	im := core.NewIndexManager(root)
	idx, _ := im.Load()

	if showShort {
		ascii := showASCII || format.IconASCIIFromEnv()
		if showJSON {
			out := ShowShortOutput{
				ID:     m.ID,
				Status: m.Status,
				Type:   m.Type,
				Weight: m.Weight,
				Title:  m.Title,
			}
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal json: %w", err)
			}
			fmt.Println(string(data))
			return nil
		}
		fmt.Print(renderShort(m, ascii))
		// Suppress AppendAccessBatch so loop iteration over many ready motes
		// does not inflate access_count and skew weight decay (Scenario 9, Q3).
		return nil
	}

	if showJSON {
		base := buildShowOutput(m, mm, idx)
		var data []byte
		if showLong {
			long := buildShowLongOutput(base, m, root)
			data, err = json.MarshalIndent(long, "", "  ")
		} else {
			data, err = json.MarshalIndent(base, "", "  ")
		}
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		_ = mm.AppendAccessBatch(m.ID)
		return nil
	}

	emitDefaultText(m, mm, idx)
	if showLong {
		emitLongInternalState(m, mm, root)
	}
	_ = mm.AppendAccessBatch(m.ID)
	return nil
}

// renderShort produces the single-line dense form:
//
//	<icon> <id> <weight:.2f> [<type>] <title-truncated-to-60>\n
//
// The title is the LAST field so partial visual truncation by the terminal
// never loses the icon, ID, weight, or type.
func renderShort(m *core.Mote, ascii bool) string {
	icon := format.StatusIcon(m.Status, ascii)
	title := format.Truncate(m.Title, 60)
	return fmt.Sprintf("%s %s %.2f [%s] %s\n", icon, m.ID, m.Weight, m.Type, title)
}

// buildShowOutput constructs the default JSON struct. Extracted from the
// inline JSON branch so --long --json can reuse it as the embedded base
// (Scenario 7 superset guarantee).
func buildShowOutput(m *core.Mote, mm *core.MoteManager, idx *core.EdgeIndex) ShowOutput {
	out := ShowOutput{
		ID:             m.ID,
		Type:           m.Type,
		Status:         m.Status,
		Title:          m.Title,
		Tags:           m.Tags,
		Weight:         m.Weight,
		Origin:         m.Origin,
		Size:           m.Size,
		Parent:         m.Parent,
		CreatedAt:      m.CreatedAt.Format(time.RFC3339),
		AccessCount:    m.AccessCount,
		ExternalRefs:   m.ExternalRefs,
		DependsOn:      m.DependsOn,
		Blocks:         m.Blocks,
		RelatesTo:      m.RelatesTo,
		BuildsOn:       m.BuildsOn,
		Contradicts:    m.Contradicts,
		Supersedes:     m.Supersedes,
		CausedBy:       m.CausedBy,
		InformedBy:     m.InformedBy,
		DiscoveredFrom: m.DiscoveredFrom,
		Discovered:     discoveredChildren(idx, m.ID),
		Acceptance:     m.Acceptance,
		AcceptanceMet:  m.AcceptanceMet,
		Action:         m.Action,
		Body:           m.Body,
	}
	if m.LastAccessed != nil {
		out.LastAccessed = m.LastAccessed.Format(time.RFC3339)
	}
	bodyLinkIDs := core.ExtractBodyLinks(m.Body, m.ID)
	for _, blID := range bodyLinkIDs {
		entry := BodyLinkEntry{ID: blID}
		if linked, err := mm.Read(blID); err == nil {
			entry.Type = linked.Type
			entry.Title = linked.Title
		}
		out.BodyLinks = append(out.BodyLinks, entry)
	}
	out.Concepts = getConceptEntries(m.ID, idx)
	return out
}

// buildShowLongOutput wraps a default ShowOutput with forensic extension fields.
func buildShowLongOutput(base ShowOutput, m *core.Mote, root string) ShowLongOutput {
	long := ShowLongOutput{
		ShowOutput:           base,
		AuditLogPath:         filepath.Join(root, "audit.jsonl"),
		AuditLogEntriesCount: countAuditEntries(root, m.ID),
		PromotedTo:           m.PromotedTo,
		StrataCorpus:         m.StrataCorpus,
		StrataQueryHint:      m.StrataQueryHint,
		StrataQueryCount:     m.StrataQueryCount,
		DeprecatedBy:         m.DeprecatedBy,
	}
	if t := lastPrimeAt(root); t != "" {
		long.LastPrimeAt = t
	}
	if m.StrataLastQueried != nil {
		long.StrataLastQueried = m.StrataLastQueried.Format(time.RFC3339)
	}
	if m.StatusChangedAt != nil {
		long.StatusChangedAt = m.StatusChangedAt.Format(time.RFC3339)
	}
	if m.DeprecatedBy != "" {
		long.DeprecationChain = walkDeprecationChain(core.NewMoteManager(root), m)
	}
	return long
}

// emitDefaultText writes the existing default-mode rich text output to stdout.
// Extracted so --long can call it first and then append the internal-state
// section, preserving Scenario 3's "default output is present unchanged".
func emitDefaultText(m *core.Mote, mm *core.MoteManager, idx *core.EdgeIndex) {
	fmt.Println(format.Header(m.ID))
	fmt.Println(format.Field("type", m.Type))
	fmt.Println(format.Field("status", m.Status))
	fmt.Println(format.Field("title", m.Title))
	fmt.Println(format.Field("tags", format.TagList(m.Tags)))
	fmt.Println(format.Field("weight", fmt.Sprintf("%.2f", m.Weight)))
	fmt.Println(format.Field("origin", m.Origin))
	if m.Size != "" {
		fmt.Println(format.Field("size", m.Size))
	}
	if m.Action != "" {
		fmt.Println(format.Field("action", m.Action))
	}
	if m.Parent != "" {
		parentTitle := m.Parent
		if p, err := mm.Read(m.Parent); err == nil {
			parentTitle = m.Parent + " (" + p.Title + ")"
		}
		fmt.Println(format.Field("parent", parentTitle))
	}
	fmt.Println(format.Field("created_at", m.CreatedAt.Format(time.RFC3339)))
	if m.LastAccessed != nil {
		fmt.Println(format.Field("last_accessed", m.LastAccessed.Format(time.RFC3339)))
	} else {
		fmt.Println(format.Field("last_accessed", "(never)"))
	}
	fmt.Println(format.Field("access_count", fmt.Sprintf("%d", m.AccessCount)))

	if len(m.ExternalRefs) > 0 {
		fmt.Println("\n--- external refs ---")
		for _, ref := range m.ExternalRefs {
			if ref.URL != "" {
				fmt.Println(format.Field(ref.Provider, ref.ID+" "+ref.URL))
			} else {
				fmt.Println(format.Field(ref.Provider, ref.ID))
			}
		}
	}

	discoveredKids := discoveredChildren(idx, m.ID)
	if hasAnyLinks(m) || len(discoveredKids) > 0 {
		fmt.Println("\n--- links ---")
		printLinks(mm, "depends_on", m.DependsOn)
		printLinks(mm, "blocks", m.Blocks)
		printLinks(mm, "relates_to", m.RelatesTo)
		printLinks(mm, "builds_on", m.BuildsOn)
		printLinks(mm, "contradicts", m.Contradicts)
		printLinks(mm, "supersedes", m.Supersedes)
		printLinks(mm, "caused_by", m.CausedBy)
		printLinks(mm, "informed_by", m.InformedBy)
		printLinks(mm, "discovered_from", m.DiscoveredFrom)
		printLinks(mm, "discovered", discoveredKids)
	}

	bodyLinkIDs := core.ExtractBodyLinks(m.Body, m.ID)
	if len(bodyLinkIDs) > 0 {
		fmt.Println("\n--- body links ---")
		for _, blID := range bodyLinkIDs {
			if linked, err := mm.Read(blID); err == nil {
				fmt.Printf("  -> %s (%s) %s\n", blID, linked.Type, linked.Title)
			} else {
				fmt.Printf("  -> %s (unresolved)\n", blID)
			}
		}
	}

	concepts := getConceptEntries(m.ID, idx)
	if len(concepts) > 0 {
		fmt.Println("\n--- concepts ---")
		for _, c := range concepts {
			distinctive := ""
			if c.Distinctive {
				distinctive = "  *distinctive*"
			}
			fmt.Printf("  [[%s]]  freq=%d  idf=%.2f%s\n", c.Term, c.Frequency, c.IDF, distinctive)
		}
	}

	children, _ := mm.Children(m.ID)
	if len(children) > 0 {
		fmt.Println("\n--- children ---")
		for _, c := range children {
			marker := "[ ]"
			if c.Status == "completed" {
				marker = "[x]"
			}
			fmt.Printf("  %s %s %q [%s]\n", marker, c.ID, c.Title, c.Status)
		}
	}

	if len(m.Acceptance) > 0 {
		fmt.Println("\n--- acceptance ---")
		met := 0
		for i, a := range m.Acceptance {
			check := "[ ]"
			if i < len(m.AcceptanceMet) && m.AcceptanceMet[i] {
				check = "[x]"
				met++
			}
			fmt.Printf("  %s %d. %s\n", check, i+1, a)
		}
		fmt.Printf("  Progress: %d/%d\n", met, len(m.Acceptance))
	}

	if m.Body != "" {
		fmt.Println("\n--- body ---")
		fmt.Print(m.Body)
		if m.Body[len(m.Body)-1] != '\n' {
			fmt.Println()
		}
	}
}

// emitLongInternalState appends the forensic "--- internal state ---" section
// (Scenario 3). Fields with no value are still emitted with "(none)" so the
// section's shape is stable and easy to diff.
func emitLongInternalState(m *core.Mote, mm *core.MoteManager, root string) {
	fmt.Println("\n--- internal state ---")
	if t := lastPrimeAt(root); t != "" {
		fmt.Println(format.Field("last_prime_at", t))
	} else {
		fmt.Println(format.Field("last_prime_at", "(never)"))
	}
	fmt.Println(format.Field("audit_log_path", filepath.Join(root, "audit.jsonl")))
	fmt.Println(format.Field("audit_log_entries", fmt.Sprintf("%d", countAuditEntries(root, m.ID))))
	fmt.Println(format.Field("promoted_to", orNone(m.PromotedTo)))
	fmt.Println(format.Field("strata_corpus", orNone(m.StrataCorpus)))
	if m.StrataCorpus != "" {
		fmt.Println(format.Field("strata_query_hint", orNone(m.StrataQueryHint)))
		fmt.Println(format.Field("strata_query_count", fmt.Sprintf("%d", m.StrataQueryCount)))
		if m.StrataLastQueried != nil {
			fmt.Println(format.Field("strata_last_queried", m.StrataLastQueried.Format(time.RFC3339)))
		} else {
			fmt.Println(format.Field("strata_last_queried", "(never)"))
		}
	}
	fmt.Println(format.Field("deprecated_by", orNone(m.DeprecatedBy)))
	if m.StatusChangedAt != nil {
		fmt.Println(format.Field("status_changed_at", m.StatusChangedAt.Format(time.RFC3339)))
	} else {
		fmt.Println(format.Field("status_changed_at", "(never)"))
	}
	if m.DeprecatedBy != "" {
		chain := walkDeprecationChain(mm, m)
		if len(chain) > 0 {
			fmt.Println(format.Field("deprecation_chain", renderChain(chain)))
		}
	}
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func renderChain(ids []string) string {
	out := ""
	for i, id := range ids {
		if i == 0 {
			out = "-> " + id
		} else {
			out += " -> " + id
		}
	}
	return out
}

// lastPrimeAt returns the RFC3339-formatted mtime of .memory/last_prime.txt
// (the file `mote prime` writes on every successful run), or "" if absent.
func lastPrimeAt(root string) string {
	info, err := os.Stat(filepath.Join(root, lastPrimeFilename))
	if err != nil {
		return ""
	}
	return info.ModTime().Format(time.RFC3339)
}

// countAuditEntries scans .memory/audit.jsonl line-by-line and counts entries
// matching moteID. Missing file → 0; malformed lines are skipped.
func countAuditEntries(root, moteID string) int {
	f, err := os.Open(filepath.Join(root, "audit.jsonl"))
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	// Allow long audit lines (default 64K buffer is fine for normal use,
	// but bump it to 1MB for safety on systems with very long FieldsSet lists).
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	count := 0
	for sc.Scan() {
		var e core.AuditEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if e.MoteID == moteID {
			count++
		}
	}
	return count
}

// walkDeprecationChain follows DeprecatedBy transitively, cycle-safe via a
// visited set and capped at 32 hops to bound worst-case I/O on pathological
// graphs.
func walkDeprecationChain(mm *core.MoteManager, m *core.Mote) []string {
	const maxHops = 32
	var chain []string
	visited := map[string]bool{m.ID: true}
	current := m
	for i := 0; i < maxHops && current.DeprecatedBy != ""; i++ {
		next := current.DeprecatedBy
		if visited[next] {
			break
		}
		visited[next] = true
		chain = append(chain, next)
		nm, err := mm.Read(next)
		if err != nil {
			break
		}
		current = nm
	}
	return chain
}

func hasAnyLinks(m *core.Mote) bool {
	return len(m.DependsOn)+len(m.Blocks)+len(m.RelatesTo)+
		len(m.BuildsOn)+len(m.Contradicts)+len(m.Supersedes)+
		len(m.CausedBy)+len(m.InformedBy)+len(m.DiscoveredFrom) > 0
}

// discoveredChildren returns mote IDs whose discovered_from points to parentID,
// read from the index's reverse discovered_ref edges.
func discoveredChildren(idx *core.EdgeIndex, parentID string) []string {
	if idx == nil {
		return nil
	}
	edges := idx.Neighbors(parentID, map[string]bool{"discovered_ref": true})
	if len(edges) == 0 {
		return nil
	}
	ids := make([]string, 0, len(edges))
	for _, e := range edges {
		ids = append(ids, e.Target)
	}
	return ids
}

func printLinks(mm *core.MoteManager, label string, ids []string) {
	for _, id := range ids {
		linked, err := mm.Read(id)
		if err == nil {
			fmt.Println(format.Field(label, id+" ("+linked.Title+")"))
		} else {
			fmt.Println(format.Field(label, id))
		}
	}
}
