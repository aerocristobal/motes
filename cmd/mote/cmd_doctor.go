// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate graph integrity and surface issues",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().Bool("cross-project", false, "validate cross-project refs by loading all discovered projects under --projects-root")
	doctorCmd.Flags().String("projects-root", "", "root directory to scan for sibling projects (default: parent of current project)")
}

// extractMotePrefix returns the project prefix of a mote ID (e.g. "turingpi" from "turingpi-Txxx").
func extractMotePrefix(id string) string {
	if i := strings.IndexByte(id, '-'); i > 0 {
		return id[:i]
	}
	return ""
}

// discoverProjectMotes loads motes from all sibling projects under projectsRoot,
// excluding the project at currentMemRoot.
func discoverProjectMotes(projectsRoot, currentMemRoot string) ([]*core.Mote, error) {
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil, fmt.Errorf("read projects root %s: %w", projectsRoot, err)
	}
	var all []*core.Mote
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projDir := filepath.Join(projectsRoot, e.Name())
		if _, err := os.Stat(filepath.Join(projDir, ".memory", "nodes")); err != nil {
			continue
		}
		if filepath.Clean(filepath.Join(projDir, ".memory")) == filepath.Clean(currentMemRoot) {
			continue
		}
		pm := core.NewMoteManager(filepath.Join(projDir, ".memory"))
		motes, err := pm.ReadAllParallel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cross-project: skipping %s: %v\n", projDir, err)
			continue
		}
		all = append(all, motes...)
	}
	return all, nil
}

type doctorIssue struct {
	Category string
	MoteID   string
	Detail   string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	motes, err := readAllWithGlobal(mm)
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	// --cross-project: load motes from all sibling projects to validate cross-project refs.
	crossProject, _ := cmd.Flags().GetBool("cross-project")
	if crossProject {
		projectsRoot, _ := cmd.Flags().GetString("projects-root")
		if projectsRoot == "" {
			// Default: grandparent of .memory dir (e.g. ~/projects/ when root is ~/projects/myapp/.memory)
			projectsRoot = filepath.Dir(filepath.Dir(root))
		}
		extra, err := discoverProjectMotes(projectsRoot, root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cross-project scan: %v\n", err)
		}
		for _, m := range extra {
			if _, exists := moteMap[m.ID]; !exists {
				moteMap[m.ID] = m
			}
		}
	}

	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)
	advisories := runDoctorAdvisories(idx, moteMap, cfg)
	advisories = append(advisories, runDoctorProviderAdvisories(cfg)...)

	// Separate cross_project_ref advisories from integrity errors.
	var errorIssues, crossRefs []doctorIssue
	for _, iss := range issues {
		if iss.Category == "cross_project_ref" {
			crossRefs = append(crossRefs, iss)
		} else {
			errorIssues = append(errorIssues, iss)
		}
	}

	if len(errorIssues) == 0 && len(crossRefs) == 0 {
		fmt.Println("No issues found. Graph is healthy.")
	} else {
		if len(errorIssues) > 0 {
			fmt.Printf("%-16s  %-26s  %s\n", "ISSUE", "MOTE", "DETAIL")
			fmt.Println(strings.Repeat("-", 80))
			for _, iss := range errorIssues {
				fmt.Printf("%-16s  %-26s  %s\n", iss.Category, iss.MoteID, iss.Detail)
			}
			fmt.Printf("\n%d issue(s) found.\n", len(errorIssues))
		} else {
			fmt.Println("No integrity issues found.")
		}

		if len(crossRefs) > 0 {
			// Show a compact summary grouped by target project prefix.
			sourceMotes := map[string]bool{}
			targetPrefixCounts := map[string]int{}
			for _, iss := range crossRefs {
				sourceMotes[iss.MoteID] = true
				// Detail format: "<linkType> target <targetID> is in another project..."
				fields := strings.Fields(iss.Detail)
				for i, f := range fields {
					if f == "target" && i+1 < len(fields) {
						if p := extractMotePrefix(fields[i+1]); p != "" {
							targetPrefixCounts[p]++
						}
						break
					}
				}
			}
			prefixes := make([]string, 0, len(targetPrefixCounts))
			for p := range targetPrefixCounts {
				prefixes = append(prefixes, p)
			}
			sort.Strings(prefixes)
			fmt.Printf("\n%d cross-project reference(s) in %d mote(s) (advisory):\n", len(crossRefs), len(sourceMotes))
			for _, p := range prefixes {
				fmt.Printf("  %s: %d ref(s)\n", p, targetPrefixCounts[p])
			}
			fmt.Println("  Run `mote doctor --cross-project` to validate these references.")
		}
	}

	if len(advisories) > 0 {
		fmt.Println()
		for _, w := range advisories {
			fmt.Printf("⚠ %s\n", w)
		}
	}

	if len(errorIssues) > 0 {
		os.Exit(1)
	}
	return nil
}

