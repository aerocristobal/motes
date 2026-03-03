package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "Tag analysis tools",
}

var tagsAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit tag quality and specificity",
	RunE:  runTagsAudit,
}

func init() {
	tagsCmd.AddCommand(tagsAuditCmd)
	rootCmd.AddCommand(tagsCmd)
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

	// Co-occurrence suggestions for overloaded tags
	overloaded := findOverloadedTags(idx.TagStats, tagOverloadThreshold)
	if len(overloaded) > 0 {
		fmt.Println()
		fmt.Println("Co-occurrence suggestions for overloaded tags:")
		tagToMotes := buildTagToMotes(motes)
		for _, tag := range overloaded {
			cooccur := findCooccurrences(tag, tagToMotes)
			if len(cooccur) > 0 {
				fmt.Printf("  %s often co-occurs with: %s\n", tag, strings.Join(cooccur, ", "))
			}
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

// findCooccurrences finds the top 3 tags that co-occur with the given tag.
func findCooccurrences(tag string, tagToMotes map[string][]string) []string {
	moteIDs := tagToMotes[tag]
	moteSet := make(map[string]bool, len(moteIDs))
	for _, id := range moteIDs {
		moteSet[id] = true
	}

	// Count co-occurring tags
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

	// Sort by count, take top 3
	type kv struct {
		Tag   string
		Count int
	}
	var pairs []kv
	for t, c := range cooccur {
		pairs = append(pairs, kv{t, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Count > pairs[j].Count
	})

	limit := 3
	if len(pairs) < limit {
		limit = len(pairs)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = pairs[i].Tag
	}
	return result
}
