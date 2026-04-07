// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
	"motes/internal/security"
)

var crystallizeCmd = &cobra.Command{
	Use:   "crystallize <mote-id>",
	Short: "Convert a completed mote into permanent knowledge",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCrystallize,
}

var (
	crystallizeType       string
	crystallizeCandidates bool
)

func init() {
	crystallizeCmd.Flags().StringVar(&crystallizeType, "type", "", "Override target mote type (decision|lesson|context|explore)")
	crystallizeCmd.Flags().BoolVar(&crystallizeCandidates, "candidates", false, "List crystallization candidates")
	rootCmd.AddCommand(crystallizeCmd)
}

func runCrystallize(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	if crystallizeCandidates {
		return listCandidates(mm)
	}

	if len(args) == 0 {
		return fmt.Errorf("mote ID required (or use --candidates)")
	}

	source, err := mm.Read(args[0])
	if err != nil {
		return fmt.Errorf("read mote: %w", err)
	}

	// Infer target type
	targetType := inferCrystallizeType(source)
	if crystallizeType != "" {
		targetType = crystallizeType
	}

	// Infer origin
	origin := inferOrigin(source)

	// Create draft mote
	now := time.Now().UTC()
	draft := &core.Mote{
		ID:             core.GenerateID(scopeFromMoteID(source.ID), targetType),
		Type:           targetType,
		Status:         "active",
		Title:          source.Title,
		Tags:           source.Tags,
		Weight:         source.Weight,
		Origin:         origin,
		CreatedAt:      now,
		SourceIssue:    source.ID,
		CrystallizedAt: &now,
		Body:           source.Body,
	}

	// Write temp file for editing
	draftData, err := core.SerializeMote(draft)
	if err != nil {
		return fmt.Errorf("serialize draft: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "mote-crystallize-*.md")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(draftData); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Get file info before edit
	infoBefore, _ := os.Stat(tmpPath)

	// Open in editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Validate the editor command for security
	if err := security.ValidateCommand(editor); err != nil {
		return fmt.Errorf("invalid EDITOR command: %w", err)
	}

	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor: %w", err)
	}

	// Check if file was modified
	infoAfter, err := os.Stat(tmpPath)
	if err != nil {
		return fmt.Errorf("stat after edit: %w", err)
	}
	if infoAfter.Size() == 0 {
		fmt.Println("Crystallization cancelled (empty file).")
		return nil
	}

	// Re-parse the edited mote
	edited, err := core.ParseMote(tmpPath)
	if err != nil {
		return fmt.Errorf("parse edited mote: %w", err)
	}

	// Write the final mote
	finalData, err := core.SerializeMote(edited)
	if err != nil {
		return fmt.Errorf("serialize final: %w", err)
	}
	motePath, err := mm.MoteFilePath(edited.ID)
	if err != nil {
		return fmt.Errorf("get file path: %w", err)
	}
	if err := core.AtomicWrite(motePath, finalData, 0644); err != nil {
		return fmt.Errorf("write mote: %w", err)
	}

	// Mark source as completed if it was a task
	if source.Type == "task" && source.Status == "active" {
		source.Status = "completed"
		sourceData, _ := core.SerializeMote(source)
		_ = core.AtomicWrite(source.FilePath, sourceData, 0644)
	}

	fmt.Printf("Crystallized %s -> %s (type=%s)\n", source.ID, edited.ID, edited.Type)

	// Suppress unused variable warning
	_ = infoBefore

	return nil
}

func listCandidates(mm *core.MoteManager) error {
	motes, err := mm.ReadAllParallel()
	if err != nil {
		return err
	}

	// Build set of source_issue references
	sourceIssueSet := make(map[string]bool)
	for _, m := range motes {
		if m.SourceIssue != "" {
			sourceIssueSet[m.SourceIssue] = true
		}
	}

	// Find candidates: completed/archived motes not yet crystallized
	var candidates []*core.Mote
	for _, m := range motes {
		if (m.Status == "completed" || m.Status == "archived") && !sourceIssueSet[m.ID] {
			candidates = append(candidates, m)
		}
	}

	if len(candidates) == 0 {
		fmt.Println("No crystallization candidates found.")
		return nil
	}

	fmt.Printf("%-24s  %-12s  %-12s  %s\n", "ID", "TYPE", "STATUS", "TITLE")
	fmt.Println(strings.Repeat("-", 76))
	for _, m := range candidates {
		fmt.Printf("%-24s  %-12s  %-12s  %s\n",
			m.ID, m.Type, m.Status, format.Truncate(m.Title, 40))
	}
	return nil
}

func inferCrystallizeType(m *core.Mote) string {
	body := strings.ToLower(m.Body + " " + m.Title)

	// Check for exploration markers
	explorationMarkers := []string{"evaluated", "compared", "analysis", "investigation",
		"benchmark", "tradeoff", "trade-off", "alternative", "http://", "https://"}
	explorationCount := 0
	for _, marker := range explorationMarkers {
		if strings.Contains(body, marker) {
			explorationCount++
		}
	}
	if explorationCount >= 2 {
		return "explore"
	}

	// Infer from source type
	switch m.Type {
	case "decision":
		return "decision"
	case "question":
		return "context"
	default:
		return "context"
	}
}

func inferOrigin(m *core.Mote) string {
	if m.Origin != "" && m.Origin != "normal" {
		return m.Origin
	}

	text := strings.ToLower(m.Title + " " + m.Body)
	failureMarkers := []string{"fix", "bug", "crash", "fail", "broke", "broken",
		"revert", "hotfix", "regression", "incident"}
	for _, marker := range failureMarkers {
		if strings.Contains(text, marker) {
			return "failure"
		}
	}
	return "normal"
}

func scopeFromMoteID(id string) string {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return "proj"
}
