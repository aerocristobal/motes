// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/skills"
)

type onboardSource string

const (
	sourceMarkdown onboardSource = "markdown"
	sourceBeads    onboardSource = "beads"
	sourceGithub   onboardSource = "github"
	sourceFresh    onboardSource = "fresh"
)

type sourceDetection struct {
	beadsIssues     []beadsIssue
	openBeads       int
	closedBeads     int
	memoryMDPath    string
	memoryDirExists bool
	ghAvailable     bool
	claudeHasMotes  bool
	settingsHasBd   bool
}

type menuOption struct {
	source      onboardSource
	label       string
	description string
}

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Detect and migrate existing systems (beads, MEMORY.md) into motes",
	Long: `Onboards the current project (or global layer with --global) by:
  1. Detecting existing sources (.beads/, MEMORY.md, .memory/, gh CLI)
  2. Prompting which source to migrate from (or use --from to skip)
  3. Initializing .memory/ if absent
  4. Migrating the selected source into motes
  5. Rebuilding the index
  6. Updating CLAUDE.md with motes instructions`,
	RunE: runOnboard,
}

var (
	onboardGlobal        bool
	onboardDryRun        bool
	onboardIncludeClosed bool
	onboardCleanup       bool
	onboardFrom          string
	onboardRepo          string
	onboardPhaseParents  bool
	onboardCodex         bool
	onboardGemini        bool
)

func init() {
	onboardCmd.Flags().BoolVar(&onboardGlobal, "global", false, "Onboard the global layer (~/.motes/)")
	onboardCmd.Flags().BoolVar(&onboardDryRun, "dry-run", false, "Show what would happen without writing")
	onboardCmd.Flags().BoolVar(&onboardIncludeClosed, "include-closed", false, "Also import closed beads/github issues (default: open only)")
	onboardCmd.Flags().BoolVar(&onboardCleanup, "cleanup", false, "Remove .beads/ after successful import")
	onboardCmd.Flags().StringVar(&onboardFrom, "from", "", "Migration source: markdown, beads, github, fresh (skips interactive prompt)")
	onboardCmd.Flags().StringVar(&onboardRepo, "repo", "", "GitHub repo (owner/repo) for --from=github")
	onboardCmd.Flags().BoolVar(&onboardPhaseParents, "phase-parents", false, "Create parent task motes per phase label (github import)")
	onboardCmd.Flags().BoolVar(&onboardCodex, "codex", false, "Install OpenAI Codex hooks at ~/.codex/ and skills at ~/.agents/skills/ (auto-enabled if ~/.codex/ exists)")
	onboardCmd.Flags().BoolVar(&onboardGemini, "gemini", false, "Install Gemini CLI hooks at ~/.gemini/settings.json and skills at ~/.agents/skills/ (auto-enabled if ~/.gemini/ exists)")
	rootCmd.AddCommand(onboardCmd)
}

// codexEnabled returns true when the user explicitly opted in via --codex,
// or when ~/.codex/ already exists (auto-detect).
func codexEnabled(homeDir string) bool {
	if onboardCodex {
		return true
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".codex")); err == nil {
		return true
	}
	return false
}

// geminiEnabled returns true when the user explicitly opted in via --gemini,
// or when ~/.gemini/ already exists (auto-detect).
func geminiEnabled(homeDir string) bool {
	if onboardGemini {
		return true
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".gemini")); err == nil {
		return true
	}
	return false
}

// agentsSkillsEnabled is true when either Codex or Gemini CLI is enabled.
// Both honor ~/.agents/skills/ at higher precedence than their own tool-
// specific paths (~/.codex/skills/ doesn't exist; ~/.gemini/skills/ is
// dominated by ~/.agents/skills/), so the install logic is shared.
func agentsSkillsEnabled(homeDir string) bool {
	return codexEnabled(homeDir) || geminiEnabled(homeDir)
}

func runOnboard(cmd *cobra.Command, args []string) error {
	if onboardGlobal {
		return runOnboardGlobal()
	}
	return runOnboardProject()
}

// detectSources scans the environment for migration sources.
func detectSources(cwd string) sourceDetection {
	var d sourceDetection

	beadsPath := filepath.Join(cwd, ".beads", "issues.jsonl")
	d.beadsIssues, _ = parseBeadsFile(beadsPath)
	d.openBeads, d.closedBeads = countBeadsByStatus(d.beadsIssues)

	d.memoryMDPath = findMemoryMD(cwd)
	d.memoryDirExists = dirExists(filepath.Join(cwd, ".memory"))

	claudeMDPath := filepath.Join(cwd, "CLAUDE.md")
	d.claudeHasMotes = fileContains(claudeMDPath, "## Motes")

	_, err := exec.LookPath("gh")
	d.ghAvailable = err == nil

	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")
	d.settingsHasBd = settingsHasBdRefs(filepath.Join(claudeDir, "settings.json"))

	return d
}

