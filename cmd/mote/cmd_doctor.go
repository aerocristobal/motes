package main

import (
	"fmt"
	"os"
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
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	motes, err := mm.ReadAllParallel()
	if err != nil {
		return fmt.Errorf("read motes: %w", err)
	}

	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	var issues []doctorIssue

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

		// Stale motes: active with last_accessed > 180 days or nil
		if m.Status == "active" {
			if m.LastAccessed == nil {
				issues = append(issues, doctorIssue{
					Category: "stale",
					MoteID:   m.ID,
					Detail:   "never accessed",
				})
			} else if time.Since(*m.LastAccessed).Hours()/24 > 180 {
				days := int(time.Since(*m.LastAccessed).Hours() / 24)
				issues = append(issues, doctorIssue{
					Category: "stale",
					MoteID:   m.ID,
					Detail:   fmt.Sprintf("last accessed %d days ago", days),
				})
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
	for tag, count := range idx.TagStats {
		if count > 15 {
			issues = append(issues, doctorIssue{
				Category: "overloaded_tag",
				MoteID:   "tag:" + tag,
				Detail:   fmt.Sprintf("%d motes (consider splitting)", count),
			})
		}
	}

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
