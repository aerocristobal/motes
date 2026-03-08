package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var githubImportCmd = &cobra.Command{
	Use:   "github-import <owner/repo>",
	Short: "Import GitHub issues as motes",
	Long:  "Fetches issues from a GitHub repository using the gh CLI and creates motes from them. Requires gh to be installed and authenticated.",
	Args:  cobra.ExactArgs(1),
	RunE:  runGithubImport,
}

var (
	ghImportIncludeClosed bool
	ghImportDryRun        bool
	ghImportPhaseParents  bool
)

func init() {
	githubImportCmd.Flags().BoolVar(&ghImportIncludeClosed, "include-closed", false, "Also import closed issues (default: open only)")
	githubImportCmd.Flags().BoolVar(&ghImportDryRun, "dry-run", false, "Preview without writing")
	githubImportCmd.Flags().BoolVar(&ghImportPhaseParents, "phase-parents", false, "Create parent task motes per phase label, chained with depends_on")
	rootCmd.AddCommand(githubImportCmd)
}

type githubIssue struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	State     string        `json:"state"`
	Labels    []githubLabel `json:"labels"`
	Body      string        `json:"body"`
	CreatedAt string        `json:"createdAt"`
	ClosedAt  string        `json:"closedAt"`
	URL       string        `json:"url"`
}

type githubLabel struct {
	Name string `json:"name"`
}

func runGithubImport(cmd *cobra.Command, args []string) error {
	repo := args[0]

	// Check gh CLI
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found — install from https://cli.github.com")
	}

	// Fetch issues
	issues, err := fetchGithubIssues(repo)
	if err != nil {
		return fmt.Errorf("fetch issues: %w", err)
	}

	var openCount, closedCount int
	for _, iss := range issues {
		if iss.State == "CLOSED" {
			closedCount++
		} else {
			openCount++
		}
	}
	fmt.Printf("Found %d issues (%d open, %d closed) in %s\n\n", len(issues), openCount, closedCount, repo)

	root := mustFindRoot()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	existingSourceIssues := buildSourceIssueSet(mm)

	if ghImportDryRun {
		return runGithubImportDryRun(repo, issues, existingSourceIssues)
	}

	created, skipped, errCount := importGithubIssues(mm, im, repo, issues, existingSourceIssues)

	// Rebuild indices
	allMotes, _ := mm.ReadAllParallel()
	if allMotes != nil {
		im.Rebuild(allMotes)
		_ = rebuildMoteBM25(root, allMotes)
	}

	fmt.Printf("\nImport complete: %d created, %d skipped, %d errors\n", created, skipped, errCount)
	return nil
}

func runGithubImportDryRun(repo string, issues []githubIssue, existing map[string]bool) error {
	if ghImportPhaseParents {
		phases := detectPhases(issues)
		if len(phases) > 0 {
			fmt.Println("Phase parents:")
			for _, p := range phases {
				key := githubSourceIssueKey(repo, fmt.Sprintf("phase:%s", p))
				if existing[key] {
					fmt.Printf("  [skip] %s (already exists)\n", phaseTitle(p))
				} else {
					fmt.Printf("  [create] %s\n", phaseTitle(p))
				}
			}
			fmt.Println()
		}
	}

	var wouldCreate, wouldSkip int
	for _, iss := range issues {
		if iss.State == "CLOSED" && !ghImportIncludeClosed {
			continue
		}
		key := githubSourceIssueKey(repo, fmt.Sprintf("%d", iss.Number))
		if existing[key] {
			wouldSkip++
			continue
		}
		moteType, _ := githubTypeToMote(iss.Labels)
		fmt.Printf("  [create] #%d [%s] %s\n", iss.Number, moteType, iss.Title)
		wouldCreate++
	}

	fmt.Printf("\nDry run: %d would be created, %d would be skipped\n", wouldCreate, wouldSkip)
	return nil
}

