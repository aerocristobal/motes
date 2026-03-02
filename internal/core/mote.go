package core

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Mote is the atomic unit of knowledge in the nebula.
type Mote struct {
	// Identity
	ID     string   `yaml:"id"`
	Type   string   `yaml:"type"`   // task|decision|lesson|context|question|constellation|anchor|explore
	Status string   `yaml:"status"` // active|deprecated|archived|completed
	Title  string   `yaml:"title"`
	Tags   []string `yaml:"tags"`
	Weight float64  `yaml:"weight"` // 0.0-1.0
	Origin string   `yaml:"origin"` // normal|failure|revert|hotfix|discovery

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

	// Issue integration
	SourceIssue    string     `yaml:"source_issue,omitempty"`
	CrystallizedAt *time.Time `yaml:"crystallized_at,omitempty"`

	// Global promotion
	PromotedTo string `yaml:"promoted_to,omitempty"`

	// Deprecation tracking
	DeprecatedBy string `yaml:"deprecated_by,omitempty"`

	// Strata integration (anchor motes only)
	StrataCorpus      string     `yaml:"strata_corpus,omitempty"`
	StrataQueryHint   string     `yaml:"strata_query_hint,omitempty"`
	StrataQueryCount  int        `yaml:"strata_query_count,omitempty"`
	StrataLastQueried *time.Time `yaml:"strata_last_queried,omitempty"`

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
	rest := content[3:]
	// Skip the newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil
	}

	fm := rest[:idx]
	after := rest[idx+4:] // skip \n---

	// Strip leading newline from body
	if len(after) > 0 && after[0] == '\n' {
		after = after[1:]
	} else if len(after) > 1 && after[0] == '\r' && after[1] == '\n' {
		after = after[2:]
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