// printSourceSummary prints what was detected.
func printSourceSummary(d sourceDetection) {
	fmt.Println()
	if len(d.beadsIssues) > 0 {
		fmt.Printf("  .beads/       %d open, %d closed\n", d.openBeads, d.closedBeads)
	} else {
		fmt.Println("  .beads/       not found")
	}
	if d.memoryMDPath != "" {
		fmt.Printf("  %-14s found\n", filepath.Base(d.memoryMDPath))
	} else {
		fmt.Println("  MEMORY.md     not found")
	}
	if d.ghAvailable {
		fmt.Println("  gh CLI        available")
	} else {
		fmt.Println("  gh CLI        not found")
	}
	if d.memoryDirExists {
		fmt.Println("  .memory/      exists")
	} else {
		fmt.Println("  .memory/      will create")
	}
	if d.claudeHasMotes {
		fmt.Println("  CLAUDE.md     has ## Motes")
	} else {
		fmt.Println("  CLAUDE.md     needs ## Motes")
	}
	if d.settingsHasBd {
		fmt.Println("  settings.json has bd references")
	}
	fmt.Println()
}

// buildMenu builds numbered options from detected sources.
func buildMenu(d sourceDetection) []menuOption {
	var opts []menuOption

	if d.memoryMDPath != "" {
		opts = append(opts, menuOption{
			source:      sourceMarkdown,
			label:       "Markdown files (MEMORY.md)",
			description: "Splits sections into typed motes, archives the original.",
		})
	}

	if len(d.beadsIssues) > 0 {
		desc := fmt.Sprintf("Imports %d open issues as motes", d.openBeads)
		if d.closedBeads > 0 {
			desc += fmt.Sprintf(" (use --include-closed for all %d).", d.openBeads+d.closedBeads)
		} else {
			desc += "."
		}
		opts = append(opts, menuOption{
			source:      sourceBeads,
			label:       "Beads issues",
			description: desc,
		})
	}

	if d.ghAvailable {
		opts = append(opts, menuOption{
			source:      sourceGithub,
			label:       "GitHub Issues",
			description: "Fetches issues from a GitHub repo via the gh CLI.",
		})
	}

	// Fresh start is always last
	opts = append(opts, menuOption{
		source:      sourceFresh,
		label:       "Fresh start",
		description: "No migration — just initialize and configure.",
	})

	return opts
}

// promptSelection prints the menu and reads a choice from r.
func promptSelection(r io.Reader, opts []menuOption) (menuOption, error) {
	fmt.Println("Select a migration source:")
	fmt.Println()
	for i, opt := range opts {
		fmt.Printf("  %d) %s\n", i+1, opt.label)
		fmt.Printf("     %s\n", opt.description)
		fmt.Println()
	}
	fmt.Printf("Enter choice [1-%d]: ", len(opts))

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return menuOption{}, fmt.Errorf("no input received")
	}
	input := strings.TrimSpace(scanner.Text())
	n, err := strconv.Atoi(input)
	if err != nil {
		return menuOption{}, fmt.Errorf("invalid choice: %q (not a number)", input)
	}
	if n < 1 || n > len(opts) {
		return menuOption{}, fmt.Errorf("invalid choice: %d (must be 1-%d)", n, len(opts))
	}
	return opts[n-1], nil
}

// promptRepo reads an owner/repo string from r.
func promptRepo(r io.Reader) (string, error) {
	fmt.Print("Enter GitHub repo (owner/repo): ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input received")
	}
	repo := strings.TrimSpace(scanner.Text())
	if !strings.Contains(repo, "/") || strings.Count(repo, "/") != 1 {
		return "", fmt.Errorf("invalid repo format: %q (expected owner/repo)", repo)
	}
	parts := strings.Split(repo, "/")
	if parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid repo format: %q (expected owner/repo)", repo)
	}
	return repo, nil
}

// runCommonSetup performs post-migration setup: index rebuild, CLAUDE.md, hooks, skills.
func runCommonSetup(cwd, root string, mm *core.MoteManager, im *core.IndexManager, dryRun bool) error {
	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")

	if dryRun {
		settingsHasBd := settingsHasBdRefs(filepath.Join(claudeDir, "settings.json"))
		if settingsHasBd {
			migrateClaudeSettings(claudeDir, true)
		}
		ensureClaudeHooks(claudeDir, true)
		if codexEnabled(home) {
			ensureCodexHooks(filepath.Join(home, ".codex"), true)
		}
		if geminiEnabled(home) {
			ensureGeminiSettings(filepath.Join(home, ".gemini"), true)
		}
		ensureMoteSkills(home, true)
		ensureClaudeGlobalShim(home, true)
		if codexEnabled(home) {
			ensureCodexGlobalShim(home, true)
		}
		if geminiEnabled(home) {
			ensureGeminiGlobalShim(home, true)
		}
		ensurePreCommitHook(cwd, true)
		return nil
	}

	// Rebuild index
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	// Update CLAUDE.md
	modified, err := ensureClaudeMD(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: CLAUDE.md: %v\n", err)
	} else if modified {
		fmt.Println("Updated CLAUDE.md with ## Motes section")
	}

	// Migrate settings.json hooks
	migrated, err := migrateClaudeSettings(claudeDir, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: settings migration: %v\n", err)
	} else if migrated > 0 {
		fmt.Printf("Migrated %d hook(s) in ~/.claude/settings.json\n", migrated)
	}

	// Install hooks
	if err := ensureClaudeHooks(claudeDir, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: hooks installation: %v\n", err)
	}

	// Install Codex hooks when --codex set or ~/.codex/ exists
	if codexEnabled(home) {
		if err := ensureCodexHooks(filepath.Join(home, ".codex"), false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: codex hooks installation: %v\n", err)
		}
	}

	// Install Gemini CLI hooks + context.fileName when --gemini set or ~/.gemini/ exists
	if geminiEnabled(home) {
		if err := ensureGeminiSettings(filepath.Join(home, ".gemini"), false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: gemini settings installation: %v\n", err)
		}
	}

	// Install skills (writes to ~/.agents/skills too when codex or gemini is enabled)
	if err := ensureMoteSkills(home, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: skills installation: %v\n", err)
	}

	// Install per-agent global shims pointing at MOTES.md.
	if err := ensureClaudeGlobalShim(home, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: claude shim: %v\n", err)
	}
	if codexEnabled(home) {
		if err := ensureCodexGlobalShim(home, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: codex shim: %v\n", err)
		}
	}
	if geminiEnabled(home) {
		if err := ensureGeminiGlobalShim(home, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: gemini shim: %v\n", err)
		}
	}

	// Install pre-commit hook
	if err := ensurePreCommitHook(cwd, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: pre-commit hook: %v\n", err)
	}

	return nil
}

