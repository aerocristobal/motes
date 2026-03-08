package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
)

var promptContextCmd = &cobra.Command{
	Use:    "prompt-context",
	Short:  "Surface relevant motes for a user prompt (UserPromptSubmit hook)",
	Hidden: true,
	RunE:   runPromptContext,
}

func init() {
	rootCmd.AddCommand(promptContextCmd)
}

// hookInput is the JSON structure Claude Code sends to UserPromptSubmit hooks.
type hookInput struct {
	Prompt string `json:"prompt"`
}

// hookOutput is the JSON structure Claude Code expects from hooks.
type hookOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func runPromptContext(cmd *cobra.Command, args []string) error {
	// Read hook JSON from stdin
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var input hookInput
	if err := json.Unmarshal(data, &input); err != nil {
		// Not valid JSON — output empty response
		fmt.Println("{}")
		return nil
	}

	keywords := core.ExtractKeywords(input.Prompt)
	if len(keywords) < 3 {
		// Too few keywords — skip to avoid noise on short prompts
		fmt.Println("{}")
		return nil
	}

	root, err := findMemoryRoot()
	if err != nil {
		fmt.Println("{}")
		return nil
	}

	bm25Idx, err := loadMoteBM25(root)
	if err != nil || bm25Idx == nil || bm25Idx.DocCount == 0 {
		fmt.Println("{}")
		return nil
	}

	query := strings.Join(keywords, " ")
	results := bm25Idx.Search(query, 5)

	// Filter to score >= adaptive threshold and active motes, cap at 3
	mm := core.NewMoteManager(root)
	type surfacedMote struct {
		mote  *core.Mote
		score float64
	}
	minScore := bm25Idx.ThresholdFor("prompt_context")
	var surfaced []surfacedMote
	for _, r := range results {
		if r.Score < minScore {
			continue
		}
		m, err := mm.Read(r.Chunk.ID)
		if err != nil || m.Status != "active" {
			continue
		}
		surfaced = append(surfaced, surfacedMote{mote: m, score: r.Score})
		if len(surfaced) >= 3 {
			break
		}
	}

	if len(surfaced) == 0 {
		fmt.Println("{}")
		return nil
	}

	// Build context string, hard cap 500 chars
	var sb strings.Builder
	sb.WriteString("Relevant motes:\n")
	for _, s := range surfaced {
		line := fmt.Sprintf("- [%.1f] %s (%s): %s", s.score, s.mote.ID, s.mote.Type, format.Truncate(s.mote.Title, 60))
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	ctx := sb.String()
	if len(ctx) > 500 {
		ctx = ctx[:497] + "..."
	}

	out := hookOutput{AdditionalContext: ctx}
	outData, _ := json.Marshal(out)
	fmt.Println(string(outData))

	// Batch access records
	for _, s := range surfaced {
		_ = mm.AppendAccessBatch(s.mote.ID)
	}

	return nil
}
