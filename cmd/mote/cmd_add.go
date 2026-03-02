package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"motes/internal/core"
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
)

func init() {
	addCmd.Flags().StringVar(&addType, "type", "", "Mote type (task|decision|lesson|context|question|constellation|anchor|explore)")
	addCmd.Flags().StringVar(&addTitle, "title", "", "Mote title")
	addCmd.Flags().StringArrayVar(&addTags, "tag", nil, "Tag (repeatable)")
	addCmd.Flags().Float64Var(&addWeight, "weight", 0.5, "Initial weight (0.0-1.0)")
	addCmd.Flags().StringVar(&addOrigin, "origin", "normal", "Origin (normal|failure|revert|hotfix|discovery)")
	_ = addCmd.MarkFlagRequired("type")
	_ = addCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	root, err := findMemoryRoot()
	if err != nil {
		cwd, _ := os.Getwd()
		root = filepath.Join(cwd, ".memory")
	}
	if err := initMemoryDir(root); err != nil {
		return fmt.Errorf("init memory dir: %w", err)
	}

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

	bodyBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
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

	fmt.Println("Created mote", m.ID)
	return nil
}