func runOnboardProject() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root := filepath.Join(cwd, ".memory")

	// --- Detection ---
	fmt.Println("Detecting sources...")
	d := detectSources(cwd)

	// --- Determine source ---
	var source onboardSource

	if onboardFrom != "" {
		// Validate --from flag
		switch onboardSource(onboardFrom) {
		case sourceMarkdown, sourceBeads, sourceGithub, sourceFresh:
			source = onboardSource(onboardFrom)
		default:
			return fmt.Errorf("invalid --from value: %q (must be markdown, beads, github, or fresh)", onboardFrom)
		}
	} else if onboardDryRun {
		// Dry run without --from: show all detected sources (current behavior)
		printSourceSummary(d)
		fmt.Println("Dry run — no changes made.")
		if d.memoryMDPath != "" {
			data, _ := os.ReadFile(d.memoryMDPath)
			sections := parseSections(string(data))
			fmt.Printf("  Would create %d motes from %s\n", len(sections), filepath.Base(d.memoryMDPath))
		}
		importCount := d.openBeads
		if onboardIncludeClosed {
			importCount += d.closedBeads
		}
		if importCount > 0 {
			fmt.Printf("  Would import %d beads issues\n", importCount)
		}

		home, _ := os.UserHomeDir()
		claudeDir := filepath.Join(home, ".claude")
		if d.settingsHasBd {
			migrateClaudeSettings(claudeDir, true)
		}
		ensureClaudeHooks(claudeDir, true)
		if codexEnabled(home) {
			ensureCodexHooks(filepath.Join(home, ".codex"), true)
		}
		if geminiEnabled(home) {
			ensureGeminiSettings(filepath.Join(home, ".gemini"), true)
		}
		ensureMoteSkills(home, true)
		if onboardCleanup && dirExists(filepath.Join(cwd, ".beads")) {
			fmt.Println("  Would remove .beads/")
		}
		return nil
	} else {
		// Interactive: show summary and prompt
		printSourceSummary(d)
		opts := buildMenu(d)
		chosen, err := promptSelection(os.Stdin, opts)
		if err != nil {
			return err
		}
		source = chosen.source
	}

	// --- Scaffold .memory/ ---
	if !d.memoryDirExists {
		fmt.Println("Initializing .memory/...")
		if err := scaffoldMemoryDir(root); err != nil {
			return err
		}
	}

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	var totalCreated int

	// --- Execute selected migration ---
	switch source {
	case sourceMarkdown:
		if d.memoryMDPath == "" {
			return fmt.Errorf("no MEMORY.md found to migrate")
		}
		n, err := runMigrateMarkdown(mm, d.memoryMDPath)
		if err != nil {
			return err
		}
		totalCreated = n

	case sourceBeads:
		if len(d.beadsIssues) == 0 {
			return fmt.Errorf("no .beads/issues.jsonl found to migrate")
		}
		n, err := runMigrateBeads(mm, d.beadsIssues, onboardIncludeClosed)
		if err != nil {
			return err
		}
		totalCreated = n

	case sourceGithub:
		repo := onboardRepo
		if repo == "" {
			repo, err = promptRepo(os.Stdin)
			if err != nil {
				return err
			}
		}
		n, err := runImportGithub(mm, im, repo, onboardIncludeClosed, onboardPhaseParents)
		if err != nil {
			return err
		}
		totalCreated = n

	case sourceFresh:
		// No-op: just initialize and configure
	}

	// --- Common setup ---
	if err := runCommonSetup(cwd, root, mm, im, false); err != nil {
		return err
	}

	fmt.Printf("\nOnboarding complete: %d motes created.\n", totalCreated)

	// --- Cleanup ---
	if source == sourceBeads {
		beadsDir := filepath.Join(cwd, ".beads")
		if onboardCleanup && dirExists(beadsDir) {
			if err := os.RemoveAll(beadsDir); err != nil {
				fmt.Fprintf(os.Stderr, "warning: remove .beads/: %v\n", err)
			} else {
				fmt.Println("Removed .beads/")
			}
		} else if len(d.beadsIssues) > 0 && !onboardCleanup {
			fmt.Println(`
--- Manual steps ---
  - Remove .beads/ once you've verified the import (or use --cleanup)`)
		}
	}

	return nil
}

