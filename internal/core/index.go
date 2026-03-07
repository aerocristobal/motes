package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Edge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	EdgeType string `json:"edge_type"`
}

type EdgeIndex struct {
	Edges    []Edge
	TagStats map[string]int
	MoteIDs  map[string]bool

	outgoing map[string][]Edge
	incoming map[string][]Edge
}

// Neighbors returns outgoing edges from moteID, optionally filtered by type.
func (idx *EdgeIndex) Neighbors(moteID string, edgeTypes map[string]bool) []Edge {
	var result []Edge
	for _, e := range idx.outgoing[moteID] {
		if edgeTypes == nil || edgeTypes[e.EdgeType] {
			result = append(result, e)
		}
	}
	return result
}

// Incoming returns incoming edges to moteID, optionally filtered by type.
func (idx *EdgeIndex) Incoming(moteID string, edgeTypes map[string]bool) []Edge {
	var result []Edge
	for _, e := range idx.incoming[moteID] {
		if edgeTypes == nil || edgeTypes[e.EdgeType] {
			result = append(result, e)
		}
	}
	return result
}

// HasEdges returns true if the moteID has any outgoing or incoming edges.
func (idx *EdgeIndex) HasEdges(moteID string) bool {
	return len(idx.outgoing[moteID]) > 0 || len(idx.incoming[moteID]) > 0
}

type IndexManager struct {
	path  string
	index *EdgeIndex
}

func NewIndexManager(root string) *IndexManager {
	return &IndexManager{
		path: filepath.Join(root, "index.jsonl"),
	}
}

func emptyIndex() *EdgeIndex {
	return &EdgeIndex{
		Edges:    []Edge{},
		TagStats: map[string]int{},
		MoteIDs:  map[string]bool{},
		outgoing: map[string][]Edge{},
		incoming: map[string][]Edge{},
	}
}

// Load reads index.jsonl. Missing file returns empty index (not an error).
func (im *IndexManager) Load() (*EdgeIndex, error) {
	idx := emptyIndex()

	data, err := os.ReadFile(im.path)
	if os.IsNotExist(err) {
		im.index = idx
		return idx, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		if strings.Contains(line, `"tag_stats"`) {
			var footer struct {
				TagStats map[string]int `json:"tag_stats"`
			}
			if err := json.Unmarshal([]byte(line), &footer); err == nil && footer.TagStats != nil {
				idx.TagStats = footer.TagStats
				continue
			}
		}
		var edge Edge
		if err := json.Unmarshal([]byte(line), &edge); err != nil {
			continue
		}
		idx.Edges = append(idx.Edges, edge)
	}

	im.buildDerived(idx)
	im.index = idx
	return idx, nil
}

func (im *IndexManager) buildDerived(idx *EdgeIndex) {
	idx.outgoing = map[string][]Edge{}
	idx.incoming = map[string][]Edge{}
	idx.MoteIDs = map[string]bool{}
	for _, e := range idx.Edges {
		idx.outgoing[e.Source] = append(idx.outgoing[e.Source], e)
		idx.incoming[e.Target] = append(idx.incoming[e.Target], e)
		idx.MoteIDs[e.Source] = true
		idx.MoteIDs[e.Target] = true
	}
}

