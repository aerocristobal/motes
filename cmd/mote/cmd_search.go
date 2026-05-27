// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/strata"
)

// SearchOutput is the JSON output structure for mote search --json.
type SearchOutput struct {
	Query   string              `json:"query"`
	Results []SearchResultEntry `json:"results"`
}

// SearchResultEntry represents a search result in JSON output.
type SearchResultEntry struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

var searchTopK int
var searchJSON bool
var searchType string
var searchTag string
var searchStatus string
var searchExcludeStatus string
var searchMetadataField []string
var searchHasMetadataKey []string

var searchCmd = &cobra.Command{
	Use:   "search <query...>",
	Short: "Full-text search across all motes",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSearch,
}

func init() {
	searchCmd.Flags().IntVarP(&searchTopK, "top", "k", 10, "Number of results to return")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output in JSON format")
	searchCmd.Flags().StringVar(&searchType, "type", "", "Filter by mote type")
	searchCmd.Flags().StringVar(&searchTag, "tag", "", "Filter by tag")
	searchCmd.Flags().StringVar(&searchStatus, "status", "", "Filter by status")
	searchCmd.Flags().StringVar(&searchExcludeStatus, "exclude-status", "", "Exclude motes with this status")
	searchCmd.Flags().StringArrayVar(&searchMetadataField, "metadata-field", nil, "Filter by frontmatter key=value (repeatable; ANDs with text match)")
	searchCmd.Flags().StringArrayVar(&searchHasMetadataKey, "has-metadata-key", nil, "Filter to motes with this frontmatter key present (repeatable; ANDs with text match)")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	mode, err := outputMode(searchJSON)
	if err != nil {
		return err
	}

	query := strings.Join(args, " ")

	// Parse + validate metadata flags before any store I/O (STORY-MQRY-001).
	metaFields, hasKeys, mferr := resolveMetadataFlags(searchMetadataField, searchHasMetadataKey)
	if mferr != nil {
		return mferr
	}

	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	motes, err := readAllWithGlobal(mm)
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	if len(motes) == 0 {
		fmt.Println("No motes found.")
		return nil
	}

	// Pre-filter motes before building BM25 index
	hasFilter := searchType != "" || searchTag != "" || searchStatus != "" || searchExcludeStatus != "" || len(metaFields) > 0 || len(hasKeys) > 0
	if hasFilter {
		var filtered []*core.Mote
		for _, m := range motes {
			if searchType != "" && m.Type != searchType {
				continue
			}
			if searchTag != "" && !moteHasTag(m, searchTag) {
				continue
			}
			if searchStatus != "" && m.Status != searchStatus {
				continue
			}
			if searchExcludeStatus != "" && m.Status == searchExcludeStatus {
				continue
			}
			if !core.MotePassesMetadata(m, metaFields, hasKeys) {
				continue
			}
			filtered = append(filtered, m)
		}
		motes = filtered
		if len(motes) == 0 {
			fmt.Println("No motes match the given filters.")
			return nil
		}
	}

	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	// Build searchable text including external ref IDs
	moteSearchText := func(m *core.Mote) string {
		text := m.Title + " " + m.Body
		for _, ref := range m.ExternalRefs {
			text += " " + ref.Provider + " " + ref.ID
		}
		return text
	}

	// Build ephemeral BM25 index from all motes (local + global)
	var results []strata.ChunkResult
	{
		chunks := make([]strata.Chunk, len(motes))
		for i, m := range motes {
			chunks[i] = strata.Chunk{
				ID:   m.ID,
				Text: moteSearchText(m),
			}
		}
		idx := strata.BuildBM25Index(chunks)
		results = idx.Search(query, searchTopK)
	}

	if len(results) == 0 {
		switch mode {
		case ModeJSON:
			fmt.Println(`{"query":"` + query + `","results":[]}`)
			return nil
		case ModePlain:
			return nil
		default:
			fmt.Println("No matching motes found.")
			return nil
		}
	}

	if mode == ModeJSON {
		entries := make([]SearchResultEntry, 0, len(results))
		for _, r := range results {
			m := moteMap[r.Chunk.ID]
			if m == nil {
				continue
			}
			entries = append(entries, SearchResultEntry{
				ID:    m.ID,
				Type:  m.Type,
				Title: m.Title,
				Score: r.Score,
			})
		}
		data, jerr := json.MarshalIndent(SearchOutput{Query: query, Results: entries}, "", "  ")
		if jerr != nil {
			return fmt.Errorf("marshal json: %w", jerr)
		}
		fmt.Println(string(data))
		return nil
	}

	if mode == ModePlain {
		// STORY-PLAIN-001: `<id> <type> <score> <title>` per line — id is the
		// first whitespace-delimited token to match the ls/pulse/context plain
		// convention.
		for _, r := range results {
			m := moteMap[r.Chunk.ID]
			if m == nil {
				continue
			}
			fmt.Printf("%s %s %.3f %s\n", m.ID, m.Type, r.Score, m.Title)
		}
		return nil
	}

	fmt.Printf("%-8s  %-24s  %-14s  %s\n", "SCORE", "ID", "TYPE", "TITLE")
	fmt.Println(strings.Repeat("-", 76))
	for _, r := range results {
		m := moteMap[r.Chunk.ID]
		if m == nil {
			continue
		}
		fmt.Printf("%-8.3f  %-24s  %-14s  %s\n",
			r.Score,
			m.ID,
			m.Type,
			format.Truncate(m.Title, 40))
	}

	return nil
}

func moteHasTag(m *core.Mote, tag string) bool {
	for _, t := range m.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