func runOnboardGlobal() error {
	gRoot, err := core.GlobalRoot()
	if err != nil {
		return err
	}

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
		fmt.Printf("  %s   exists\n", gRoot)
	} else {
		fmt.Printf("  %s   will create\n", gRoot)
	}

	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")
	settingsHasBd := settingsHasBdRefs(filepath.Join(claudeDir, "settings.json"))
	if settingsHasBd {
		fmt.Println("  settings.json          has bd references")
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
		if settingsHasBd {
			migrateClaudeSettings(claudeDir, true)
		}
		ensureClaudeHooks(claudeDir, true)
		if codexEnabled(home) {
			ensureCodexHooks(filepath.Join(home, ".codex"), true)
		}
		if geminiEnabled(home) {
			ensureGeminiSettings(filepath.Join(home, ".gemini"), true)
		}
		ensureMoteSkills(home, true)
		if onboardCleanup && dirExists(filepath.Join(home, ".beads")) {
			fmt.Println("  Would remove ~/.beads/")
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
				_ = mm.Update(m.ID, core.UpdateOpts{Status: core.StringPtr("completed")})
			}

			totalCreated++
			fmt.Printf("  created %s [%s] %s\n", m.ID, moteType, issue.Title)
		}
	}

	// --- Rebuild index ---
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	// --- Migrate settings.json hooks ---
	migrated, err := migrateClaudeSettings(claudeDir, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: settings migration: %v\n", err)
	} else if migrated > 0 {
		fmt.Printf("Migrated %d hook(s) in ~/.claude/settings.json\n", migrated)
	}

	// --- Install hooks ---
	if err := ensureClaudeHooks(claudeDir, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: hooks installation: %v\n", err)
	}

	// --- Install Codex hooks (when --codex set or ~/.codex/ exists) ---
	if codexEnabled(home) {
		if err := ensureCodexHooks(filepath.Join(home, ".codex"), false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: codex hooks installation: %v\n", err)
		}
	}

	// --- Install Gemini CLI settings (when --gemini set or ~/.gemini/ exists) ---
	if geminiEnabled(home) {
		if err := ensureGeminiSettings(filepath.Join(home, ".gemini"), false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: gemini settings installation: %v\n", err)
		}
	}

	// --- Install skills ---
	if err := ensureMoteSkills(home, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: skills installation: %v\n", err)
	}

	// --- Install per-agent global shims ---
	if err := ensureClaudeGlobalShim(home, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: claude shim: %v\n", err)
	}
	if codexEnabled(home) {
		if err := ensureCodexGlobalShim(home, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: codex shim: %v\n", err)
		}
	}
	if geminiEnabled(home) {
		if err := ensureGeminiGlobalShim(home, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: gemini shim: %v\n", err)
		}
	}

	// --- Seed MOTES.md index ---
	if err := core.GenerateMotesIndex(gRoot); err != nil {
		fmt.Fprintf(os.Stderr, "warning: MOTES.md generation: %v\n", err)
	}

	fmt.Printf("\nGlobal onboarding complete: %d motes created.\n", totalCreated)

	globalBeadsDir := filepath.Join(home, ".beads")
	if onboardCleanup && dirExists(globalBeadsDir) {
		if err := os.RemoveAll(globalBeadsDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: remove ~/.beads/: %v\n", err)
		} else {
			fmt.Println("Removed ~/.beads/")
		}
	} else if len(beadsIssues) > 0 && !onboardCleanup {
		fmt.Println(`
--- Manual steps ---
  - Remove ~/.beads/ once you've verified the import (or use --cleanup)`)
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

// settingsHasBdRefs checks if a settings.json file contains bd command references.
func settingsHasBdRefs(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "\"bd ")
}

// hookSpec defines a desired hook entry.
type hookSpec struct {
	event   string
	matcher string
	command string
}

// claudeAgentKindPrefix is prefixed to every Claude-installed mote hook command
// so that LoadConfig can apply per-agent provider overrides.
const claudeAgentKindPrefix = "MOTE_AGENT_KIND=claude "

// desiredHooks returns the full set of hooks mote should install.
func desiredHooks() []hookSpec {
	return []hookSpec{
		// Differentiated SessionStart modes
		{"SessionStart", "startup", claudeAgentKindPrefix + "mote prime --hook --mode=startup"},
		{"SessionStart", "resume", claudeAgentKindPrefix + "mote prime --hook --mode=resume"},
		{"SessionStart", "compact", claudeAgentKindPrefix + "mote prime --hook --mode=compact"},
		{"SessionStart", "clear", claudeAgentKindPrefix + "mote prime --hook --mode=startup"},
		// PreCompact stays as-is
		{"PreCompact", "", claudeAgentKindPrefix + "mote prime --hook --mode=compact"},
		// UserPromptSubmit for per-prompt context
		{"UserPromptSubmit", "", claudeAgentKindPrefix + "mote prompt-context"},
		// Stop hook for guaranteed session-end
		{"Stop", "", claudeAgentKindPrefix + "mote session-end --hook"},
	}
}