// Rebuild reconstructs the index from mote frontmatter link fields.
func (im *IndexManager) Rebuild(motes []*Mote) error {
	var edges []Edge
	tagStats := map[string]int{}

	for _, m := range motes {
		for _, tag := range m.Tags {
			tagStats[tag]++
		}
		edges = appendEdges(edges, m.ID, "depends_on", m.DependsOn)
		edges = appendEdges(edges, m.ID, "blocks", m.Blocks)
		edges = appendEdges(edges, m.ID, "relates_to", m.RelatesTo)
		edges = appendEdges(edges, m.ID, "builds_on", m.BuildsOn)
		edges = appendEdges(edges, m.ID, "contradicts", m.Contradicts)
		edges = appendEdges(edges, m.ID, "supersedes", m.Supersedes)
		edges = appendEdges(edges, m.ID, "caused_by", m.CausedBy)
		edges = appendEdges(edges, m.ID, "informed_by", m.InformedBy)

		// Index-only reverse edges for builds_on
		for _, target := range m.BuildsOn {
			edges = append(edges, Edge{Source: target, Target: m.ID, EdgeType: "built_by_ref"})
		}

		// Index-only edges for body wiki-links
		for _, target := range ExtractBodyLinks(m.Body, m.ID) {
			edges = append(edges, Edge{Source: m.ID, Target: target, EdgeType: "body_ref"})
		}
	}

	idx := &EdgeIndex{Edges: edges, TagStats: tagStats}
	im.buildDerived(idx)
	im.index = idx
	return im.writeIndex()
}

func appendEdges(edges []Edge, source, edgeType string, targets []string) []Edge {
	for _, target := range targets {
		edges = append(edges, Edge{Source: source, Target: target, EdgeType: edgeType})
	}
	return edges
}

// AddEdge adds an edge to the index, skipping duplicates.
func (im *IndexManager) AddEdge(edge Edge) error {
	if im.index == nil {
		if _, err := im.Load(); err != nil {
			return err
		}
	}
	for _, e := range im.index.Edges {
		if e.Source == edge.Source && e.Target == edge.Target && e.EdgeType == edge.EdgeType {
			return nil
		}
	}
	im.index.Edges = append(im.index.Edges, edge)
	im.index.outgoing[edge.Source] = append(im.index.outgoing[edge.Source], edge)
	im.index.incoming[edge.Target] = append(im.index.incoming[edge.Target], edge)
	im.index.MoteIDs[edge.Source] = true
	im.index.MoteIDs[edge.Target] = true
	return im.writeIndex()
}

// RemoveEdge removes a specific edge from the index.
func (im *IndexManager) RemoveEdge(source, target, edgeType string) error {
	if im.index == nil {
		if _, err := im.Load(); err != nil {
			return err
		}
	}
	filtered := im.index.Edges[:0]
	for _, e := range im.index.Edges {
		if !(e.Source == source && e.Target == target && e.EdgeType == edgeType) {
			filtered = append(filtered, e)
		}
	}
	im.index.Edges = filtered
	im.buildDerived(im.index)
	return im.writeIndex()
}

// MoteBM25Manager manages a persistent BM25 index over mote content.
type MoteBM25Manager struct {
	path string
}

// NewMoteBM25Manager creates a manager for the mote BM25 index.
func NewMoteBM25Manager(root string) *MoteBM25Manager {
	return &MoteBM25Manager{path: filepath.Join(root, "mote_bm25.json")}
}

// Path returns the filesystem path to the BM25 index file.
func (mbm *MoteBM25Manager) Path() string {
	return mbm.path
}

// LoadRaw reads the persistent BM25 index bytes from disk.
// Returns nil, nil if the file does not exist.
func (mbm *MoteBM25Manager) LoadRaw() ([]byte, error) {
	data, err := os.ReadFile(mbm.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SaveRaw writes BM25 index bytes to disk atomically.
func (mbm *MoteBM25Manager) SaveRaw(data []byte) error {
	return AtomicWrite(mbm.path, data, 0644)
}

func (im *IndexManager) writeIndex() error {
	var buf strings.Builder
	for _, e := range im.index.Edges {
		line, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal edge: %w", err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	footer, err := json.Marshal(struct {
		TagStats map[string]int `json:"tag_stats"`
	}{TagStats: im.index.TagStats})
	if err != nil {
		return fmt.Errorf("marshal tag stats: %w", err)
	}
	buf.Write(footer)
	buf.WriteByte('\n')
	return AtomicWrite(im.path, []byte(buf.String()), 0644)
}
