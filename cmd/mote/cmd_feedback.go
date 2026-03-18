package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var feedbackCmd = &cobra.Command{
	Use:   "feedback <mote-id> <useful|irrelevant>",
	Short: "Provide relevance feedback on a mote",
	Args:  cobra.ExactArgs(2),
	RunE:  runFeedback,
}

func init() {
	rootCmd.AddCommand(feedbackCmd)
}

type FeedbackEntry struct {
	MoteID    string  `json:"mote_id"`
	Feedback  string  `json:"feedback"`
	Timestamp string  `json:"timestamp"`
	OldWeight float64 `json:"old_weight"`
	NewWeight float64 `json:"new_weight"`
}

func runFeedback(cmd *cobra.Command, args []string) error {
	moteID := args[0]
	feedback := args[1]

	if feedback != "useful" && feedback != "irrelevant" {
		return fmt.Errorf("feedback must be 'useful' or 'irrelevant', got %q", feedback)
	}

	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	m, err := mm.Read(moteID)
	if err != nil {
		return fmt.Errorf("read mote: %w", err)
	}

	oldWeight := m.Weight
	var newWeight float64

	switch feedback {
	case "useful":
		newWeight = oldWeight + 0.05
		if newWeight > 1.0 {
			newWeight = 1.0
		}
	case "irrelevant":
		newWeight = oldWeight - 0.05
		if newWeight < 0.1 {
			newWeight = 0.1
		}
	}

	if err := mm.Update(moteID, core.UpdateOpts{
		Weight: &newWeight,
	}); err != nil {
		return fmt.Errorf("update weight: %w", err)
	}

	// Log feedback
	entry := FeedbackEntry{
		MoteID:    moteID,
		Feedback:  feedback,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		OldWeight: oldWeight,
		NewWeight: newWeight,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal feedback: %w", err)
	}

	logPath := filepath.Join(root, ".feedback_log.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open feedback log: %w", err)
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte("\n"))

	fmt.Printf("Feedback recorded: %s %s (weight: %.2f -> %.2f)\n",
		moteID, feedback, oldWeight, newWeight)
	return nil
}
