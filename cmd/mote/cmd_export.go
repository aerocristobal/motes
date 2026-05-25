// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export motes and memories as a JSON envelope",
	Long: `Emit a single JSON object with top-level "motes" and "memories"
arrays. NOTE: This is a breaking change from the prior JSONL output —
consumers that line-split must switch to whole-document JSON parsing.`,
	RunE: runExport,
}

var (
	exportType   string
	exportTag    string
	exportStatus string
	exportOutput string
)

// ExportMote is the JSON representation of a mote for export.
type ExportMote struct {
	ID             string             `json:"id"`
	Type           string             `json:"type"`
	Status         string             `json:"status"`
	Title          string             `json:"title"`
	Tags           []string           `json:"tags"`
	Weight         float64            `json:"weight"`
	Origin         string             `json:"origin"`
	CreatedAt      time.Time          `json:"created_at"`
	Body           string             `json:"body"`
	DependsOn      []string           `json:"depends_on,omitempty"`
	Blocks         []string           `json:"blocks,omitempty"`
	RelatesTo      []string           `json:"relates_to,omitempty"`
	BuildsOn       []string           `json:"builds_on,omitempty"`
	Contradicts    []string           `json:"contradicts,omitempty"`
	Supersedes     []string           `json:"supersedes,omitempty"`
	CausedBy       []string           `json:"caused_by,omitempty"`
	InformedBy     []string           `json:"informed_by,omitempty"`
	DiscoveredFrom []string           `json:"discovered_from,omitempty"`
	SourceIssue    string             `json:"source_issue,omitempty"`
	Parent         string             `json:"parent,omitempty"`
	Acceptance     []string           `json:"acceptance,omitempty"`
	AcceptanceMet  []bool             `json:"acceptance_met,omitempty"`
	Size           string             `json:"size,omitempty"`
	ExternalRefs   []core.ExternalRef `json:"external_refs,omitempty"`
}

// ExportMemory is the JSON representation of a memory for export.
type ExportMemory struct {
	Key       string    `json:"key"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExportEnvelope is the top-level shape of `mote export --json` and
// `mote export` output. Both arrays are always present (possibly empty)
// so consumers can parse without nil-checks.
type ExportEnvelope struct {
	Motes    []ExportMote   `json:"motes"`
	Memories []ExportMemory `json:"memories"`
}

func init() {
	exportCmd.Flags().StringVar(&exportType, "type", "", "Filter by mote type")
	exportCmd.Flags().StringVar(&exportTag, "tag", "", "Filter by tag")
	exportCmd.Flags().StringVar(&exportStatus, "status", "", "Filter by status")
	exportCmd.Flags().StringVar(&exportOutput, "output", "", "Output file (default: stdout)")
	rootCmd.AddCommand(exportCmd)
}

func moteToExport(m *core.Mote) ExportMote {
	return ExportMote{
		ID:             m.ID,
		Type:           m.Type,
		Status:         m.Status,
		Title:          m.Title,
		Tags:           m.Tags,
		Weight:         m.Weight,
		Origin:         m.Origin,
		CreatedAt:      m.CreatedAt,
		Body:           m.Body,
		DependsOn:      m.DependsOn,
		Blocks:         m.Blocks,
		RelatesTo:      m.RelatesTo,
		BuildsOn:       m.BuildsOn,
		Contradicts:    m.Contradicts,
		Supersedes:     m.Supersedes,
		CausedBy:       m.CausedBy,
		InformedBy:     m.InformedBy,
		DiscoveredFrom: m.DiscoveredFrom,
		SourceIssue:    m.SourceIssue,
		Parent:         m.Parent,
		Acceptance:     m.Acceptance,
		AcceptanceMet:  m.AcceptanceMet,
		Size:           m.Size,
		ExternalRefs:   m.ExternalRefs,
	}
}

func runExport(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	motes, err := mm.List(core.ListFilters{
		Type:   exportType,
		Tag:    exportTag,
		Status: exportStatus,
	})
	if err != nil {
		return fmt.Errorf("list motes: %w", err)
	}

	envelope := ExportEnvelope{
		Motes:    make([]ExportMote, 0, len(motes)),
		Memories: []ExportMemory{},
	}
	for _, m := range motes {
		envelope.Motes = append(envelope.Motes, moteToExport(m))
	}

	// Memories ignore the --type / --tag / --status filters because they
	// have no type/tag/status — they're flat key/body pairs.
	store := core.NewMemoryStore(root)
	memories, memErr := store.List("")
	if memErr == nil {
		for _, mr := range memories {
			envelope.Memories = append(envelope.Memories, ExportMemory{
				Key:       mr.Key,
				Body:      mr.Body,
				CreatedAt: mr.CreatedAt,
				UpdatedAt: mr.UpdatedAt,
			})
		}
	}

	w := os.Stdout
	if exportOutput != "" {
		f, err := os.Create(exportOutput)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(envelope); err != nil {
		return fmt.Errorf("encode export envelope: %w", err)
	}

	if exportOutput != "" {
		fmt.Fprintf(os.Stderr, "Exported %d motes and %d memories to %s\n",
			len(envelope.Motes), len(envelope.Memories), exportOutput)
	}
	return nil
}
