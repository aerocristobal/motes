// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"motes/internal/security"
)

// ExternalRef represents a reference to an external system (e.g., GitHub issue, Jira ticket).
type ExternalRef struct {
	Provider string `yaml:"provider" json:"provider"`
	ID       string `yaml:"id" json:"id"`
	URL      string `yaml:"url,omitempty" json:"url,omitempty"`
}

var wikiLinkRe = regexp.MustCompile(`\[\[([a-zA-Z0-9._-]+)\]\]`)

// ExtractBodyLinks finds all [[id]] wiki-links in body text, excluding self-references and duplicates.
func ExtractBodyLinks(body, selfID string) []string {
	matches := wikiLinkRe.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	var result []string
	for _, m := range matches {
		id := m[1]
		if id == selfID || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

// ExtractBodyLinksClassified classifies wiki-links as resolved (target is a known mote ID)
// or concept (target is an unresolved term like "authentication").
func ExtractBodyLinksClassified(body, selfID string, moteIDs map[string]bool) (resolved, concepts []string) {
	matches := wikiLinkRe.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	for _, m := range matches {
		target := m[1]
		if target == selfID || seen[target] {
			continue
		}
		seen[target] = true
		if moteIDs[target] {
			resolved = append(resolved, target)
		} else {
			concepts = append(concepts, target)
		}
	}
	return
}

// CountConcepts returns the number of concept terms associated with a mote (tags + unresolved wiki-links).
func CountConcepts(m *Mote) int {
	return len(m.Tags) + len(ExtractBodyLinks(m.Body, m.ID))
}

// Mote is the atomic unit of knowledge in the nebula.
type Mote struct {
	// Identity
	ID     string   `yaml:"id"`
	Type   string   `yaml:"type"`   // task|decision|lesson|context|question|constellation|anchor|explore
	Status string   `yaml:"status"` // active|in_progress|deprecated|archived|completed
	Title  string   `yaml:"title"`
	Tags   []string `yaml:"tags"`
	Weight float64  `yaml:"weight"` // 0.0-1.0
	Origin string   `yaml:"origin"` // normal|failure|revert|hotfix|discovery
	Action string   `yaml:"action,omitempty"` // Dream-extracted prescriptive summary

	// Retrieval metadata (auto-managed)
	CreatedAt    time.Time  `yaml:"created_at"`
	LastAccessed *time.Time `yaml:"last_accessed"`
	AccessCount  int        `yaml:"access_count"`

	// Planning links
	DependsOn []string `yaml:"depends_on"`
	Blocks    []string `yaml:"blocks"`

	// Memory links
	RelatesTo   []string `yaml:"relates_to"`
	BuildsOn    []string `yaml:"builds_on"`
	Contradicts []string `yaml:"contradicts"`
	Supersedes  []string `yaml:"supersedes"`
	CausedBy    []string `yaml:"caused_by"`
	InformedBy  []string `yaml:"informed_by"`

	// External references
	ExternalRefs []ExternalRef `yaml:"external_refs,omitempty"`

	// Issue integration
	SourceIssue    string     `yaml:"source_issue,omitempty"`
	CrystallizedAt *time.Time `yaml:"crystallized_at,omitempty"`

	// Global promotion
	PromotedTo string `yaml:"promoted_to,omitempty"`

	// Deprecation tracking
	DeprecatedBy    string     `yaml:"deprecated_by,omitempty"`
	StatusChangedAt *time.Time `yaml:"status_changed_at,omitempty"`

	// Hierarchy
	Parent string `yaml:"parent,omitempty"`

	// Acceptance criteria
	Acceptance    []string `yaml:"acceptance,omitempty"`
	AcceptanceMet []bool   `yaml:"acceptance_met,omitempty"`

	// Effort sizing
	Size string `yaml:"size,omitempty"` // xs|s|m|l|xl

	// Strata integration (anchor motes only)
	StrataCorpus      string     `yaml:"strata_corpus,omitempty"`
	StrataQueryHint   string     `yaml:"strata_query_hint,omitempty"`
	StrataQueryCount  int        `yaml:"strata_query_count,omitempty"`
	StrataLastQueried *time.Time `yaml:"strata_last_queried,omitempty"`

	// Code artifact references (anchor motes linking to source files)
	CodeFilePaths []string `yaml:"code_file_paths,omitempty"`

	// Agent tracking
	CreatedBy  string `yaml:"created_by,omitempty"`
	ModifiedBy string `yaml:"modified_by,omitempty"`

	// Global knowledge routing
	OriginProject string `yaml:"origin_project,omitempty"`
	ForwardedTo   string `yaml:"forwarded_to,omitempty"`

	// Soft-delete tracking
	DeletedAt *time.Time `yaml:"deleted_at,omitempty"`

	// Non-YAML (populated after parse)
	Body     string `yaml:"-"` // markdown content below frontmatter
	FilePath string `yaml:"-"` // absolute path to .md file
}

type frontmatterParts struct {
	frontmatter string
	body        string
}

// splitFrontmatter splits a mote file into YAML frontmatter and markdown body.
// Returns nil if the file doesn't have valid --- delimiters.
func splitFrontmatter(content string) *frontmatterParts {
	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		return nil
	}

	// Find the closing ---
	if len(content) < 4 {
		return nil
	}
	rest, err := security.SafeSubstring(content, 3, len(content))
	if err != nil {
		return nil
	}
	// Skip the newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		if newRest, err := security.SafeSubstring(rest, 1, len(rest)); err == nil {
			rest = newRest
		}
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		if newRest, err := security.SafeSubstring(rest, 2, len(rest)); err == nil {
			rest = newRest
		}
	}

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil
	}

	fm, err := security.SafeSubstring(rest, 0, idx)
	if err != nil {
		return nil
	}
	if idx+4 > len(rest) {
		return nil
	}
	after, err := security.SafeSubstring(rest, idx+4, len(rest))
	if err != nil {
		return nil
	}

	// Strip leading newline from body
	if len(after) > 0 && after[0] == '\n' {
		if newAfter, err := security.SafeSubstring(after, 1, len(after)); err == nil {
			after = newAfter
		}
	} else if len(after) > 1 && after[0] == '\r' && after[1] == '\n' {
		if newAfter, err := security.SafeSubstring(after, 2, len(after)); err == nil {
			after = newAfter
		}
	}

	return &frontmatterParts{
		frontmatter: fm,
		body:        after,
	}
}

// ParseMote reads and parses a mote file from disk.
func ParseMote(path string) (*Mote, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parts := splitFrontmatter(string(data))
	if parts == nil {
		return nil, fmt.Errorf("no frontmatter in %s", path)
	}
	var m Mote
	if err := yaml.Unmarshal([]byte(parts.frontmatter), &m); err != nil {
		return nil, fmt.Errorf("bad frontmatter in %s: %w", path, err)
	}
	m.Body = parts.body
	m.FilePath = path
	return &m, nil
}

// SerializeMote renders a Mote back to the markdown-with-frontmatter format.
func SerializeMote(m *Mote) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	var buf strings.Builder
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	if m.Body != "" {
		buf.WriteString(m.Body)
	}
	return []byte(buf.String()), nil
}
