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
)

func init() {
	onboardCmd.Flags().BoolVar(&onboardGlobal, "global", false, "Onboard the global layer (~/.claude/memory/)")
	onboardCmd.Flags().BoolVar(&onboardDryRun, "dry-run", false, "Show what would happen without writing")
	onboardCmd.Flags().BoolVar(&onboardIncludeClosed, "include-closed", false, "Also import closed beads/github issues (default: open only)")
	onboardCmd.Flags().BoolVar(&onboardCleanup, "cleanup", false, "Remove .beads/ after successful import")
	onboardCmd.Flags().StringVar(&onboardFrom, "from", "", "Migration source: markdown, beads, github, fresh (skips interactive prompt)")
	onboardCmd.Flags().StringVar(&onboardRepo, "repo", "", "GitHub repo (owner/repo) for --from=github")
	onboardCmd.Flags().BoolVar(&onboardPhaseParents, "phase-parents", false, "Create parent task motes per phase label (github import)")
	rootCmd.AddCommand(onboardCmd)
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
		ensureMoteSkills(home, true)
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

	// Install skills
	if err := ensureMoteSkills(home, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: skills installation: %v\n", err)
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

	// --- Install skills ---
	if err := ensureMoteSkills(home, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: skills installation: %v\n", err)
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

// desiredHooks returns the full set of hooks mote should install.
func desiredHooks() []hookSpec {
	return []hookSpec{
		// Differentiated SessionStart modes
		{"SessionStart", "startup", "mote prime --hook --mode=startup"},
		{"SessionStart", "resume", "mote prime --hook --mode=resume"},
		{"SessionStart", "compact", "mote prime --hook --mode=compact"},
		{"SessionStart", "clear", "mote prime --hook --mode=startup"},
		// PreCompact stays as-is
		{"PreCompact", "", "mote prime --hook --mode=compact"},
		// UserPromptSubmit for per-prompt context
		{"UserPromptSubmit", "", "mote prompt-context"},
		// Stop hook for guaranteed session-end
		{"Stop", "", "mote session-end --hook"},
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

// migrateOldHooks removes old catch-all "mote prime" (without --hook) entries
// from SessionStart and PreCompact so they can be replaced by differentiated hooks.
func migrateOldHooks(hooks map[string]interface{}) {
	for _, eventName := range []string{"SessionStart", "PreCompact"} {
		entries, ok := hooks[eventName].([]interface{})
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
			isOld := false
			for _, h := range hooksList {
				hMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, _ := hMap["command"].(string)
				if cmd == "mote prime" {
					isOld = true
					break
				}
			}
			if !isOld {
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

// ensureMoteSkills installs mote skill files to ~/.claude/skills/.
// Updates existing files if content has changed.
func ensureMoteSkills(homeDir string, dryRun bool) error {
	skillsDir := filepath.Join(homeDir, ".claude", "skills")

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
			fmt.Printf("  Would install skill: %s\n", s.name)
			continue
		}

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("create skill dir %s: %w", s.name, err)
		}
		if err := core.AtomicWrite(targetFile, s.content, 0644); err != nil {
			return fmt.Errorf("write skill %s: %w", s.name, err)
		}
		fmt.Printf("  %s skill: %s\n", action, s.name)
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