// ensureClaudeHooks installs SessionStart, PreCompact, and UserPromptSubmit hooks in settings.json.
// It migrates old catch-all matchers to differentiated ones and is idempotent.
func ensureClaudeHooks(claudeDir string, dryRun bool) error {
	settingsPath := filepath.Join(claudeDir, "settings.json")

	var settings map[string]interface{}

	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		settings = map[string]interface{}{}
	} else if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse settings.json: %w", err)
		}
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	// Migrate: remove old catch-all "mote prime" entries from SessionStart
	migrateOldHooks(hooks)

	var installed []string

	for _, spec := range desiredHooks() {
		if hookEventHasMatcherCommand(hooks, spec.event, spec.matcher, spec.command) {
			continue
		}

		entry := map[string]interface{}{
			"matcher": spec.matcher,
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": spec.command,
				},
			},
		}

		existing, _ := hooks[spec.event].([]interface{})
		hooks[spec.event] = append(existing, entry)
		installed = append(installed, fmt.Sprintf("%s[%s]", spec.event, spec.matcher))
	}

	if len(installed) == 0 {
		return nil
	}

	if dryRun {
		fmt.Printf("  Would install hooks: %s\n", strings.Join(installed, ", "))
		return nil
	}

	settings["hooks"] = hooks

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	newData = append(newData, '\n')

	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("create claude dir: %w", err)
	}
	if err := core.AtomicWrite(settingsPath, newData, 0644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}

	for _, name := range installed {
		fmt.Printf("  installed hook: %s\n", name)
	}
	return nil
}

// migrateOldHooks removes obsolete mote-managed hook entries so they can be
// replaced by current desiredHooks(). Two categories of obsolete entries:
//
//  1. Catch-all "mote prime" (no flags) — predates the differentiated mode
//     hooks. Stripped from SessionStart and PreCompact only.
//  2. Un-prefixed mote commands matching the old form of any currently-desired
//     hook — predates the MOTE_AGENT_KIND=claude prefix added for layered
//     config's per-agent overrides. Stripped across all events.
func migrateOldHooks(hooks map[string]interface{}) {
	// Collect the un-prefixed forms of currently-desired commands so we can
	// recognize and strip them.
	unprefixedDesired := map[string]bool{}
	for _, spec := range desiredHooks() {
		unprefixed := strings.TrimPrefix(spec.command, claudeAgentKindPrefix)
		if unprefixed != spec.command {
			unprefixedDesired[unprefixed] = true
		}
	}

	isObsolete := func(cmd string) bool {
		if cmd == "mote prime" {
			return true
		}
		return unprefixedDesired[cmd]
	}

	// Iterate every event so the prefix migration covers UserPromptSubmit and
	// Stop too (not just SessionStart/PreCompact).
	for eventName, raw := range hooks {
		entries, ok := raw.([]interface{})
		if !ok {
			continue
		}
		var kept []interface{}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				kept = append(kept, entry)
				continue
			}
			hooksList, ok := entryMap["hooks"].([]interface{})
			if !ok {
				kept = append(kept, entry)
				continue
			}
			drop := false
			for _, h := range hooksList {
				hMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, _ := hMap["command"].(string)
				if isObsolete(cmd) {
					drop = true
					break
				}
			}
			if !drop {
				kept = append(kept, entry)
			}
		}
		if len(kept) == 0 {
			delete(hooks, eventName)
		} else {
			hooks[eventName] = kept
		}
	}
}

// hookEventHasCommand checks if a hook event already contains a hook with the given command.
func hookEventHasCommand(hooks map[string]interface{}, eventName, command string) bool {
	entries, ok := hooks[eventName].([]interface{})
	if !ok {
		return false
	}
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		hooksList, ok := entryMap["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hMap, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, ok := hMap["command"].(string); ok && cmd == command {
				return true
			}
		}
	}
	return false
}

// hookEventHasMatcherCommand checks if a hook event has an entry with both the given matcher and command.
func hookEventHasMatcherCommand(hooks map[string]interface{}, eventName, matcher, command string) bool {
	entries, ok := hooks[eventName].([]interface{})
	if !ok {
		return false
	}
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		entryMatcher, _ := entryMap["matcher"].(string)
		if entryMatcher != matcher {
			continue
		}
		hooksList, ok := entryMap["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hMap, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, ok := hMap["command"].(string); ok && cmd == command {
				return true
			}
		}
	}
	return false
}

// codexAgentKindPrefix is prefixed to every Codex-installed mote hook command
// so that LoadConfig can apply per-agent provider overrides keyed on
// MOTE_AGENT_KIND=codex.
const codexAgentKindPrefix = "MOTE_AGENT_KIND=codex "

// desiredCodexHooks returns the hook entries Codex should install.
//
// Codex's SessionStart matchers are startup, resume, clear (no separate
// compact event — Codex doesn't fire one), so a single combined matcher
// covers all three. UserPromptSubmit and Stop ignore matcher entirely per
// the Codex spec. The Stop hook gets an explicit 600s timeout to make the
// observed worst-case session-end runtime visible to anyone reading the file.
func desiredCodexHooks() []codexHookSpec {
	return []codexHookSpec{
		{event: "SessionStart", matcher: "startup|resume|clear", command: codexAgentKindPrefix + "mote prime --hook --mode=startup", statusMessage: "Loading mote context"},
		{event: "UserPromptSubmit", matcher: "", command: codexAgentKindPrefix + "mote prompt-context"},
		{event: "Stop", matcher: "", command: codexAgentKindPrefix + "mote session-end --hook", timeout: 600, statusMessage: "Flushing mote session state"},
	}
}

