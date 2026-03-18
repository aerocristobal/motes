package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

// TagsOutput is the JSON output structure for mote tags --json.
type TagsOutput struct {
	Tags []TagEntry `json:"tags"`
}

// TagEntry represents a tag in JSON output.
type TagEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

var (
	tagsCompact bool
	tagsJSON    bool
)

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "Tag analysis tools",
	RunE:  runTagsList,
}

var tagsAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit tag quality and specificity",
	RunE:  runTagsAudit,
}

func init() {
	tagsCmd.Flags().BoolVar(&tagsCompact, "compact", false, "Single line, comma-separated tag list")
	tagsCmd.Flags().BoolVar(&tagsJSON, "json", false, "Output in JSON format")
	tagsCmd.AddCommand(tagsAuditCmd)
	rootCmd.AddCommand(tagsCmd)
}

func runTagsList(cmd *cobra.Command, args []string) error {
	// If subcommand not specified, list tags
	root := mustFindRoot()
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	if len(idx.TagStats) == 0 {
		fmt.Println("No tags found.")
		return nil
	}

	var tags []string
	for tag := range idx.TagStats {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	if tagsJSON {
		entries := make([]TagEntry, len(tags))
		for i, tag := range tags {
			entries[i] = TagEntry{Name: tag, Count: idx.TagStats[tag]}
		}
		data, err := json.MarshalIndent(TagsOutput{Tags: entries}, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if tagsCompact {
		fmt.Println(strings.Join(tags, ", "))
		return nil
	}

	for _, tag := range tags {
		fmt.Printf("%-20s  %d\n", tag, idx.TagStats[tag])
	}
	return nil
}

type auditEntry struct {
	Tag         string
	Count       int
	Specificity float64
	Status      string
}

func runTagsAudit(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
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

	if len(idx.TagStats) == 0 {
		fmt.Println("No tags found.")
		return nil
	}

	tagOverloadThreshold := cfg.Dream.PreScan.TagOverloadThreshold
	if tagOverloadThreshold <= 0 {
		tagOverloadThreshold = 15
	}

	// Build audit entries
	var entries []auditEntry
	for tag, count := range idx.TagStats {
		specificity := 1.0 / math.Log2(float64(count)+2)
		status := "ok"
		if count > tagOverloadThreshold {
			status = fmt.Sprintf("overloaded (>%d)", tagOverloadThreshold)
		} else if count == 1 {
			status = "unique"
		}
		entries = append(entries, auditEntry{
			Tag:         tag,
			Count:       count,
			Specificity: specificity,
			Status:      status,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})

	fmt.Printf("%-20s  %-6s  %-12s  %s\n", "TAG", "COUNT", "SPECIFICITY", "STATUS")
	fmt.Println(strings.Repeat("-", 65))
	for _, e := range entries {
		fmt.Printf("%-20s  %-6d  %-12.2f  %s\n",
			e.Tag, e.Count, e.Specificity, e.Status)
	}

	tagToMotes := buildTagToMotes(motes)

	// Check for overloaded and orphaned tags
	overloaded := findOverloadedTags(idx.TagStats, tagOverloadThreshold)
	orphaned := findOrphanedTags(tagToMotes, motes)

	// Mark orphaned tags in entries
	orphanedSet := make(map[string]bool, len(orphaned))
	for _, tag := range orphaned {
		orphanedSet[tag] = true
	}
	for i := range entries {
		if orphanedSet[entries[i].Tag] {
			entries[i].Status = "orphaned"
		}
	}

	// Clean health report: if no issues, print OK and return
	if len(overloaded) == 0 && len(orphaned) == 0 {
		fmt.Println("\nTag health: OK")
		return nil
	}

	// Co-occurrence suggestions for overloaded tags with split suggestions
	if len(overloaded) > 0 {
		fmt.Println()
		fmt.Println("Co-occurrence suggestions for overloaded tags:")
		for _, tag := range overloaded {
			cooccur := findCooccurrencesWithCounts(tag, tagToMotes)
			if len(cooccur) == 0 {
				continue
			}
			// Format co-occurrence counts
			parts := make([]string, len(cooccur))
			for j, c := range cooccur {
				parts[j] = fmt.Sprintf("%s (%d)", c.Tag, c.Count)
			}
			fmt.Printf("  %s (%d motes): co-occurs with %s\n", tag, idx.TagStats[tag], strings.Join(parts, ", "))
			// Split suggestion using top 2
			if len(cooccur) >= 2 {
				fmt.Printf("    Split suggestion: %s-%s, %s-%s\n", tag, cooccur[0].Tag, tag, cooccur[1].Tag)
			} else if len(cooccur) == 1 {
				fmt.Printf("    Split suggestion: %s-%s\n", tag, cooccur[0].Tag)
			}
		}
	}

	// Orphaned tags section
	if len(orphaned) > 0 {
		fmt.Println()
		fmt.Println("Orphaned tags (only on deprecated/archived motes):")
		for _, tag := range orphaned {
			count := len(tagToMotes[tag])
			fmt.Printf("  %-20s (%d mote", tag, count)
			if count != 1 {
				fmt.Print("s")
			}
			fmt.Println(", all deprecated/archived)")
		}
	}

	return nil
}

func findOverloadedTags(tagStats map[string]int, threshold int) []string {
	if threshold <= 0 {
		threshold = 15
	}
	var overloaded []string
	for tag, count := range tagStats {
		if count > threshold {
			overloaded = append(overloaded, tag)
		}
	}
	sort.Strings(overloaded)
	return overloaded
}

// buildTagToMotes maps tag -> list of mote IDs that have that tag.
func buildTagToMotes(motes []*core.Mote) map[string][]string {
	m := make(map[string][]string)
	for _, mote := range motes {
		for _, tag := range mote.Tags {
			m[tag] = append(m[tag], mote.ID)
		}
	}
	return m
}

// findOrphanedTags returns tags where ALL motes carrying them are deprecated or archived.
func findOrphanedTags(tagToMotes map[string][]string, motes []*core.Mote) []string {
	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	var orphaned []string
	for tag, moteIDs := range tagToMotes {
		allInactive := true
		for _, id := range moteIDs {
			m, ok := moteMap[id]
			if !ok {
				continue
			}
			if m.Status != "deprecated" && m.Status != "archived" {
				allInactive = false
				break
			}
		}
		if allInactive {
			orphaned = append(orphaned, tag)
		}
	}
	sort.Strings(orphaned)
	return orphaned
}

// CooccurEntry pairs a tag with its co-occurrence count.
type CooccurEntry struct {
	Tag   string
	Count int
}

// findCooccurrencesWithCounts finds the top 3 tags that co-occur with the given tag, with counts.
func findCooccurrencesWithCounts(tag string, tagToMotes map[string][]string) []CooccurEntry {
	moteIDs := tagToMotes[tag]
	moteSet := make(map[string]bool, len(moteIDs))
	for _, id := range moteIDs {
		moteSet[id] = true
	}

	cooccur := map[string]int{}
	for otherTag, otherMotes := range tagToMotes {
		if otherTag == tag {
			continue
		}
		for _, id := range otherMotes {
			if moteSet[id] {
				cooccur[otherTag]++
			}
		}
	}

	var pairs []CooccurEntry
	for t, c := range cooccur {
		pairs = append(pairs, CooccurEntry{t, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Count > pairs[j].Count
	})

	limit := 3
	if len(pairs) < limit {
		limit = len(pairs)
	}
	return pairs[:limit]
}

