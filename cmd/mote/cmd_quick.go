package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var quickCmd = &cobra.Command{
	Use:   "quick <sentence>",
	Short: "Quick-capture a mote with auto-inferred type and tags",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runQuick,
}

func init() {
	rootCmd.AddCommand(quickCmd)
}

func runQuick(cmd *cobra.Command, args []string) error {
	sentence := strings.Join(args, " ")

	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	// Auto-infer type from keywords
	moteType := inferType(sentence)

	// Auto-infer tags from existing tag index overlap
	tags := inferQuickTags(sentence, idx.TagStats)

	// Title is first 80 chars, body is full sentence
	title := sentence
	if len(title) > 80 {
		title = title[:80]
	}

	m, err := mm.Create(moteType, title, core.CreateOpts{
		Tags: tags,
		Body: sentence,
	})
	if err != nil {
		return fmt.Errorf("create mote: %w", err)
	}

	fmt.Printf("Created %s %s — %s\n", m.Type, m.ID, m.Title)
	if len(tags) > 0 {
		fmt.Printf("  tags: %s\n", strings.Join(tags, ", "))
	}
	return nil
}

func inferType(sentence string) string {
	lower := strings.ToLower(sentence)

	decisionWords := []string{"decided", "decision", "chose", "chosen", "will use", "going with"}
	for _, w := range decisionWords {
		if strings.Contains(lower, w) {
			return "decision"
		}
	}

	lessonWords := []string{"learned", "lesson", "realized", "discovered", "turns out", "gotcha", "pitfall"}
	for _, w := range lessonWords {
		if strings.Contains(lower, w) {
			return "lesson"
		}
	}

	taskWords := []string{"todo", "need to", "should", "must", "implement", "fix", "add"}
	for _, w := range taskWords {
		if strings.Contains(lower, w) {
			return "task"
		}
	}

	exploreWords := []string{"explored", "investigated", "found that", "research", "analysis"}
	for _, w := range exploreWords {
		if strings.Contains(lower, w) {
			return "explore"
		}
	}

	return "context"
}

func inferQuickTags(sentence string, tagStats map[string]int) []string {
	keywords := core.ExtractKeywords(sentence)
	var tags []string
	for _, kw := range keywords {
		if _, exists := tagStats[kw]; exists {
			tags = append(tags, kw)
		}
	}
	// Cap at 5 tags
	if len(tags) > 5 {
		tags = tags[:5]
	}
	return tags
}
