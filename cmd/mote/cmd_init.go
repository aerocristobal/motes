package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .memory/ directory",
	Long:  "Creates the full .memory/ directory structure with config, index files, and optionally updates CLAUDE.md.",
	RunE:  runInit,
}

var initGlobal bool

func init() {
	initCmd.Flags().BoolVar(&initGlobal, "global", false, "Initialize global memory at ~/.claude/memory/")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if initGlobal {
		return runInitGlobal()
	}
	return runInitProject()
}

func runInitProject() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root := filepath.Join(cwd, ".memory")
	fmt.Printf("Initialized .memory/ in %s\n", cwd)

	// Directories
	for _, dir := range []string{"nodes", "dream", "strata"} {
		created, err := ensureDir(filepath.Join(root, dir))
		if err != nil {
			return err
		}
		printStatus(created, dir+"/")
	}

	// config.yaml (never overwrite)
	configPath := filepath.Join(root, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := core.SaveConfig(root, core.DefaultConfig()); err != nil {
			return err
		}
		printStatus(true, "config.yaml")
	} else {
		printStatus(false, "config.yaml")
	}

	// Empty JSONL files
	jsonlFiles := []struct {
		path string
		name string
	}{
		{filepath.Join(root, "index.jsonl"), "index.jsonl"},
		{filepath.Join(root, "constellations.jsonl"), "constellations.jsonl"},
		{filepath.Join(root, "dream", "log.jsonl"), "dream/log.jsonl"},
		{filepath.Join(root, "strata", "query_log.jsonl"), "strata/query_log.jsonl"},
	}
	for _, f := range jsonlFiles {
		created, err := createFileIfAbsent(f.path, nil)
		if err != nil {
			return err
		}
		printStatus(created, f.name)
	}

	// CLAUDE.md
	created, err := ensureClaudeMD(cwd)
	if err != nil {
		return err
	}
	printStatus(created, "CLAUDE.md")

	// Install hooks and skills
	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := ensureClaudeHooks(claudeDir, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: hooks installation: %v\n", err)
	}
	if err := ensureMoteSkills(home, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: skills installation: %v\n", err)
	}

	return nil
}

func runInitGlobal() error {
	root := globalRoot()
	fmt.Printf("Initialized global memory at %s\n", root)

	created, err := ensureDir(filepath.Join(root, "nodes"))
	if err != nil {
		return err
	}
	printStatus(created, "nodes/")

	return nil
}

// ensureDir creates the directory if absent. Returns true if created.
func ensureDir(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	return true, nil
}

// createFileIfAbsent creates the file with data if it doesn't exist. Returns true if created.
func createFileIfAbsent(path string, data []byte) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if data == nil {
		data = []byte{}
	}
	if err := core.AtomicWrite(path, data, 0644); err != nil {
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	return true, nil
}

// printStatus prints a "created" or "exists" line for the given name.
func printStatus(created bool, name string) {
	if created {
		fmt.Printf("  created  %s\n", name)
	} else {
		fmt.Printf("  exists   %s\n", name)
	}
}

const motesSection = `## Motes

This project uses motes for all planning, memory, and task tracking. Knowledge is stored in ` + "`.memory/`" + `.

**Do NOT use** markdown files, TodoWrite, TaskCreate, or external issue trackers for tracking work.

### Session Start

` + "***Run `mote prime` at the start of every session for scored, relevant context.***" + `

Prime outputs: active tasks, recent decisions, lessons, explores, echoes, and contradiction alerts. It auto-parses your git branch as keywords.

Focus priming on a topic: ` + "`mote prime <topic>`" + `
Inspect a surfaced mote: ` + "`mote show <id>`" + `

### Mid-Session Retrieval

When you need context beyond what prime surfaced:

| Need | Command | Example |
|------|---------|---------|
| Graph traversal | ` + "`mote context <topic>`" + ` | ` + "`mote context authentication`" + ` |
| Full-text search | ` + "`mote search <query>`" + ` | ` + "`mote search \"retry logic\"`" + ` |
| Reference docs | ` + "`mote strata query <topic>`" + ` | ` + "`mote strata query scoring`" + ` |
| Dependency chain view | ` + "`mote context --planning <id>`" + ` | ` + "`mote context --planning proj-t1abc`" + ` |

### Task Tracking & Planning

Find available work:

    mote ls --ready           # Tasks with no unfinished blockers
    mote pulse                # Active tasks sorted by weight

Create tasks with dependency links:

    mote add --type=task --title="Summary" --tag=topic --body "What and why"
    mote link <story-id> depends_on <epic-id>
    mote update <id> --status=completed

### Capturing Knowledge

Capture when you encounter:

| Trigger | Type | Command |
|---------|------|---------|
| Non-obvious choice made | decision | ` + "`mote add --type=decision --title=\"Summary\" --tag=topic --body \"Rationale\"`" + ` |
| Gotcha or surprise discovered | lesson | ` + "`mote add --type=lesson --title=\"Summary\" --tag=topic --body \"Details\"`" + ` |
| Researched alternatives | explore | ` + "`mote add --type=explore --title=\"Summary\" --tag=topic --body \"Findings\"`" + ` |
| Quick thought | (auto) | ` + "`mote quick \"your sentence here\"`" + ` |

After capturing, link related motes: ` + "`mote link <id1> relates_to <id2>`" + `
Give feedback on surfaced motes: ` + "`mote feedback <id> useful`" + ` or ` + "`mote feedback <id> irrelevant`" + `

**Tag strategy:** Rare, specific tags beat generic ones.

### Session End

Run ` + "`mote session-end`" + ` for access flush and maintenance suggestions.

Run ` + "`mote dream`" + ` periodically for automated maintenance. Review with ` + "`mote dream --review`" + `.
`

// ensureClaudeMD creates or appends the motes section to CLAUDE.md.
// Returns true if the file was created or modified, false if section already exists.
func ensureClaudeMD(projectDir string) (bool, error) {
	claudePath := filepath.Join(projectDir, "CLAUDE.md")

	data, err := os.ReadFile(claudePath)
	if os.IsNotExist(err) {
		// Create new file with just the motes section
		if err := core.AtomicWrite(claudePath, []byte(motesSection), 0644); err != nil {
			return false, fmt.Errorf("create CLAUDE.md: %w", err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read CLAUDE.md: %w", err)
	}

	// Check if motes section already exists
	if strings.Contains(string(data), "## Motes") {
		return false, nil
	}

	// Append motes section
	content := string(data)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + motesSection

	if err := core.AtomicWrite(claudePath, []byte(content), 0644); err != nil {
		return false, fmt.Errorf("update CLAUDE.md: %w", err)
	}
	return true, nil
}
