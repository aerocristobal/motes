// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
)

// ShowOutput is the JSON output structure for mote show --json.
type ShowOutput struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Status        string             `json:"status"`
	Title         string             `json:"title"`
	Tags          []string           `json:"tags"`
	Weight        float64            `json:"weight"`
	Origin        string             `json:"origin"`
	Size          string             `json:"size,omitempty"`
	Parent        string             `json:"parent,omitempty"`
	CreatedAt     string             `json:"created_at"`
	LastAccessed  string             `json:"last_accessed,omitempty"`
	AccessCount   int                `json:"access_count"`
	ExternalRefs  []core.ExternalRef `json:"external_refs,omitempty"`
	DependsOn     []string           `json:"depends_on,omitempty"`
	Blocks        []string           `json:"blocks,omitempty"`
	RelatesTo     []string           `json:"relates_to,omitempty"`
	BuildsOn      []string           `json:"builds_on,omitempty"`
	Contradicts   []string           `json:"contradicts,omitempty"`
	Supersedes    []string           `json:"supersedes,omitempty"`
	CausedBy      []string           `json:"caused_by,omitempty"`
	InformedBy    []string           `json:"informed_by,omitempty"`
	Acceptance    []string           `json:"acceptance,omitempty"`
	AcceptanceMet []bool             `json:"acceptance_met,omitempty"`
	Action        string             `json:"action,omitempty"`
	Body          string             `json:"body"`
	BodyLinks     []BodyLinkEntry    `json:"body_links,omitempty"`
	Concepts      []ConceptEntry     `json:"concepts,omitempty"`
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

var showJSON bool

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Display a mote's content and links",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "Output in JSON format")
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
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	m, err := mm.Read(args[0])
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "mote not found: %s\n", args[0])
			os.Exit(1)
		}
		return err
	}

	// Load index for concept display
	im := core.NewIndexManager(root)
	idx, _ := im.Load()

	if showJSON {
		out := ShowOutput{
			ID:            m.ID,
			Type:          m.Type,
			Status:        m.Status,
			Title:         m.Title,
			Tags:          m.Tags,
			Weight:        m.Weight,
			Origin:        m.Origin,
			Size:          m.Size,
			Parent:        m.Parent,
			CreatedAt:     m.CreatedAt.Format(time.RFC3339),
			AccessCount:   m.AccessCount,
			ExternalRefs:  m.ExternalRefs,
			DependsOn:     m.DependsOn,
			Blocks:        m.Blocks,
			RelatesTo:     m.RelatesTo,
			BuildsOn:      m.BuildsOn,
			Contradicts:   m.Contradicts,
			Supersedes:    m.Supersedes,
			CausedBy:      m.CausedBy,
			InformedBy:    m.InformedBy,
			Acceptance:    m.Acceptance,
			AcceptanceMet: m.AcceptanceMet,
			Action:        m.Action,
			Body:          m.Body,
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
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		_ = mm.AppendAccessBatch(m.ID)
		return nil
	}

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

	if hasAnyLinks(m) {
		fmt.Println("\n--- links ---")
		printLinks(mm, "depends_on", m.DependsOn)
		printLinks(mm, "blocks", m.Blocks)
		printLinks(mm, "relates_to", m.RelatesTo)
		printLinks(mm, "builds_on", m.BuildsOn)
		printLinks(mm, "contradicts", m.Contradicts)
		printLinks(mm, "supersedes", m.Supersedes)
		printLinks(mm, "caused_by", m.CausedBy)
		printLinks(mm, "informed_by", m.InformedBy)
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

	_ = mm.AppendAccessBatch(m.ID)
	return nil
}

func hasAnyLinks(m *core.Mote) bool {
	return len(m.DependsOn)+len(m.Blocks)+len(m.RelatesTo)+
		len(m.BuildsOn)+len(m.Contradicts)+len(m.Supersedes)+
		len(m.CausedBy)+len(m.InformedBy) > 0
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
