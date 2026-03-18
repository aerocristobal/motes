package main

import (
	"fmt"

	"motes/internal/core"
)

// runImportGithub wraps the github-import functions for use from onboard.
func runImportGithub(mm *core.MoteManager, im *core.IndexManager, repo string, includeClosed bool, phaseParents bool) (int, error) {
	issues, err := fetchGithubIssues(repo)
	if err != nil {
		return 0, fmt.Errorf("fetch issues from %s: %w", repo, err)
	}

	var openCount, closedCount int
	for _, iss := range issues {
		if iss.State == "CLOSED" {
			closedCount++
		} else {
			openCount++
		}
	}
	fmt.Printf("Found %d issues (%d open, %d closed) in %s\n", len(issues), openCount, closedCount, repo)

	existingSourceIssues := buildSourceIssueSet(mm)

	// Set the global flags that importGithubIssues reads
	oldIncludeClosed := ghImportIncludeClosed
	oldPhaseParents := ghImportPhaseParents
	ghImportIncludeClosed = includeClosed
	ghImportPhaseParents = phaseParents
	defer func() {
		ghImportIncludeClosed = oldIncludeClosed
		ghImportPhaseParents = oldPhaseParents
	}()

	created, _, _ := importGithubIssues(mm, im, repo, issues, existingSourceIssues)
	return created, nil
}