// codexHookSpec extends hookSpec with Codex-only fields (timeout, statusMessage).
// Kept separate from hookSpec to avoid leaking Codex concepts into the Claude
// install path; the two ecosystems use different JSON shapes for these fields.
type codexHookSpec struct {
	event         string
	matcher       string
	command       string
	timeout       int    // seconds; 0 means rely on Codex's 600s default
	statusMessage string // optional; surfaced in Codex's UI while the hook runs
}

// ensureCodexHooks installs Codex hooks in ~/.codex/hooks.json. Idempotent —
// matches the Claude install path's behavior: existing matching entries are
// left alone, missing ones are appended. Also writes the codex_hooks feature
// flag to ~/.codex/config.toml when that file is missing or doesn't yet have
// a [features] section. If [features] exists, prints a one-line advisory
// rather than risk corrupting arbitrary user TOML.
func ensureCodexHooks(codexDir string, dryRun bool) error {
	if err := ensureCodexFeatureFlag(codexDir, dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "warning: codex feature flag: %v\n", err)
	}

	hooksPath := filepath.Join(codexDir, "hooks.json")

	var settings map[string]interface{}
	data, err := os.ReadFile(hooksPath)
	if os.IsNotExist(err) {
		settings = map[string]interface{}{}
	} else if err != nil {
		return fmt.Errorf("read codex hooks.json: %w", err)
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse codex hooks.json: %w", err)
		}
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	var installed []string
	for _, spec := range desiredCodexHooks() {
		if hookEventHasMatcherCommand(hooks, spec.event, spec.matcher, spec.command) {
			continue
		}

		hookCmd := map[string]interface{}{
			"type":    "command",
			"command": spec.command,
		}
		if spec.timeout > 0 {
			hookCmd["timeout"] = spec.timeout
		}
		if spec.statusMessage != "" {
			hookCmd["statusMessage"] = spec.statusMessage
		}

		entry := map[string]interface{}{
			"hooks": []interface{}{hookCmd},
		}
		if spec.matcher != "" {
			entry["matcher"] = spec.matcher
		}

		existing, _ := hooks[spec.event].([]interface{})
		hooks[spec.event] = append(existing, entry)
		installed = append(installed, fmt.Sprintf("%s[%s]", spec.event, spec.matcher))
	}

	if len(installed) == 0 {
		return nil
	}

	if dryRun {
		fmt.Printf("  Would install codex hooks: %s\n", strings.Join(installed, ", "))
		return nil
	}

	settings["hooks"] = hooks
	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal codex hooks: %w", err)
	}
	newData = append(newData, '\n')

	if err := os.MkdirAll(codexDir, 0755); err != nil {
		return fmt.Errorf("create codex dir: %w", err)
	}
	if err := core.AtomicWrite(hooksPath, newData, 0644); err != nil {
		return fmt.Errorf("write codex hooks.json: %w", err)
	}
	for _, name := range installed {
		fmt.Printf("  installed codex hook: %s\n", name)
	}
	return nil
}

// ensureCodexFeatureFlag enables `codex_hooks = true` in ~/.codex/config.toml.
// Codex ignores hooks unless this flag is set. To avoid clobbering arbitrary
// user TOML structure, we only write when the file is missing or has no
// [features] section. Otherwise we emit an advisory and let the user merge
// the flag themselves.
func ensureCodexFeatureFlag(codexDir string, dryRun bool) error {
	configPath := filepath.Join(codexDir, "config.toml")
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		if dryRun {
			fmt.Printf("  Would create %s with codex_hooks = true\n", configPath)
			return nil
		}
		if err := os.MkdirAll(codexDir, 0755); err != nil {
			return fmt.Errorf("create codex dir: %w", err)
		}
		body := "[features]\ncodex_hooks = true\n"
		if err := core.AtomicWrite(configPath, []byte(body), 0644); err != nil {
			return fmt.Errorf("write codex config.toml: %w", err)
		}
		fmt.Printf("  created %s with codex_hooks = true\n", configPath)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read codex config.toml: %w", err)
	}

	if strings.Contains(string(data), "codex_hooks") {
		return nil
	}
	if strings.Contains(string(data), "[features]") {
		fmt.Fprintf(os.Stderr,
			"  ⚠ ~/.codex/config.toml has [features] but no codex_hooks key — add `codex_hooks = true` manually for hooks to fire\n")
		return nil
	}

	if dryRun {
		fmt.Printf("  Would append [features]\\ncodex_hooks = true to %s\n", configPath)
		return nil
	}
	appended := string(data)
	if !strings.HasSuffix(appended, "\n") {
		appended += "\n"
	}
	appended += "\n[features]\ncodex_hooks = true\n"
	if err := core.AtomicWrite(configPath, []byte(appended), 0644); err != nil {
		return fmt.Errorf("write codex config.toml: %w", err)
	}
	fmt.Printf("  appended codex_hooks = true to %s\n", configPath)
	return nil
}

// geminiHookSpec carries the Gemini CLI hook fields. Distinct from hookSpec
// (Claude) and codexHookSpec because Gemini's timeouts are in milliseconds
// rather than seconds, and the spec includes a `name` field for /hooks panel
// management. Keeping the type separate avoids leaking units across ecosystems.
type geminiHookSpec struct {
	event     string
	matcher   string
	name      string
	command   string
	timeoutMs int
}

// geminiAgentKindPrefix is prefixed to every Gemini-installed mote hook command
// so that LoadConfig can apply per-agent provider overrides keyed on
// MOTE_AGENT_KIND=gemini.
const geminiAgentKindPrefix = "MOTE_AGENT_KIND=gemini "

