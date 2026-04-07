// SPDX-License-Identifier: AGPL-3.0-or-later
package core

// LinkBehavior describes how a link type propagates between motes.
type LinkBehavior struct {
	// Symmetric: write the same link type to both A and B frontmatter.
	Symmetric bool

	// InverseType: write this type into B's frontmatter when linking A to B.
	InverseType string

	// IndexReverse: add this edge type to the index (but not frontmatter).
	IndexReverse string

	// AutoDeprecate: set B to deprecated with deprecated_by=A.
	AutoDeprecate bool
}

// ValidLinkTypes maps each link type to its bidirectionality behavior.
var ValidLinkTypes = map[string]LinkBehavior{
	"relates_to":  {Symmetric: true},
	"contradicts": {Symmetric: true},
	"depends_on":  {InverseType: "blocks"},
	"blocks":      {InverseType: "depends_on"},
	"builds_on":   {IndexReverse: "built_by_ref"},
	"supersedes":  {AutoDeprecate: true},
	"caused_by":   {},
	"informed_by": {},
}

// GetLinkSlice returns the link slice from a Mote for a given link type.
func GetLinkSlice(m *Mote, linkType string) []string {
	switch linkType {
	case "depends_on":
		return m.DependsOn
	case "blocks":
		return m.Blocks
	case "relates_to":
		return m.RelatesTo
	case "builds_on":
		return m.BuildsOn
	case "contradicts":
		return m.Contradicts
	case "supersedes":
		return m.Supersedes
	case "caused_by":
		return m.CausedBy
	case "informed_by":
		return m.InformedBy
	default:
		return nil
	}
}

// SetLinkSlice sets the link slice on a Mote for a given link type.
func SetLinkSlice(m *Mote, linkType string, ids []string) {
	switch linkType {
	case "depends_on":
		m.DependsOn = ids
	case "blocks":
		m.Blocks = ids
	case "relates_to":
		m.RelatesTo = ids
	case "builds_on":
		m.BuildsOn = ids
	case "contradicts":
		m.Contradicts = ids
	case "supersedes":
		m.Supersedes = ids
	case "caused_by":
		m.CausedBy = ids
	case "informed_by":
		m.InformedBy = ids
	}
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func sliceRemove(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
