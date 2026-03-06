package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Create a new mote",
	RunE:  runAdd,
}

var (
	addType   string
	addTitle  string
	addTags   []string
	addWeight float64
	addOrigin string
	addBody   string
)

func init() {
	addCmd.Flags().StringVar(&addType, "type", "", "Mote type (task|decision|lesson|context|question|constellation|anchor|explore)")
	addCmd.Flags().StringVar(&addTitle, "title", "", "Mote title")
	addCmd.Flags().StringArrayVar(&addTags, "tag", nil, "Tag (repeatable)")
	addCmd.Flags().Float64Var(&addWeight, "weight", 0.5, "Initial weight (0.0-1.0)")
	addCmd.Flags().StringVar(&addOrigin, "origin", "normal", "Origin (normal|failure|revert|hotfix|discovery)")
	addCmd.Flags().StringVar(&addBody, "body", "", "Mote body (use - for stdin)")
	_ = addCmd.MarkFlagRequired("type")
	_ = addCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	// Validate input parameters
	validTypes := []string{"task", "decision", "lesson", "context", "question", "constellation", "anchor", "explore"}
	if err := security.ValidateEnum(addType, validTypes, "type"); err != nil {
		return fmt.Errorf("invalid type: %w", err)
	}

	if addTitle == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if len(addTitle) > 200 {
		return fmt.Errorf("title too long (max 200 characters)")
	}

	for _, tag := range addTags {
		if err := security.ValidateTag(tag); err != nil {
			return fmt.Errorf("invalid tag %q: %w", tag, err)
		}
	}

	if err := security.ValidateWeight(addWeight); err != nil {
		return fmt.Errorf("invalid weight: %w", err)
	}

	validOrigins := []string{"normal", "failure", "revert", "hotfix", "discovery"}
	if err := security.ValidateEnum(addOrigin, validOrigins, "origin"); err != nil {
		return fmt.Errorf("invalid origin: %w", err)
	}

	root, err := findMemoryRoot()
	if err != nil {
		cwd, _ := os.Getwd()
		root = filepath.Join(cwd, ".memory")
	}
	if err := initMemoryDir(root); err != nil {
		return fmt.Errorf("init memory dir: %w", err)
	}

	// Get body from --body flag, stdin, or editor
	var bodyBytes []byte
	stdinStat, _ := os.Stdin.Stat()
	stdinIsPipe := stdinStat != nil && (stdinStat.Mode()&os.ModeCharDevice) == 0
	if addBody == "-" || (addBody == "" && stdinIsPipe) {
		// Read from stdin
		bodyBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	} else if addBody != "" {
		bodyBytes = []byte(addBody)
	} else {
		// Open editor for body
		tmp, err := os.CreateTemp("", "mote-*.md")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmp.Name()
		tmp.Close()
		defer os.Remove(tmpPath)

		if err := openEditor(tmpPath); err != nil {
			return fmt.Errorf("editor: %w", err)
		}

		bodyBytes, err = os.ReadFile(tmpPath)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
	}

	// Validate body size
	if err := security.ValidateBodySize(string(bodyBytes)); err != nil {
		return fmt.Errorf("invalid body: %w", err)
	}

	mm := core.NewMoteManager(root)
	m, err := mm.Create(addType, addTitle, core.CreateOpts{
		Tags:   addTags,
		Weight: addWeight,
		Origin: addOrigin,
		Body:   string(bodyBytes),
	})
	if err != nil {
		return fmt.Errorf("create mote: %w", err)
	}

	// Update BM25 index
	allMotes, _ := mm.ReadAllParallel()
	if allMotes != nil {
		_ = rebuildMoteBM25(root, allMotes)
	}

	fmt.Println("Created mote", m.ID)
	return nil
}