// desiredGeminiHooks returns the hook entries Gemini CLI should install.
//
// Three differences from desiredCodexHooks:
//   - BeforeAgent (Gemini's name for the user-prompt-submit lifecycle moment)
//     instead of UserPromptSubmit.
//   - SessionEnd (per-session) instead of Stop (per-turn). Mote's session-end
//     does heavy work; firing once per session is preferable.
//   - Timeouts in milliseconds. Gemini's default is 60000ms; we set 300000ms
//     on SessionEnd because the flush regularly takes 2-3 minutes.
func desiredGeminiHooks() []geminiHookSpec {
	return []geminiHookSpec{
		{event: "SessionStart", matcher: "startup|resume|clear",
			name: "mote-prime", command: geminiAgentKindPrefix + "mote prime --hook --mode=startup", timeoutMs: 60000},
		{event: "BeforeAgent", matcher: "",
			name: "mote-prompt-context", command: geminiAgentKindPrefix + "mote prompt-context", timeoutMs: 30000},
		{event: "SessionEnd", matcher: "",
			name: "mote-session-end", command: geminiAgentKindPrefix + "mote session-end --hook", timeoutMs: 300000},
	}
}

// ensureGeminiSettings installs hooks and configures context.fileName in
// ~/.gemini/settings.json. Idempotent: existing hook entries are left alone,
// missing ones are appended. context.fileName is augmented to include
// GEMINI.md and AGENTS.md without removing any user-defined entries.
//
// Gemini CLI uses settings.json (like Claude), not a separate hooks.json
// (like Codex), so this function merges into a JSON file with multiple
// top-level keys.
func ensureGeminiSettings(geminiDir string, dryRun bool) error {
	settingsPath := filepath.Join(geminiDir, "settings.json")

	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		settings = map[string]interface{}{}
	} else if err != nil {
		return fmt.Errorf("read gemini settings.json: %w", err)
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse gemini settings.json: %w", err)
		}
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	var installed []string
	for _, spec := range desiredGeminiHooks() {
		if hookEventHasMatcherCommand(hooks, spec.event, spec.matcher, spec.command) {
			continue
		}
		hookCmd := map[string]interface{}{
			"name":    spec.name,
			"type":    "command",
			"command": spec.command,
			"timeout": spec.timeoutMs,
		}
		entry := map[string]interface{}{
			"hooks": []interface{}{hookCmd},
		}
		if spec.matcher != "" {
			entry["matcher"] = spec.matcher
		}
		existing, _ := hooks[spec.event].([]interface{})
		hooks[spec.event] = append(existing, entry)
		installed = append(installed, fmt.Sprintf("%s[%s]", spec.event, spec.matcher))
	}

	// Merge context.fileName — preserve user entries; ensure GEMINI.md and
	// AGENTS.md are present.
	ctx, _ := settings["context"].(map[string]interface{})
	if ctx == nil {
		ctx = map[string]interface{}{}
	}
	fileNames, _ := ctx["fileName"].([]interface{})
	contextChanged := false
	for _, want := range []string{"GEMINI.md", "AGENTS.md"} {
		if !stringInArray(fileNames, want) {
			fileNames = append(fileNames, want)
			contextChanged = true
		}
	}
	if contextChanged {
		ctx["fileName"] = fileNames
	}

	if len(installed) == 0 && !contextChanged {
		return nil
	}

	if dryRun {
		if len(installed) > 0 {
			fmt.Printf("  Would install gemini hooks: %s\n", strings.Join(installed, ", "))
		}
		if contextChanged {
			fmt.Printf("  Would configure gemini context.fileName: %v\n", fileNames)
		}
		return nil
	}

	settings["hooks"] = hooks
	settings["context"] = ctx

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal gemini settings: %w", err)
	}
	newData = append(newData, '\n')

	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		return fmt.Errorf("create gemini dir: %w", err)
	}
	if err := core.AtomicWrite(settingsPath, newData, 0644); err != nil {
		return fmt.Errorf("write gemini settings.json: %w", err)
	}
	for _, name := range installed {
		fmt.Printf("  installed gemini hook: %s\n", name)
	}
	if contextChanged {
		fmt.Printf("  configured gemini context.fileName: %v\n", fileNames)
	}
	return nil
}

// stringInArray returns true if needle (a string) appears in arr. Used for
// idempotency on context.fileName entries. Case-sensitive — Gemini CLI
// treats filenames case-sensitively on case-sensitive filesystems.
func stringInArray(arr []interface{}, needle string) bool {
	for _, v := range arr {
		if s, ok := v.(string); ok && s == needle {
			return true
		}
	}
	return false
}

