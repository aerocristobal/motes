// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"motes/internal/core"
)

// beadsIssue represents a single issue from .beads/issues.jsonl.
type beadsIssue struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
}

// runMigrateBeads imports beads issues as motes.
func runMigrateBeads(mm *core.MoteManager, issues []beadsIssue, includeClosed bool) (int, error) {
	existingSourceIssues := buildSourceIssueSet(mm)

	fmt.Println("Importing beads issues...")
	var created int
	for _, issue := range issues {
		if issue.Status == "closed" && !includeClosed {
			continue
		}
		if existingSourceIssues[issue.ID] {
			fmt.Printf("  skipped %s (already imported)\n", issue.ID)
			continue
		}

		moteType, origin := beadsTypeToMote(issue.IssueType)
		weight := beadsPriorityToWeight(issue.Priority)
		tags := inferTags(issue.Title)

		m, err := mm.Create(moteType, issue.Title, core.CreateOpts{
			Tags:        tags,
			Weight:      weight,
			Origin:      origin,
			Body:        issue.Description,
			SourceIssue: issue.ID,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: %q: %v\n", issue.Title, err)
			continue
		}

		if issue.Status == "closed" {
			_ = mm.Update(m.ID, core.UpdateOpts{Status: core.StringPtr("completed")})
		}

		created++
		status := "active"
		if issue.Status == "closed" {
			status = "completed"
		}
		fmt.Printf("  created %s [%s] %s (%s)\n", m.ID, moteType, issue.Title, status)
	}

	return created, nil
}

// parseBeadsFile reads and parses a beads issues.jsonl file.
func parseBeadsFile(path string) ([]beadsIssue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var issues []beadsIssue
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var issue beadsIssue
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			continue
		}
		issues = append(issues, issue)
	}
	return issues, scanner.Err()
}

func countBeadsByStatus(issues []beadsIssue) (open, closed int) {
	for _, i := range issues {
		if i.Status == "closed" {
			closed++
		} else {
			open++
		}
	}
	return
}

func beadsTypeToMote(issueType string) (moteType, origin string) {
	switch issueType {
	case "bug":
		return "lesson", "failure"
	default:
		return "task", "normal"
	}
}

func beadsPriorityToWeight(priority int) float64 {
	switch priority {
	case 0:
		return 1.0
	case 1:
		return 0.9
	case 2:
		return 0.7
	case 3:
		return 0.5
	case 4:
		return 0.3
	default:
		return 0.5
	}
}

// buildSourceIssueSet reads all existing motes and returns a set of SourceIssue values.
func buildSourceIssueSet(mm *core.MoteManager) map[string]bool {
	motes, err := mm.ReadAllParallel()
	if err != nil {
		return map[string]bool{}
	}
	set := make(map[string]bool, len(motes))
	for _, m := range motes {
		if m.SourceIssue != "" {
			set[m.SourceIssue] = true
		}
	}
	return set
}