func importGithubIssues(mm *core.MoteManager, im *core.IndexManager, repo string, issues []githubIssue, existing map[string]bool) (created, skipped, errCount int) {
	// Phase parents
	phaseParentIDs := map[string]string{} // label -> mote ID
	if ghImportPhaseParents {
		phases := detectPhases(issues)
		var prevID string
		for _, p := range phases {
			key := githubSourceIssueKey(repo, fmt.Sprintf("phase:%s", p))
			if existing[key] {
				// Find existing mote with this SourceIssue
				motes, _ := mm.ReadAllParallel()
				for _, m := range motes {
					if m.SourceIssue == key {
						phaseParentIDs[p] = m.ID
						prevID = m.ID
						break
					}
				}
				fmt.Printf("  skipped phase parent %s (already exists)\n", phaseTitle(p))
				continue
			}

			title := phaseTitle(p)
			tag := sanitizeTag(p)
			m, err := mm.Create("task", title, core.CreateOpts{
				Tags:        []string{tag},
				Size:        "l",
				SourceIssue: key,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: create phase parent %q: %v\n", title, err)
				errCount++
				continue
			}
			phaseParentIDs[p] = m.ID

			// Chain: this phase depends_on previous phase
			if prevID != "" {
				if err := mm.Link(m.ID, "depends_on", prevID, im); err != nil {
					fmt.Fprintf(os.Stderr, "  warning: link phases: %v\n", err)
				}
			}
			prevID = m.ID
			created++
			fmt.Printf("  created phase parent %s [task] %s\n", m.ID, title)
		}
		if len(phases) > 0 {
			fmt.Println()
		}
	}

	// Import issues
	fmt.Println("Importing issues...")
	for _, iss := range issues {
		if iss.State == "CLOSED" && !ghImportIncludeClosed {
			continue
		}

		key := githubSourceIssueKey(repo, fmt.Sprintf("%d", iss.Number))
		if existing[key] {
			fmt.Printf("  skipped #%d (already imported)\n", iss.Number)
			skipped++
			continue
		}

		moteType, origin := githubTypeToMote(iss.Labels)
		weight := githubPriorityToWeight(iss.Labels)
		tags := githubLabelsToTags(iss.Labels)
		acceptance, acceptanceMet := parseAcceptanceCriteria(iss.Body)

		// Determine parent from phase label
		var parent string
		if ghImportPhaseParents {
			for _, l := range iss.Labels {
				if isPhaseLabel(l.Name) {
					if pid, ok := phaseParentIDs[l.Name]; ok {
						parent = pid
						break
					}
				}
			}
		}

		m, err := mm.Create(moteType, iss.Title, core.CreateOpts{
			Tags:        tags,
			Weight:      weight,
			Origin:      origin,
			Body:        iss.Body,
			SourceIssue: key,
			Parent:      parent,
			Acceptance:  acceptance,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: #%d %q: %v\n", iss.Number, iss.Title, err)
			errCount++
			continue
		}

		// Set ExternalRefs and AcceptanceMet on the in-memory mote before writing
		m.ExternalRefs = []core.ExternalRef{{
			Provider: "github",
			ID:       fmt.Sprintf("%d", iss.Number),
			URL:      iss.URL,
		}}
		if len(acceptanceMet) > 0 {
			m.AcceptanceMet = acceptanceMet
		}
		if iss.State == "CLOSED" || hasLabel(iss.Labels, "complete") {
			m.Status = "completed"
		}
		data, err := core.SerializeMote(m)
		if err == nil {
			path, _ := mm.MoteFilePath(m.ID)
			_ = core.AtomicWrite(path, data, 0644)
		}

		created++
		status := "active"
		if iss.State == "CLOSED" || hasLabel(iss.Labels, "complete") {
			status = "completed"
		}
		fmt.Printf("  created %s [%s] #%d %s (%s)\n", m.ID, moteType, iss.Number, iss.Title, status)
	}

	return created, skipped, errCount
}

// fetchGithubIssues runs gh to fetch issues from a repo.
func fetchGithubIssues(repo string) ([]githubIssue, error) {
	out, err := exec.Command("gh", "issue", "list",
		"-R", repo,
		"--state", "all",
		"--json", "number,title,state,labels,body,createdAt,closedAt,url",
		"--limit", "1000",
	).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh: %s", string(exitErr.Stderr))
		}
		return nil, err
	}

	var issues []githubIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}
	return issues, nil
}

// githubTypeToMote maps issue labels to mote type and origin.
func githubTypeToMote(labels []githubLabel) (moteType, origin string) {
	if hasLabel(labels, "bug") {
		return "lesson", "failure"
	}
	return "task", "normal"
}

// githubPriorityToWeight maps priority labels to weight values.
func githubPriorityToWeight(labels []githubLabel) float64 {
	for _, l := range labels {
		switch l.Name {
		case "priority:critical":
			return 1.0
		case "priority:high":
			return 0.8
		case "priority:medium":
			return 0.5
		}
	}
	return 0.5
}

// githubLabelsToTags converts labels to tags, filtering out status/priority labels.
func githubLabelsToTags(labels []githubLabel) []string {
	var tags []string
	for _, l := range labels {
		if isStatusLabel(l.Name) || isPriorityLabel(l.Name) {
			continue
		}
		tags = append(tags, sanitizeTag(l.Name))
	}
	return tags
}

var checklistRe = regexp.MustCompile(`(?m)^- \[([ xX])\] (.+)$`)

// parseAcceptanceCriteria extracts checklist items from issue body.
func parseAcceptanceCriteria(body string) (criteria []string, met []bool) {
	matches := checklistRe.FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		criteria = append(criteria, strings.TrimSpace(m[2]))
		met = append(met, m[1] != " ")
	}
	return
}

// githubSourceIssueKey returns the idempotency key for a GitHub issue.
func githubSourceIssueKey(repo string, id string) string {
	return fmt.Sprintf("github:%s#%s", repo, id)
}

// detectPhases finds unique phase labels and sorts them naturally.
func detectPhases(issues []githubIssue) []string {
	seen := map[string]bool{}
	for _, iss := range issues {
		for _, l := range iss.Labels {
			if isPhaseLabel(l.Name) && !seen[l.Name] {
				seen[l.Name] = true
			}
		}
	}
	phases := make([]string, 0, len(seen))
	for p := range seen {
		phases = append(phases, p)
	}
	sort.Strings(phases)
	return phases
}

// phaseTitle converts a phase label to a human-readable title.
// e.g. "phase:1-mvp" → "Phase 1: MVP"
func phaseTitle(label string) string {
	// Strip "phase:" prefix
	s := strings.TrimPrefix(label, "phase:")
	// Split on first hyphen: "1-mvp" → "1", "mvp"
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 2 {
		return fmt.Sprintf("Phase %s: %s", parts[0], strings.ToUpper(parts[1]))
	}
	return fmt.Sprintf("Phase %s", s)
}

// sanitizeTag converts a label to a valid mote tag (lowercase, hyphens).
func sanitizeTag(label string) string {
	s := strings.ToLower(label)
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func hasLabel(labels []githubLabel, name string) bool {
	for _, l := range labels {
		if l.Name == name {
			return true
		}
	}
	return false
}

func isPhaseLabel(name string) bool {
	return strings.HasPrefix(name, "phase:")
}

func isStatusLabel(name string) bool {
	switch name {
	case "complete", "in progress":
		return true
	}
	return false
}

func isPriorityLabel(name string) bool {
	return strings.HasPrefix(name, "priority:")
}