// ensureMoteSkills installs mote skill files at every active skills location.
// Always writes to ~/.claude/skills/ (Claude Code). When either Codex or
// Gemini CLI is enabled (--codex, --gemini, or auto-detect of ~/.codex/ or
// ~/.gemini/), also writes to ~/.agents/skills/ — the path both tools scan
// per the open agent skills standard.
func ensureMoteSkills(homeDir string, dryRun bool) error {
	targets := []string{filepath.Join(homeDir, ".claude", "skills")}
	if agentsSkillsEnabled(homeDir) {
		targets = append(targets, filepath.Join(homeDir, ".agents", "skills"))
	}
	for _, dir := range targets {
		if err := installSkillsAt(dir, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// installSkillsAt writes the bundled mote skills under skillsDir. Idempotent:
// skips files whose content is already up to date. Used for both the Claude
// (~/.claude/skills) and Codex (~/.agents/skills) install paths.
func installSkillsAt(skillsDir string, dryRun bool) error {
	type skillDef struct {
		name    string
		content []byte
	}
	defs := []skillDef{
		{"mote-capture", skills.MoteCapture},
		{"mote-retrieve", skills.MoteRetrieve},
		{"mote-subagent", skills.MoteSubagent},
		{"mote-plan", skills.MotePlan},
	}

	for _, s := range defs {
		targetDir := filepath.Join(skillsDir, s.name)
		targetFile := filepath.Join(targetDir, "SKILL.md")

		existing, err := os.ReadFile(targetFile)
		if err == nil && bytes.Equal(existing, s.content) {
			continue
		}

		action := "installed"
		if err == nil {
			action = "updated"
		}

		if dryRun {
			fmt.Printf("  Would install skill: %s (%s)\n", s.name, skillsDir)
			continue
		}

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("create skill dir %s: %w", s.name, err)
		}
		if err := core.AtomicWrite(targetFile, s.content, 0644); err != nil {
			return fmt.Errorf("write skill %s: %w", s.name, err)
		}
		fmt.Printf("  %s skill: %s (%s)\n", action, s.name, skillsDir)
	}

	return nil
}

// migrateClaudeSettings detects and migrates bd references in settings.json hooks.
// claudeDir is the directory containing settings.json (e.g. ~/.claude/).
// Returns the number of hooks migrated.
func migrateClaudeSettings(claudeDir string, dryRun bool) (int, error) {
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read settings.json: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return 0, fmt.Errorf("parse settings.json: %w", err)
	}

	hooksRaw, ok := settings["hooks"]
	if !ok {
		return 0, nil
	}
	hooks, ok := hooksRaw.(map[string]interface{})
	if !ok {
		return 0, nil
	}

	cmdMap := map[string]string{
		"bd prime": "mote prime",
		"bd sync":  "mote session-end",
	}

	migrated := 0

	for eventName, eventVal := range hooks {
		entries, ok := eventVal.([]interface{})
		if !ok {
			continue
		}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			hooksList, ok := entryMap["hooks"].([]interface{})
			if !ok {
				continue
			}
			for _, h := range hooksList {
				hMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, ok := hMap["command"].(string)
				if !ok {
					continue
				}
				if replacement, found := cmdMap[cmd]; found {
					fmt.Printf("  Migrated hook: %s → %s (%s)\n", cmd, replacement, eventName)
					if !dryRun {
						hMap["command"] = replacement
					}
					migrated++
				} else if strings.HasPrefix(cmd, "bd ") {
					fmt.Printf("  Warning: unknown bd command %q in %s — manual migration needed\n", cmd, eventName)
				}
			}
		}
	}

	if migrated > 0 && !dryRun {
		newData, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return migrated, fmt.Errorf("marshal settings: %w", err)
		}
		newData = append(newData, '\n')
		if err := core.AtomicWrite(settingsPath, newData, 0644); err != nil {
			return migrated, fmt.Errorf("write settings.json: %w", err)
		}
	}

	// Check for stale permissions in settings.local.json
	localSettingsPath := filepath.Join(claudeDir, "settings.local.json")
	if localData, err := os.ReadFile(localSettingsPath); err == nil {
		if strings.Contains(string(localData), "\"bd ") {
			fmt.Println("  Note: settings.local.json contains stale bd permissions — clean up manually if desired")
		}
	}

	return migrated, nil
}

const preCommitMarker = "# mote pre-commit hook"

const preCommitScript = `#!/bin/sh
# mote pre-commit hook
# Soft warning if no active task mote exists. Always exits 0.
if [ -d ".memory/nodes" ] && command -v mote >/dev/null 2>&1; then
    count=$(mote ls --type=task --status=active --compact 2>/dev/null | grep -c . 2>/dev/null || echo 0)
    if [ "$count" -eq 0 ]; then
        echo "mote: no active task found. Consider: mote add --type=task --title=\"...\"" >&2
    fi
fi
exit 0
`

// ensurePreCommitHook installs a soft-warning pre-commit hook in .git/hooks/pre-commit.
// It is idempotent: if the hook already contains the mote marker it is skipped.
// If an existing hook exists without the marker, the mote script is appended.
func ensurePreCommitHook(projectRoot string, dryRun bool) error {
	gitDir := filepath.Join(projectRoot, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return nil // not a git repo, skip silently
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")

	existing, readErr := os.ReadFile(hookPath)
	if readErr == nil && strings.Contains(string(existing), preCommitMarker) {
		return nil // already installed
	}

	if dryRun {
		fmt.Println("  Would install pre-commit hook (soft task warning)")
		return nil
	}

	if err := os.MkdirAll(filepath.Join(gitDir, "hooks"), 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	var content string
	if readErr == nil && len(existing) > 0 {
		// Append to existing hook
		content = strings.TrimRight(string(existing), "\n") + "\n\n" + preCommitScript
	} else {
		content = preCommitScript
	}

	if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("write pre-commit hook: %w", err)
	}

	fmt.Println("  installed pre-commit hook (soft task warning)")
	return nil
}
