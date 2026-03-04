package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Detect and migrate existing systems (beads, MEMORY.md) into motes",
	Long: `Onboards the current project (or global layer with --global) by:
  1. Detecting existing sources (.beads/, MEMORY.md, .memory/)
  2. Initializing .memory/ if absent
  3. Migrating MEMORY.md sections into typed motes
  4. Importing beads issues as motes (idempotent)
  5. Rebuilding the index
  6. Updating CLAUDE.md with motes instructions`,
	RunE: runOnboard,
}

var (
	onboardGlobal        bool
	onboardDryRun        bool
	onboardIncludeClosed bool
)

func init() {
	onboardCmd.Flags().BoolVar(&onboardGlobal, "global", false, "Onboard the global layer (~/.claude/memory/)")
	onboardCmd.Flags().BoolVar(&onboardDryRun, "dry-run", false, "Show what would happen without writing")
	onboardCmd.Flags().BoolVar(&onboardIncludeClosed, "include-closed", false, "Also import closed beads issues (default: open only)")
	rootCmd.AddCommand(onboardCmd)
}

// beadsIssue represents a single issue from .beads/issues.jsonl.
type beadsIssue struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
}

func runOnboard(cmd *cobra.Command, args []string) error {
	if onboardGlobal {
		return runOnboardGlobal()
	}
	return runOnboardProject()
}

func runOnboardProject() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root := filepath.Join(cwd, ".memory")

	// --- Detection ---
	fmt.Println("Detecting sources...")

	beadsPath := filepath.Join(cwd, ".beads", "issues.jsonl")
	beadsIssues, _ := parseBeadsFile(beadsPath)
	openBeads, closedBeads := countBeadsByStatus(beadsIssues)

	memoryMDPath := findMemoryMD(cwd)

	memoryDirExists := dirExists(root)

	claudeMDPath := filepath.Join(cwd, "CLAUDE.md")
	claudeHasMotes := fileContains(claudeMDPath, "## Motes")

	// Print summary
	fmt.Println()
	if len(beadsIssues) > 0 {
		fmt.Printf("  .beads/issues.jsonl  %d open, %d closed\n", openBeads, closedBeads)
	} else {
		fmt.Println("  .beads/              not found")
	}
	if memoryMDPath != "" {
		fmt.Printf("  %s         found\n", filepath.Base(memoryMDPath))
	} else {
		fmt.Println("  MEMORY.md            not found")
	}
	if memoryDirExists {
		fmt.Println("  .memory/             exists")
	} else {
		fmt.Println("  .memory/             will create")
	}
	if claudeHasMotes {
		fmt.Println("  CLAUDE.md            has ## Motes")
	} else {
		fmt.Println("  CLAUDE.md            needs ## Motes")
	}
	fmt.Println()

	if onboardDryRun {
		fmt.Println("Dry run — no changes made.")
		if memoryMDPath != "" {
			data, _ := os.ReadFile(memoryMDPath)
			sections := parseSections(string(data))
			fmt.Printf("  Would create %d motes from %s\n", len(sections), filepath.Base(memoryMDPath))
		}
		importCount := openBeads
		if onboardIncludeClosed {
			importCount += closedBeads
		}
		if importCount > 0 {
			fmt.Printf("  Would import %d beads issues\n", importCount)
		}
		return nil
	}

	// --- Init .memory/ ---
	if !memoryDirExists {
		fmt.Println("Initializing .memory/...")
		if err := scaffoldMemoryDir(root); err != nil {
			return err
		}
	}

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	// Build existing source_issue set for idempotency
	existingSourceIssues := buildSourceIssueSet(mm)

	var totalCreated int

	// --- Migrate MEMORY.md ---
	if memoryMDPath != "" {
		data, err := os.ReadFile(memoryMDPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", memoryMDPath, err)
		}
		sections := parseSections(string(data))
		if len(sections) > 0 {
			fmt.Printf("Migrating %s (%d sections)...\n", filepath.Base(memoryMDPath), len(sections))
			for _, s := range sections {
				m, err := mm.Create(s.moteType, s.heading, core.CreateOpts{
					Tags:   s.tags,
					Origin: s.origin,
					Body:   strings.TrimSpace(s.body),
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "  warning: %q: %v\n", s.heading, err)
					continue
				}
				totalCreated++
				fmt.Printf("  created %s [%s] %s\n", m.ID, s.moteType, s.heading)
			}
			// Archive original
			archivePath := memoryMDPath + ".migrated." + time.Now().Format("20060102")
			if err := os.Rename(memoryMDPath, archivePath); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: could not archive: %v\n", err)
			} else {
				fmt.Printf("  archived to %s\n", filepath.Base(archivePath))
			}
		}
	}

	// --- Migrate beads ---
	if len(beadsIssues) > 0 {
		fmt.Println("Importing beads issues...")
		for _, issue := range beadsIssues {
			if issue.Status == "closed" && !onboardIncludeClosed {
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

			// Mark completed if closed
			if issue.Status == "closed" {
				_ = mm.Update(m.ID, map[string]interface{}{"status": "completed"})
			}

			totalCreated++
			status := "active"
			if issue.Status == "closed" {
				status = "completed"
			}
			fmt.Printf("  created %s [%s] %s (%s)\n", m.ID, moteType, issue.Title, status)
		}
	}

	// --- Rebuild index ---
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	// --- Update CLAUDE.md ---
	modified, err := ensureClaudeMD(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: CLAUDE.md: %v\n", err)
	} else if modified {
		fmt.Println("Updated CLAUDE.md with ## Motes section")
	}

	fmt.Printf("\nOnboarding complete: %d motes created.\n", totalCreated)

	// --- Print cleanup instructions ---
	if len(beadsIssues) > 0 {
		fmt.Println(`
--- Manual steps ---
Remove bd references from CLAUDE.md:
  - Remove the bd 'Quick Reference' section
  - Replace 'bd sync' with 'mote session-end' in session workflow
  - Remove .beads/ once you've verified the import`)
	}

	return nil
}