func runDoctorChecks(mm *core.MoteManager, im *core.IndexManager, idx *core.EdgeIndex, moteMap map[string]*core.Mote, cfg *core.Config) []doctorIssue {
	var issues []doctorIssue

	// Build the set of project prefixes present in the loaded moteMap.
	// A broken link whose target prefix is absent from this set is a cross-project
	// reference — valid from its origin project but not resolvable in the current context.
	knownPrefixes := make(map[string]bool, 4)
	for id := range moteMap {
		if p := extractMotePrefix(id); p != "" {
			knownPrefixes[p] = true
		}
	}

	// Collect motes as a slice for ordered iteration
	var motes []*core.Mote
	for _, m := range moteMap {
		motes = append(motes, m)
	}

	for _, m := range motes {
		// Broken links in frontmatter fields
		allLinks := collectAllLinks(m)
		for linkType, targets := range allLinks {
			for _, target := range targets {
				if _, ok := moteMap[target]; !ok {
					prefix := extractMotePrefix(target)
					if prefix != "" && !knownPrefixes[prefix] {
						// Target belongs to a project not loaded in this context.
						// Report as advisory rather than an integrity error.
						issues = append(issues, doctorIssue{
							Category: "cross_project_ref",
							MoteID:   m.ID,
							Detail:   fmt.Sprintf("%s target %s is in another project (run --cross-project to validate)", linkType, target),
						})
					} else {
						issues = append(issues, doctorIssue{
							Category: "broken_link",
							MoteID:   m.ID,
							Detail:   fmt.Sprintf("%s target %s does not exist", linkType, target),
						})
					}
				}
			}
		}

		// Isolated motes (no edges in or out)
		if !idx.HasEdges(m.ID) {
			issues = append(issues, doctorIssue{
				Category: "isolated",
				MoteID:   m.ID,
				Detail:   "no incoming or outgoing edges",
			})
		}

		// Deprecated blockers: live motes depending on deprecated motes
		if core.IsLive(m.Status) {
			for _, depID := range m.DependsOn {
				if dep, ok := moteMap[depID]; ok && dep.Status == "deprecated" {
					issues = append(issues, doctorIssue{
						Category: "deprecated_dep",
						MoteID:   m.ID,
						Detail:   fmt.Sprintf("depends_on deprecated mote %s", depID),
					})
				}
			}
		}

		// Stale motes: live with last_accessed > threshold days or nil
		stalenessThreshold := cfg.Dream.PreScan.StalenessThresholdDays
		if stalenessThreshold <= 0 {
			stalenessThreshold = 180
		}
		if core.IsLive(m.Status) {
			if m.LastAccessed == nil {
				issues = append(issues, doctorIssue{
					Category: "stale",
					MoteID:   m.ID,
					Detail:   "never accessed",
				})
			} else if time.Since(*m.LastAccessed).Hours()/24 > float64(stalenessThreshold) {
				days := int(time.Since(*m.LastAccessed).Hours() / 24)
				issues = append(issues, doctorIssue{
					Category: "stale",
					MoteID:   m.ID,
					Detail:   fmt.Sprintf("last accessed %d days ago", days),
				})
			}
		}
	}

	// Test/scratch motes: live motes with test-like titles
	testPrefixes := []string{"test ", "access test", "tag test", "scratch "}
	for _, m := range motes {
		if !core.IsLive(m.Status) {
			continue
		}
		lower := strings.ToLower(m.Title)
		for _, prefix := range testPrefixes {
			if strings.HasPrefix(lower, prefix) {
				issues = append(issues, doctorIssue{
					Category: "test_mote",
					MoteID:   m.ID,
					Detail:   fmt.Sprintf("possible test/scratch mote still active: %q", m.Title),
				})
				break
			}
		}
	}

	// Active contradictions: pairs of active motes linked by contradicts
	type pair struct{ a, b string }
	seen := make(map[pair]bool)
	for _, m := range motes {
		if m.Status == "deprecated" || m.Status == "archived" {
			continue
		}
		for _, cID := range m.Contradicts {
			other, ok := moteMap[cID]
			if !ok || other.Status == "deprecated" || other.Status == "archived" {
				continue
			}
			p := pair{m.ID, cID}
			pr := pair{cID, m.ID}
			if !seen[p] && !seen[pr] {
				seen[p] = true
				issues = append(issues, doctorIssue{
					Category: "contradiction",
					MoteID:   m.ID,
					Detail:   fmt.Sprintf("%s <-> %s unresolved conflict", m.ID, cID),
				})
			}
		}
	}

	// Overloaded tags
	tagOverloadThreshold := cfg.Dream.PreScan.TagOverloadThreshold
	if tagOverloadThreshold <= 0 {
		tagOverloadThreshold = 15
	}
	for tag, count := range idx.TagStats {
		if count > tagOverloadThreshold {
			issues = append(issues, doctorIssue{
				Category: "overloaded_tag",
				MoteID:   "tag:" + tag,
				Detail:   fmt.Sprintf("%d motes (consider splitting)", count),
			})
		}
	}

	// Orphaned edges: edges pointing to motes not in the active graph.
	// concept_ref edges point to semantic concept terms, not mote IDs — skip them.
	trashedSet := make(map[string]bool)
	if trashed, err := mm.ListTrash(); err == nil {
		for _, m := range trashed {
			trashedSet[m.ID] = true
		}
	}
	for _, e := range idx.Edges {
		if e.EdgeType == "concept_ref" {
			continue
		}
		for _, id := range []string{e.Source, e.Target} {
			if _, ok := moteMap[id]; ok {
				continue
			}
			detail := fmt.Sprintf("edge %s->%s (%s): %s", e.Source, e.Target, e.EdgeType, id)
			if trashedSet[id] {
				detail += " is in trash"
			} else {
				detail += " not found"
			}
			detail += " (run: mote index rebuild)"
			issues = append(issues, doctorIssue{
				Category: "orphaned_edge",
				MoteID:   e.Source + "->" + e.Target,
				Detail:   detail,
			})
		}
	}

	// Circular dependency detection
	for _, cycle := range detectDependsCycles(idx) {
		issues = append(issues, doctorIssue{
			Category: "circular_dep",
			MoteID:   cycle[0],
			Detail:   strings.Join(cycle, " -> ") + " -> " + cycle[0],
		})
	}

	// Bloat detection: high inflow with zero outflow over 30d
	if len(motes) >= 20 {
		fs := computeFlowStats(motes)
		if fs.Created30d >= 15 && fs.Deprecated30d == 0 {
			issues = append(issues, doctorIssue{
				Category: "bloat",
				MoteID:   "(graph)",
				Detail: fmt.Sprintf(
					"%d motes created in 30d with 0 deprecated. Run `mote ls --stale` or `mote dream`.",
					fs.Created30d,
				),
			})
		}
	}

	return issues
}

