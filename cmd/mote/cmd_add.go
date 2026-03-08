package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	addParent string
	addAccept []string
	addSize   string
	addRefs   []string
)

func init() {
	addCmd.Flags().StringVar(&addType, "type", "", "Mote type (task|decision|lesson|context|question|constellation|anchor|explore)")
	addCmd.Flags().StringVar(&addTitle, "title", "", "Mote title")
	addCmd.Flags().StringArrayVar(&addTags, "tag", nil, "Tag (repeatable)")
	addCmd.Flags().Float64Var(&addWeight, "weight", 0.5, "Initial weight (0.0-1.0)")
	addCmd.Flags().StringVar(&addOrigin, "origin", "normal", "Origin (normal|failure|revert|hotfix|discovery)")
	addCmd.Flags().StringVar(&addBody, "body", "", "Mote body (use - for stdin)")
	addCmd.Flags().StringVar(&addParent, "parent", "", "Parent mote ID for hierarchy")
	addCmd.Flags().StringArrayVar(&addAccept, "accept", nil, "Acceptance criterion (repeatable)")
	addCmd.Flags().StringVar(&addSize, "size", "", "Effort size (xs|s|m|l|xl)")
	addCmd.Flags().StringArrayVar(&addRefs, "ref", nil, "External reference (format: provider:id[:url], repeatable)")
	_ = addCmd.MarkFlagRequired("type")
	_ = addCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(addCmd)
}

// parseExternalRef parses a "provider:id[:url]" string into an ExternalRef.
func parseExternalRef(s string) (core.ExternalRef, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return core.ExternalRef{}, fmt.Errorf("expected format provider:id[:url]")
	}
	ref := core.ExternalRef{
		Provider: parts[0],
		ID:       parts[1],
	}
	if len(parts) == 3 {
		ref.URL = parts[2]
	}
	return ref, nil
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

	if addParent != "" {
		if err := security.ValidateMoteID(addParent); err != nil {
			return fmt.Errorf("invalid parent ID: %w", err)
		}
	}

	if addSize != "" {
		validSizes := []string{"xs", "s", "m", "l", "xl"}
		if err := security.ValidateEnum(addSize, validSizes, "size"); err != nil {
			return fmt.Errorf("invalid size: %w", err)
		}
	}

	validOrigins := []string{"normal", "failure", "revert", "hotfix", "discovery"}
	if err := security.ValidateEnum(addOrigin, validOrigins, "origin"); err != nil {
		return fmt.Errorf("invalid origin: %w", err)
	}

	// Parse external refs
	var refs []core.ExternalRef
	for _, r := range addRefs {
		ref, err := parseExternalRef(r)
		if err != nil {
			return fmt.Errorf("invalid --ref %q: %w", r, err)
		}
		refs = append(refs, ref)
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
		Tags:       addTags,
		Weight:     addWeight,
		Origin:     addOrigin,
		Body:       string(bodyBytes),
		Parent:     addParent,
		Acceptance: addAccept,
		Size:       addSize,
	})
	if err != nil {
		return fmt.Errorf("create mote: %w", err)
	}

	// Set external refs if provided
	if len(refs) > 0 {
		m.ExternalRefs = refs
		data, serErr := core.SerializeMote(m)
		if serErr == nil {
			path, _ := mm.MoteFilePath(m.ID)
			_ = core.AtomicWrite(path, data, 0644)
		}
	}

	// Update BM25 index
	allMotes, _ := mm.ReadAllParallel()
	if allMotes != nil {
		_ = rebuildMoteBM25(root, allMotes)
	}

	fmt.Println("Created mote", m.ID)
	return nil
}
