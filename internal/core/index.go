// SPDX-License-Identifier: AGPL-3.0-or-later
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
	Deleted  bool   `json:"deleted,omitempty"`
}

type EdgeIndex struct {
	Edges        []Edge
	TagStats     map[string]int
	ConceptStats map[string]int // merged tag + concept term frequencies for IDF
	MoteIDs      map[string]bool

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
	path             string
	tagStatsPath     string
	conceptStatsPath string
	index            *EdgeIndex
}

func NewIndexManager(root string) *IndexManager {
	return &IndexManager{
		path:             filepath.Join(root, "index.jsonl"),
		tagStatsPath:     filepath.Join(root, "tag_stats.json"),
		conceptStatsPath: filepath.Join(root, "concept_stats.json"),
	}
}

func emptyIndex() *EdgeIndex {
	return &EdgeIndex{
		Edges:        []Edge{},
		TagStats:     map[string]int{},
		ConceptStats: map[string]int{},
		MoteIDs:      map[string]bool{},
		outgoing:     map[string][]Edge{},
		incoming:     map[string][]Edge{},
	}
}

// Load reads index.jsonl. Missing file returns empty index (not an error).
func (im *IndexManager) Load() (*EdgeIndex, error) {
	idx := emptyIndex()

	data, err := os.ReadFile(im.path)
	if os.IsNotExist(err) {
		im.index = idx
		// Try loading stats from separate files
		im.loadTagStats(idx)
		im.loadConceptStats(idx)
		return idx, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	// Track tombstones: source|target|edge_type -> bool
	type edgeKey struct{ source, target, edgeType string }
	tombstones := map[edgeKey]bool{}
	var allEdges []Edge

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		// Legacy format: tag_stats footer line
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
		key := edgeKey{edge.Source, edge.Target, edge.EdgeType}
		if edge.Deleted {
			tombstones[key] = true
		} else {
			allEdges = append(allEdges, edge)
		}
	}

	// Filter out tombstoned edges
	for _, e := range allEdges {
		key := edgeKey{e.Source, e.Target, e.EdgeType}
		if !tombstones[key] {
			idx.Edges = append(idx.Edges, e)
		}
	}

	// Load stats from separate files (may override legacy footer)
	im.loadTagStats(idx)
	im.loadConceptStats(idx)

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
		// concept_ref targets are terms, not mote IDs — exclude from MoteIDs
		if e.EdgeType != "concept_ref" {
			idx.MoteIDs[e.Target] = true
		}
	}
}

func (im *IndexManager) loadTagStats(idx *EdgeIndex) {
	data, err := os.ReadFile(im.tagStatsPath)
	if err != nil {
		return // Not found is fine, keep whatever was loaded from legacy footer
	}
	var stats map[string]int
	if err := json.Unmarshal(data, &stats); err == nil && stats != nil {
		idx.TagStats = stats
	}
}

func (im *IndexManager) loadConceptStats(idx *EdgeIndex) {
	data, err := os.ReadFile(im.conceptStatsPath)
	if err != nil {
		// Fallback: if no concept_stats.json, use TagStats
		if idx.ConceptStats == nil || len(idx.ConceptStats) == 0 {
			idx.ConceptStats = make(map[string]int, len(idx.TagStats))
			for k, v := range idx.TagStats {
				idx.ConceptStats[k] = v
			}
		}
		return
	}
	var stats map[string]int
	if err := json.Unmarshal(data, &stats); err == nil && stats != nil {
		idx.ConceptStats = stats
	}
}

func (im *IndexManager) writeConceptStats() error {
	if im.index == nil {
		return nil
	}
	data, err := json.Marshal(im.index.ConceptStats)
	if err != nil {
		return fmt.Errorf("marshal concept stats: %w", err)
	}
	return AtomicWrite(im.conceptStatsPath, data, 0644)
}