// detectDependsCycles finds cycles in depends_on edges using three-color DFS.
// Returns deduplicated cycles, each normalized by rotation to smallest ID.
func detectDependsCycles(idx *core.EdgeIndex) [][]string {
	// Build adjacency list from depends_on edges only
	adj := make(map[string][]string)
	nodes := make(map[string]bool)
	for _, e := range idx.Edges {
		if e.EdgeType != "depends_on" {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
		nodes[e.Source] = true
		nodes[e.Target] = true
	}

	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully processed
	)

	color := make(map[string]int)
	parent := make(map[string]string)
	var cycles [][]string
	seen := make(map[string]bool) // normalized cycle keys for dedup

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		for _, next := range adj[node] {
			switch color[next] {
			case white:
				parent[next] = node
				dfs(next)
			case gray:
				// Back edge found — reconstruct cycle
				cycle := []string{next}
				cur := node
				for cur != next {
					cycle = append(cycle, cur)
					cur = parent[cur]
				}
				// Reverse so cycle reads in dependency order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				// Normalize: rotate so smallest ID is first
				cycle = normalizeCycle(cycle)
				key := strings.Join(cycle, "|")
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
			// black: already fully processed, skip (diamond dep is safe)
		}
		color[node] = black
	}

	// Sort nodes for deterministic output
	sortedNodes := make([]string, 0, len(nodes))
	for n := range nodes {
		sortedNodes = append(sortedNodes, n)
	}
	sort.Strings(sortedNodes)

	for _, n := range sortedNodes {
		if color[n] == white {
			dfs(n)
		}
	}

	return cycles
}

