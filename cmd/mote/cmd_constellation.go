package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var constellationCmd = &cobra.Command{
	Use:   "constellation",
	Short: "Manage constellation discovery",
}

var constellationListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show tag frequency and constellation status",
	RunE:  runConstellationList,
}

var constellationSynthesizeCmd = &cobra.Command{
	Use:   "synthesize",
	Short: "Create hub motes for tags with 3+ motes and no constellation",
	RunE:  runConstellationSynthesize,
}

var synthesizeMinCount int

// Global mutex to protect constellation record writes across processes
var constellationWriteMux sync.Mutex

func init() {
	constellationSynthesizeCmd.Flags().IntVar(&synthesizeMinCount, "min-count", 3, "Minimum mote count for a tag to become a constellation")
	constellationCmd.AddCommand(constellationListCmd)
	constellationCmd.AddCommand(constellationSynthesizeCmd)
	rootCmd.AddCommand(constellationCmd)
}

type tagEntry struct {
	Tag           string
	Count         int
	Specificity   float64
	Constellation string // mote ID if exists, "-" otherwise
}

func runConstellationList(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	motes, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	// Build map of tag -> constellation mote ID
	constellationMap := make(map[string]string)
	for _, m := range motes {
		if m.Type == "constellation" {
			for _, tag := range m.Tags {
				constellationMap[tag] = m.ID
			}
		}
	}

	// Build tag entries
	var entries []tagEntry
	for tag, count := range idx.TagStats {
		specificity := 1.0 / math.Log2(float64(count)+2)
		constellation := "-"
		if cID, ok := constellationMap[tag]; ok {
			constellation = cID
		}
		entries = append(entries, tagEntry{
			Tag:           tag,
			Count:         count,
			Specificity:   specificity,
			Constellation: constellation,
		})
	}

	// Sort by count descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})

	if len(entries) == 0 {
		fmt.Println("No tags found.")
		return nil
	}

	fmt.Printf("%-20s  %-6s  %-12s  %s\n", "TAG", "COUNT", "SPECIFICITY", "CONSTELLATION")
	fmt.Println(strings.Repeat("-", 70))
	for _, e := range entries {
		fmt.Printf("%-20s  %-6d  %-12.2f  %s\n",
			e.Tag, e.Count, e.Specificity, e.Constellation)
	}

	return nil
}

type constellationRecord struct {
	Tag                string   `json:"tag"`
	ConstellationMoteID string  `json:"constellation_mote_id"`
	MemberMoteIDs      []string `json:"member_mote_ids"`
	CreatedAt          string   `json:"created_at"`
}

func runConstellationSynthesize(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	motes, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	// Build existing constellation map
	constellationTags := map[string]bool{}
	for _, m := range motes {
		if m.Type == "constellation" {
			for _, tag := range m.Tags {
				constellationTags[tag] = true
			}
		}
	}

	// Build tag->motes map
	tagMotes := map[string][]string{}
	for _, m := range motes {
		if m.Status == "deprecated" {
			continue
		}
		for _, tag := range m.Tags {
			tagMotes[tag] = append(tagMotes[tag], m.ID)
		}
	}

	// Find eligible tags
	type candidate struct {
		tag      string
		moteIDs  []string
	}
	var candidates []candidate
	for tag, ids := range tagMotes {
		if len(ids) >= synthesizeMinCount && !constellationTags[tag] {
			candidates = append(candidates, candidate{tag: tag, moteIDs: ids})
		}
	}

	if len(candidates) == 0 {
		fmt.Println("No tags eligible for constellation synthesis.")
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].moteIDs) > len(candidates[j].moteIDs)
	})

	var records []constellationRecord
	created := 0

	for _, c := range candidates {
		// Create constellation hub mote
		title := fmt.Sprintf("Constellation: %s", c.tag)
		body := fmt.Sprintf("Hub for the **%s** theme.\n\nMembers:\n", c.tag)
		for _, id := range c.moteIDs {
			body += fmt.Sprintf("- [[%s]]\n", id)
		}

		hub, err := mm.Create("constellation", title, core.CreateOpts{
			Tags:   []string{c.tag},
			Weight: 0.6,
			Body:   body,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create constellation for %s: %v\n", c.tag, err)
			continue
		}

		// Link hub to members via relates_to
		for _, memberID := range c.moteIDs {
			_ = mm.Link(hub.ID, "relates_to", memberID, im)
		}

		records = append(records, constellationRecord{
			Tag:                 c.tag,
			ConstellationMoteID: hub.ID,
			MemberMoteIDs:       c.moteIDs,
			CreatedAt:           time.Now().UTC().Format(time.RFC3339),
		})

		fmt.Printf("  Created %s for tag %q (%d members)\n", hub.ID, c.tag, len(c.moteIDs))
		created++
	}

	// Append to constellations.jsonl
	if len(records) > 0 {
		constellationWriteMux.Lock()
		defer constellationWriteMux.Unlock()

		cPath := filepath.Join(root, "constellations.jsonl")
		f, err := os.OpenFile(cPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			for _, r := range records {
				line, err := json.Marshal(r)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: marshal constellation record: %v\n", err)
					continue
				}
				if _, err := f.Write(line); err != nil {
					fmt.Fprintf(os.Stderr, "warning: write constellation record: %v\n", err)
					continue
				}
				if _, err := f.Write([]byte{'\n'}); err != nil {
					fmt.Fprintf(os.Stderr, "warning: write constellation newline: %v\n", err)
				}
			}
		}

		// Rebuild index to include new constellation edges
		allMotes, _ := mm.ReadAllParallel()
		_ = im.Rebuild(allMotes)
	}

	fmt.Printf("\nSynthesized %d constellations.\n", created)

	// Suppress unused import warning
	_ = idx
	return nil
}