func runOnboardGlobal() error {
	gRoot := globalRoot()

	// --- Detection ---
	fmt.Println("Detecting global sources...")

	globalBeadsPath := filepath.Join(os.Getenv("HOME"), ".beads", "issues.jsonl")
	beadsIssues, _ := parseBeadsFile(globalBeadsPath)
	openBeads, closedBeads := countBeadsByStatus(beadsIssues)

	memoryDirExists := dirExists(filepath.Join(gRoot, "nodes"))

	fmt.Println()
	if len(beadsIssues) > 0 {
		fmt.Printf("  ~/.beads/issues.jsonl  %d open, %d closed\n", openBeads, closedBeads)
	} else {
		fmt.Println("  ~/.beads/              not found")
	}
	if memoryDirExists {
		fmt.Println("  ~/.claude/memory/      exists")
	} else {
		fmt.Println("  ~/.claude/memory/      will create")
	}
	fmt.Println()

	if onboardDryRun {
		fmt.Println("Dry run — no changes made.")
		importCount := openBeads
		if onboardIncludeClosed {
			importCount += closedBeads
		}
		if importCount > 0 {
			fmt.Printf("  Would import %d beads issues\n", importCount)
		}
		return nil
	}

	// --- Init global memory ---
	fmt.Println("Initializing global memory...")
	if err := scaffoldMemoryDir(gRoot); err != nil {
		return err
	}

	mm := core.NewMoteManager(gRoot)
	im := core.NewIndexManager(gRoot)
	existingSourceIssues := buildSourceIssueSet(mm)

	var totalCreated int

	// --- Migrate beads ---
	if len(beadsIssues) > 0 {
		fmt.Println("Importing global beads issues...")
		for _, issue := range beadsIssues {
			if issue.Status == "closed" && !onboardIncludeClosed {
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
				_ = mm.Update(m.ID, map[string]interface{}{"status": "completed"})
			}

			totalCreated++
			fmt.Printf("  created %s [%s] %s\n", m.ID, moteType, issue.Title)
		}
	}

	// --- Rebuild index ---
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	fmt.Printf("\nGlobal onboarding complete: %d motes created.\n", totalCreated)

	if len(beadsIssues) > 0 {
		fmt.Println(`
--- Manual steps ---
Update ~/.claude/CLAUDE.md:
  - Replace bd instructions with motes instructions
  - Remove ~/.beads/ once you've verified the import`)
	}

	return nil
}

// scaffoldMemoryDir creates the full .memory/ directory structure.
func scaffoldMemoryDir(root string) error {
	for _, dir := range []string{"nodes", "dream", "strata"} {
		if _, err := ensureDir(filepath.Join(root, dir)); err != nil {
			return err
		}
	}

	// config.yaml
	configPath := filepath.Join(root, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := core.SaveConfig(root, core.DefaultConfig()); err != nil {
			return err
		}
	}

	// Empty JSONL files
	for _, rel := range []string{
		"index.jsonl",
		"constellations.jsonl",
		filepath.Join("dream", "log.jsonl"),
		filepath.Join("strata", "query_log.jsonl"),
	} {
		createFileIfAbsent(filepath.Join(root, rel), nil)
	}

	return nil
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

func findMemoryMD(dir string) string {
	for _, name := range []string{"MEMORY.md", "memory.md"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}