// normalizeCycle rotates a cycle so the smallest ID is first.
func normalizeCycle(cycle []string) []string {
	if len(cycle) == 0 {
		return cycle
	}
	minIdx := 0
	for i, id := range cycle {
		if id < cycle[minIdx] {
			minIdx = i
		}
	}
	if minIdx == 0 {
		return cycle
	}
	rotated := make([]string, len(cycle))
	copy(rotated, cycle[minIdx:])
	copy(rotated[len(cycle)-minIdx:], cycle[:minIdx])
	return rotated
}

func collectAllLinks(m *core.Mote) map[string][]string {
	links := map[string][]string{}
	if len(m.DependsOn) > 0 {
		links["depends_on"] = m.DependsOn
	}
	if len(m.Blocks) > 0 {
		links["blocks"] = m.Blocks
	}
	if len(m.RelatesTo) > 0 {
		links["relates_to"] = m.RelatesTo
	}
	if len(m.BuildsOn) > 0 {
		links["builds_on"] = m.BuildsOn
	}
	if len(m.Contradicts) > 0 {
		links["contradicts"] = m.Contradicts
	}
	if len(m.Supersedes) > 0 {
		links["supersedes"] = m.Supersedes
	}
	if len(m.CausedBy) > 0 {
		links["caused_by"] = m.CausedBy
	}
	if len(m.InformedBy) > 0 {
		links["informed_by"] = m.InformedBy
	}
	return links
}

// runDoctorAdvisories returns advisory warnings about graph complexity.
// These are structural notices — not integrity failures — and do not affect exit code.
func runDoctorAdvisories(idx *core.EdgeIndex, moteMap map[string]*core.Mote, cfg *core.Config) []string {
	if len(moteMap) == 0 {
		return nil
	}
	var advisories []string

	maxAvgLinks := cfg.Doctor.MaxAvgLinks
	if maxAvgLinks <= 0 {
		maxAvgLinks = 8.0
	}
	maxChainDepth := cfg.Doctor.MaxChainDepth
	if maxChainDepth <= 0 {
		maxChainDepth = 10
	}
	singletonPct := cfg.Doctor.SingletonPct
	if singletonPct <= 0 {
		singletonPct = 50.0
	}

	// Check 1: average link density
	avgLinks := float64(len(idx.Edges)) / float64(len(moteMap))
	if avgLinks > maxAvgLinks {
		advisories = append(advisories, fmt.Sprintf(
			"High link density: avg %.1f links/mote (threshold: %.0f). Dense graphs slow traversal and dilute edge bonuses. Consider whether all links are meaningful.",
			avgLinks, maxAvgLinks))
	}

	// Check 2: deep dependency chains
	depth := maxDependsOnDepth(idx)
	if depth > maxChainDepth {
		advisories = append(advisories, fmt.Sprintf(
			"Deep dependency chain: max depth %d (threshold: %d). Deep chains complicate planning view and slow traversal.",
			depth, maxChainDepth))
	}

	// Check 3: tag fragmentation
	if len(idx.TagStats) > 0 {
		singletons := 0
		for _, count := range idx.TagStats {
			if count == 1 {
				singletons++
			}
		}
		pct := float64(singletons) / float64(len(idx.TagStats)) * 100
		if pct > singletonPct {
			advisories = append(advisories, fmt.Sprintf(
				"Tag fragmentation: %.0f%% of tags used by only one mote (%d/%d). Singleton tags don't improve retrieval. Consider consolidating.",
				pct, singletons, len(idx.TagStats)))
		}
	}

	return advisories
}

// runDoctorProviderAdvisories returns config-level warnings about the dream
// LLM provider configuration. These are non-fatal — they alert the user to
// likely-broken setups before the next dream run discovers them at runtime.
//
// All checks are local: no network calls, no shell-outs. The intent is that
// running `mote doctor` on a fresh machine flags the obvious mistakes (forgot
// to set OPENAI_API_KEY, didn't fill in gcp_project) without depending on
// internet access.
func runDoctorProviderAdvisories(cfg *core.Config) []string {
	var advisories []string
	for _, stage := range []struct {
		name  string
		entry core.ProviderEntry
	}{
		{"batch", cfg.Dream.Provider.Batch},
		{"reconciliation", cfg.Dream.Provider.Reconciliation},
	} {
		advisories = append(advisories, providerStageAdvisories(stage.name, stage.entry)...)
	}
	return advisories
}

