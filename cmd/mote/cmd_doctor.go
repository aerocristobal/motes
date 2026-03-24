package main

import (
	"fmt"
	"os"
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

	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	if len(issues) == 0 {
		fmt.Println("No issues found. Graph is healthy.")
		return nil
	}

	fmt.Printf("%-16s  %-26s  %s\n", "ISSUE", "MOTE", "DETAIL")
	fmt.Println(strings.Repeat("-", 80))
	for _, iss := range issues {
		fmt.Printf("%-16s  %-26s  %s\n", iss.Category, iss.MoteID, iss.Detail)
	}
	fmt.Printf("\n%d issue(s) found.\n", len(issues))
	os.Exit(1)
	return nil
}

func runDoctorChecks(mm *core.MoteManager, im *core.IndexManager, idx *core.EdgeIndex, moteMap map[string]*core.Mote, cfg *core.Config) []doctorIssue {
	var issues []doctorIssue

	// Collect motes as a slice for ordered iteration
	var motes []*core.Mote
	for _, m := range moteMap {
		motes = append(motes, m)
	}

	for _, m := range motes {
		// Broken links
		allLinks := collectAllLinks(m)
		for linkType, targets := range allLinks {
			for _, target := range targets {
				if _, ok := moteMap[target]; !ok {
					issues = append(issues, doctorIssue{
						Category: "broken_link",
						MoteID:   m.ID,
						Detail:   fmt.Sprintf("%s target %s does not exist", linkType, target),
					})
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

		// Deprecated blockers: active motes depending on deprecated motes
		if m.Status == "active" {
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

		// Stale motes: active with last_accessed > threshold days or nil
		stalenessThreshold := cfg.Dream.PreScan.StalenessThresholdDays
		if stalenessThreshold <= 0 {
			stalenessThreshold = 180
		}
		if m.Status == "active" {
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

	// Test/scratch motes: active motes with test-like titles
	testPrefixes := []string{"test ", "access test", "tag test", "scratch "}
	for _, m := range motes {
		if m.Status != "active" {
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

	// Orphaned edges: edges pointing to motes not in the active graph
	trashedSet := make(map[string]bool)
	if trashed, err := mm.ListTrash(); err == nil {
		for _, m := range trashed {
			trashedSet[m.ID] = true
		}
	}
	for _, e := range idx.Edges {
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