func (im *IndexManager) writeTagStats() error {
	if im.index == nil {
		return nil
	}
	data, err := json.Marshal(im.index.TagStats)
	if err != nil {
		return fmt.Errorf("marshal tag stats: %w", err)
	}
	return AtomicWrite(im.tagStatsPath, data, 0644)
}

func (im *IndexManager) appendEdgeLine(edge Edge) error {
	line, err := json.Marshal(edge)
	if err != nil {
		return fmt.Errorf("marshal edge: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(im.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open index for append: %w", err)
	}
	defer f.Close()

	_, err = f.Write(line)
	return err
}

// buildEdges constructs an EdgeIndex from mote frontmatter link fields (in-memory only).
func buildEdges(motes []*Mote) *EdgeIndex {
	var edges []Edge
	tagStats := map[string]int{}
	conceptTermStats := map[string]int{}

	moteIDSet := make(map[string]bool, len(motes))
	for _, m := range motes {
		moteIDSet[m.ID] = true
	}

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

		for _, target := range m.BuildsOn {
			edges = append(edges, Edge{Source: target, Target: m.ID, EdgeType: "built_by_ref"})
		}

		if m.Parent != "" {
			edges = append(edges, Edge{Source: m.ID, Target: m.Parent, EdgeType: "child_of"})
			edges = append(edges, Edge{Source: m.Parent, Target: m.ID, EdgeType: "parent_of"})
		}

		resolved, concepts := ExtractBodyLinksClassified(m.Body, m.ID, moteIDSet)
		for _, target := range resolved {
			edges = append(edges, Edge{Source: m.ID, Target: target, EdgeType: "body_ref"})
		}
		for _, term := range concepts {
			edges = append(edges, Edge{Source: m.ID, Target: term, EdgeType: "concept_ref"})
			conceptTermStats[term]++
		}
	}

	conceptStats := make(map[string]int, len(tagStats)+len(conceptTermStats))
	for k, v := range tagStats {
		conceptStats[k] = v
	}
	for k, v := range conceptTermStats {
		conceptStats[k] += v
	}

	return &EdgeIndex{Edges: edges, TagStats: tagStats, ConceptStats: conceptStats}
}

// Rebuild reconstructs the index from mote frontmatter link fields and persists to disk.
func (im *IndexManager) Rebuild(motes []*Mote) error {
	idx := buildEdges(motes)
	im.buildDerived(idx)
	im.index = idx
	return im.writeIndex()
}

// BuildInMemory constructs a transient EdgeIndex from motes without writing to disk.
// Used for unified cross-scope graph views (e.g., merging local + global motes).
func (im *IndexManager) BuildInMemory(motes []*Mote) *EdgeIndex {
	idx := buildEdges(motes)
	im.buildDerived(idx)
	im.index = idx
	return idx
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
	return im.appendEdgeLine(edge)
}

// RemoveEdge removes a specific edge from the index.
func (im *IndexManager) RemoveEdge(source, target, edgeType string) error {
	if im.index == nil {
		if _, err := im.Load(); err != nil {
			return err
		}
	}
	filtered := im.index.Edges[:0]
	found := false
	for _, e := range im.index.Edges {
		if e.Source == source && e.Target == target && e.EdgeType == edgeType {
			found = true
		} else {
			filtered = append(filtered, e)
		}
	}
	if !found {
		return nil
	}
	im.index.Edges = filtered
	im.buildDerived(im.index)
	return im.appendEdgeLine(Edge{Source: source, Target: target, EdgeType: edgeType, Deleted: true})
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
	if err := AtomicWrite(im.path, []byte(buf.String()), 0644); err != nil {
		return err
	}
	if err := im.writeTagStats(); err != nil {
		return err
	}
	return im.writeConceptStats()
}

// Compact rewrites the index file without tombstones and updates tag_stats.
func (im *IndexManager) Compact() error {
	if im.index == nil {
		if _, err := im.Load(); err != nil {
			return err
		}
	}
	return im.writeIndex()
}