func providerStageAdvisories(stage string, entry core.ProviderEntry) []string {
	var out []string
	prefix := "dream.provider." + stage
	switch entry.Backend {
	case "", "claude-cli":
		// claude-cli takes its credentials from the claude binary itself; the
		// auth field is decorative. Nothing to advise on.
	case "codex-cli":
		if _, err := exec.LookPath("codex"); err != nil {
			out = append(out, fmt.Sprintf(
				"%s.backend=%q but the codex CLI is not on PATH; install it from https://developers.openai.com/codex and run `codex login`.",
				prefix, entry.Backend))
		}
	case "gemini-cli":
		if _, err := exec.LookPath("gemini"); err != nil {
			out = append(out, fmt.Sprintf(
				"%s.backend=%q but the gemini CLI is not on PATH; install it from https://geminicli.com and run it once to authenticate.",
				prefix, entry.Backend))
		}
	case "openai":
		if entry.Auth == "" {
			out = append(out, fmt.Sprintf(
				"%s.auth is empty; set it to the env var holding your OpenAI key (e.g. OPENAI_API_KEY).", prefix))
		} else if looksLikeProviderEnvVar(entry.Auth) && os.Getenv(entry.Auth) == "" {
			out = append(out, fmt.Sprintf(
				"%s.auth=%q but $%s is not set; export it before running mote dream.",
				prefix, entry.Auth, entry.Auth))
		}
		if entry.Model == "" {
			out = append(out, fmt.Sprintf("%s.model is empty; set it to an OpenAI model id (e.g. gpt-4o).", prefix))
		}
	case "gemini":
		if entry.Auth != "vertex-ai" {
			out = append(out, fmt.Sprintf(
				"%s.auth=%q is not supported; only %q (Vertex AI ADC) works for the gemini backend in this release.",
				prefix, entry.Auth, "vertex-ai"))
		}
		if entry.Options["gcp_project"] == "" {
			out = append(out, fmt.Sprintf("%s.options.gcp_project is required for the gemini backend.", prefix))
		}
		if entry.Model == "" {
			out = append(out, fmt.Sprintf("%s.model is empty; set it to a Gemini model id (e.g. gemini-2.5-flash).", prefix))
		}
	default:
		// Unknown backends are blocked at config-load time, but emit an advisory
		// in case a user is staring at `mote doctor` output mid-edit.
		out = append(out, fmt.Sprintf(
			"%s.backend=%q is not recognized (valid: claude-cli, openai, gemini, codex-cli, gemini-cli).",
			prefix, entry.Backend))
	}
	return out
}

// looksLikeProviderEnvVar mirrors the heuristic in internal/dream/auth.go's
// looksLikeEnvVarName. Duplicated here rather than exported because the
// detection rule is intentionally narrow and may diverge later (e.g. if doctor
// wants to be stricter than runtime).
func looksLikeProviderEnvVar(s string) bool {
	if s == "" {
		return false
	}
	hasUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
			hasUnderscore = true
		default:
			return false
		}
	}
	return hasUnderscore
}

// maxDependsOnDepth returns the maximum depth of any depends_on chain in the graph.
// Uses BFS from root nodes (no incoming depends_on edges) to find the longest path.
func maxDependsOnDepth(idx *core.EdgeIndex) int {
	// Build adjacency list and track nodes with incoming depends_on edges
	adj := make(map[string][]string)
	hasIncoming := make(map[string]bool)
	allNodes := make(map[string]bool)

	for _, e := range idx.Edges {
		if e.EdgeType != "depends_on" {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
		hasIncoming[e.Target] = true
		allNodes[e.Source] = true
		allNodes[e.Target] = true
	}

	if len(allNodes) == 0 {
		return 0
	}

	// BFS from each root node (no incoming depends_on edge)
	maxDepth := 0
	for node := range allNodes {
		if hasIncoming[node] {
			continue
		}
		// BFS tracking depth
		type item struct {
			id    string
			depth int
		}
		visited := map[string]bool{node: true}
		queue := []item{{node, 0}}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur.depth > maxDepth {
				maxDepth = cur.depth
			}
			for _, next := range adj[cur.id] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, item{next, cur.depth + 1})
				}
			}
		}
	}
	return maxDepth
}
