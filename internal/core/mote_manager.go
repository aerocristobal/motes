package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"motes/internal/security"
)

type MoteManager struct {
	root           string
	accessBatchMux sync.Mutex // Protects access batch operations
}

type CreateOpts struct {
	Tags          []string
	Weight        float64
	Origin        string
	Body          string
	StrataCorpus  string
	SourceIssue   string
}

type ListFilters struct {
	Type   string
	Tag    string
	Status string
	Stale  bool
	Ready  bool
}

type AccessBatchEntry struct {
	MoteID     string `json:"mote_id"`
	AccessedAt string `json:"accessed_at"`
}

func NewMoteManager(root string) *MoteManager {
	return &MoteManager{root: root}
}

// Root returns the .memory root path.
func (mm *MoteManager) Root() string {
	return mm.root
}

func (mm *MoteManager) nodesDir() string {
	return filepath.Join(mm.root, "nodes")
}

func (mm *MoteManager) moteFilePath(moteID string) (string, error) {
	if err := security.ValidateMoteID(moteID); err != nil {
		return "", fmt.Errorf("invalid mote ID: %w", err)
	}
	return filepath.Join(mm.nodesDir(), moteID+".md"), nil
}

// MoteFilePath returns the file path for a mote by ID.
func (mm *MoteManager) MoteFilePath(moteID string) (string, error) {
	return mm.moteFilePath(moteID)
}

func scopeFromRoot(root string) string {
	return filepath.Base(filepath.Dir(root))
}

// Create makes a new mote file and returns the mote.
func (mm *MoteManager) Create(moteType, title string, opts CreateOpts) (*Mote, error) {
	scope := scopeFromRoot(mm.root)
	id := GenerateID(scope, moteType)

	weight := opts.Weight
	if weight == 0 {
		weight = 0.5
	}
	origin := opts.Origin
	if origin == "" {
		origin = "normal"
	}

	now := time.Now().UTC()
	m := &Mote{
		ID:           id,
		Type:         moteType,
		Status:       "active",
		Title:        title,
		Tags:         opts.Tags,
		Weight:       weight,
		Origin:       origin,
		CreatedAt:    now,
		AccessCount:  0,
		Body:         opts.Body,
		StrataCorpus: opts.StrataCorpus,
		SourceIssue:  opts.SourceIssue,
	}

	data, err := SerializeMote(m)
	if err != nil {
		return nil, fmt.Errorf("serialize: %w", err)
	}
	path, err := mm.moteFilePath(id)
	if err != nil {
		return nil, fmt.Errorf("get file path: %w", err)
	}
	if err := AtomicWrite(path, data, 0644); err != nil {
		return nil, fmt.Errorf("write mote: %w", err)
	}
	m.FilePath = path
	return m, nil
}

// Read loads a mote by ID.
func (mm *MoteManager) Read(moteID string) (*Mote, error) {
	path, err := mm.moteFilePath(moteID)
	if err != nil {
		return nil, fmt.Errorf("get file path: %w", err)
	}
	return ParseMote(path)
}

// Update applies field changes to a mote and persists them.
func (mm *MoteManager) Update(moteID string, fields map[string]interface{}) error {
	m, err := mm.Read(moteID)
	if err != nil {
		return err
	}
	for k, v := range fields {
		switch k {
		case "status":
			m.Status = v.(string)
		case "title":
			m.Title = v.(string)
		case "weight":
			m.Weight = v.(float64)
		case "tags":
			m.Tags = v.([]string)
		case "last_accessed":
			t := v.(time.Time)
			m.LastAccessed = &t
		case "access_count":
			m.AccessCount = v.(int)
		case "body":
			m.Body = v.(string)
		case "deprecated_by":
			m.DeprecatedBy = v.(string)
		}
	}
	data, err := SerializeMote(m)
	if err != nil {
		return err
	}
	path, err := mm.moteFilePath(moteID)
	if err != nil {
		return fmt.Errorf("get file path: %w", err)
	}
	return AtomicWrite(path, data, 0644)
}

// List returns motes matching the given filters.
func (mm *MoteManager) List(filters ListFilters) ([]*Mote, error) {
	motes, err := mm.ReadAllParallel()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	staleThreshold := 90 * 24 * time.Hour

	var result []*Mote
	for _, m := range motes {
		if filters.Type != "" && m.Type != filters.Type {
			continue
		}
		if filters.Status != "" && m.Status != filters.Status {
			continue
		}
		if filters.Tag != "" && !hasTag(m, filters.Tag) {
			continue
		}
		if filters.Stale {
			isStale := m.LastAccessed == nil || now.Sub(*m.LastAccessed) >= staleThreshold
			if !isStale {
				continue
			}
		}
		result = append(result, m)
	}

	if filters.Ready {
		// Build mote map for BFS lookups
		moteMap := make(map[string]*Mote, len(motes))
		for _, m := range motes {
			moteMap[m.ID] = m
		}

		var ready []*Mote
		for _, m := range result {
			if m.Type != "task" || m.Status != "active" {
				continue
			}
			if len(m.DependsOn) == 0 {
				ready = append(ready, m)
				continue
			}
			if transitiveReady(m, moteMap) {
				ready = append(ready, m)
			}
		}
		result = ready
	}

	return result, nil
}

// transitiveReady returns true if all transitive dependencies are non-active.
func transitiveReady(m *Mote, moteMap map[string]*Mote) bool {
	visited := map[string]bool{m.ID: true}
	queue := make([]string, len(m.DependsOn))
	copy(queue, m.DependsOn)

	for len(queue) > 0 {
		depID := queue[0]
		queue = queue[1:]
		if visited[depID] {
			continue
		}
		visited[depID] = true

		dep, ok := moteMap[depID]
		if !ok {
			return false // can't verify, assume not ready
		}
		if dep.Status == "active" {
			return false
		}
		// Continue BFS through this dep's dependencies
		queue = append(queue, dep.DependsOn...)
	}
	return true
}

func hasTag(m *Mote, tag string) bool {
	for _, t := range m.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Deprecate sets a mote's status to deprecated and records who deprecated it.
func (mm *MoteManager) Deprecate(moteID, supersededBy string) error {
	return mm.Update(moteID, map[string]interface{}{
		"status":        "deprecated",
		"deprecated_by": supersededBy,
	})
}

// ReadAll reads all motes sequentially. Malformed files are skipped.
func (mm *MoteManager) ReadAll() ([]*Mote, error) {
	entries, err := os.ReadDir(mm.nodesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var motes []*Mote
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		m, err := ParseMote(filepath.Join(mm.nodesDir(), entry.Name()))
		if err != nil {
			continue
		}
		motes = append(motes, m)
	}
	return motes, nil
}

// ReadAllParallel reads all motes using goroutines. Malformed files produce
// a stderr warning and are skipped.
func (mm *MoteManager) ReadAllParallel() ([]*Mote, error) {
	entries, err := os.ReadDir(mm.nodesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type result struct {
		mote *Mote
		err  error
		name string
	}

	var mdEntries []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			mdEntries = append(mdEntries, entry)
		}
	}

	results := make([]result, len(mdEntries))
	var wg sync.WaitGroup

	for i, entry := range mdEntries {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			path := filepath.Join(mm.nodesDir(), name)
			m, parseErr := ParseMote(path)
			results[idx] = result{mote: m, err: parseErr, name: name}
		}(i, entry.Name())
	}
	wg.Wait()

	var motes []*Mote
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", r.name, r.err)
			continue
		}
		motes = append(motes, r.mote)
	}
	return motes, nil
}

// AppendAccessBatch appends an access record to .access_batch.jsonl.
func (mm *MoteManager) AppendAccessBatch(moteID string) error {
	mm.accessBatchMux.Lock()
	defer mm.accessBatchMux.Unlock()

	entry := AccessBatchEntry{
		MoteID:     moteID,
		AccessedAt: time.Now().UTC().Format(time.RFC3339),
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal access entry: %w", err)
	}
	line = append(line, '\n')

	batchPath := filepath.Join(mm.root, ".access_batch.jsonl")
	f, err := os.OpenFile(batchPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

// FlushAccessBatch reads the access batch, updates mote files, and removes the batch.
func (mm *MoteManager) FlushAccessBatch() error {
	mm.accessBatchMux.Lock()
	defer mm.accessBatchMux.Unlock()

	batchPath := filepath.Join(mm.root, ".access_batch.jsonl")
	data, err := os.ReadFile(batchPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	type counts struct {
		lastAccessed time.Time
		count        int
	}
	grouped := map[string]*counts{}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry AccessBatchEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, entry.AccessedAt)
		if err != nil {
			continue
		}
		if g, ok := grouped[entry.MoteID]; ok {
			g.count++
			if t.After(g.lastAccessed) {
				g.lastAccessed = t
			}
		} else {
			grouped[entry.MoteID] = &counts{lastAccessed: t, count: 1}
		}
	}

	for moteID, g := range grouped {
		m, err := mm.Read(moteID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: flush batch: cannot read %s: %v\n", moteID, err)
			continue
		}
		m.LastAccessed = &g.lastAccessed
		m.AccessCount += g.count
		out, err := SerializeMote(m)
		if err != nil {
			continue
		}
		if path, pathErr := mm.moteFilePath(moteID); pathErr == nil {
			_ = AtomicWrite(path, out, 0644)
		}
	}

	return os.Remove(batchPath)
}

// FlushAccessBatchStats flushes the batch and returns stats (access count, mote count).
func (mm *MoteManager) FlushAccessBatchStats() (accessCount, moteCount int, err error) {
	mm.accessBatchMux.Lock()
	defer mm.accessBatchMux.Unlock()

	batchPath := filepath.Join(mm.root, ".access_batch.jsonl")
	data, err := os.ReadFile(batchPath)
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}

	type counts struct {
		lastAccessed time.Time
		count        int
	}
	grouped := map[string]*counts{}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry AccessBatchEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		t, parseErr := time.Parse(time.RFC3339, entry.AccessedAt)
		if parseErr != nil {
			continue
		}
		accessCount++
		if g, ok := grouped[entry.MoteID]; ok {
			g.count++
			if t.After(g.lastAccessed) {
				g.lastAccessed = t
			}
		} else {
			grouped[entry.MoteID] = &counts{lastAccessed: t, count: 1}
		}
	}

	for moteID, g := range grouped {
		m, readErr := mm.Read(moteID)
		if readErr != nil {
			continue
		}
		m.LastAccessed = &g.lastAccessed
		m.AccessCount += g.count
		out, serErr := SerializeMote(m)
		if serErr != nil {
			continue
		}
		if path, pathErr := mm.moteFilePath(moteID); pathErr == nil {
			_ = AtomicWrite(path, out, 0644)
		}
	}

	moteCount = len(grouped)
	return accessCount, moteCount, os.Remove(batchPath)
}

// Link creates a link from sourceID to targetID with the given type.
// Bidirectionality is handled automatically per ValidLinkTypes rules.
func (mm *MoteManager) Link(sourceID, linkType, targetID string, im *IndexManager) error {
	behavior, ok := ValidLinkTypes[linkType]
	if !ok {
		return fmt.Errorf("unknown link type: %q", linkType)
	}
	if sourceID == targetID {
		return fmt.Errorf("cannot link a mote to itself")
	}

	source, err := mm.Read(sourceID)
	if err != nil {
		return fmt.Errorf("read source %s: %w", sourceID, err)
	}
	target, err := mm.Read(targetID)
	if err != nil {
		return fmt.Errorf("read target %s: %w", targetID, err)
	}

	// Add link to source's frontmatter (idempotent)
	if !sliceContains(GetLinkSlice(source, linkType), targetID) {
		SetLinkSlice(source, linkType, append(GetLinkSlice(source, linkType), targetID))
	}

	// Handle bidirectionality on target's frontmatter
	targetModified := false
	if behavior.Symmetric {
		if !sliceContains(GetLinkSlice(target, linkType), sourceID) {
			SetLinkSlice(target, linkType, append(GetLinkSlice(target, linkType), sourceID))
			targetModified = true
		}
	} else if behavior.InverseType != "" {
		if !sliceContains(GetLinkSlice(target, behavior.InverseType), sourceID) {
			SetLinkSlice(target, behavior.InverseType, append(GetLinkSlice(target, behavior.InverseType), sourceID))
			targetModified = true
		}
	}

	if behavior.AutoDeprecate {
		target.Status = "deprecated"
		target.DeprecatedBy = sourceID
		targetModified = true
	}

	// Persist source
	sourceData, err := SerializeMote(source)
	if err != nil {
		return fmt.Errorf("serialize source: %w", err)
	}
	sourcePath, err := mm.moteFilePath(sourceID)
	if err != nil {
		return fmt.Errorf("get source file path: %w", err)
	}
	if err := AtomicWrite(sourcePath, sourceData, 0644); err != nil {
		return fmt.Errorf("write source: %w", err)
	}

	// Persist target if modified
	if targetModified {
		targetData, err := SerializeMote(target)
		if err != nil {
			return fmt.Errorf("serialize target: %w", err)
		}
		targetPath, err := mm.moteFilePath(targetID)
		if err != nil {
			return fmt.Errorf("get target file path: %w", err)
		}
		if err := AtomicWrite(targetPath, targetData, 0644); err != nil {
			return fmt.Errorf("write target: %w", err)
		}
	}

	// Update index: forward edge
	if err := im.AddEdge(Edge{Source: sourceID, Target: targetID, EdgeType: linkType}); err != nil {
		return fmt.Errorf("add index edge: %w", err)
	}

	// Reverse edges in index
	if behavior.Symmetric {
		_ = im.AddEdge(Edge{Source: targetID, Target: sourceID, EdgeType: linkType})
	} else if behavior.InverseType != "" {
		_ = im.AddEdge(Edge{Source: targetID, Target: sourceID, EdgeType: behavior.InverseType})
	}
	if behavior.IndexReverse != "" {
		_ = im.AddEdge(Edge{Source: targetID, Target: sourceID, EdgeType: behavior.IndexReverse})
	}

	return nil
}

// Unlink removes a link from sourceID to targetID.
// Reverses bidirectional writes per ValidLinkTypes rules.
func (mm *MoteManager) Unlink(sourceID, linkType, targetID string, im *IndexManager) error {
	behavior, ok := ValidLinkTypes[linkType]
	if !ok {
		return fmt.Errorf("unknown link type: %q", linkType)
	}

	source, err := mm.Read(sourceID)
	if err != nil {
		return fmt.Errorf("read source %s: %w", sourceID, err)
	}

	// Remove from source frontmatter
	SetLinkSlice(source, linkType, sliceRemove(GetLinkSlice(source, linkType), targetID))

	sourceData, err := SerializeMote(source)
	if err != nil {
		return fmt.Errorf("serialize source: %w", err)
	}
	sourcePath, err := mm.moteFilePath(sourceID)
	if err != nil {
		return fmt.Errorf("get source file path: %w", err)
	}
	if err := AtomicWrite(sourcePath, sourceData, 0644); err != nil {
		return fmt.Errorf("write source: %w", err)
	}

	// Remove reverse from target frontmatter
	if behavior.Symmetric || behavior.InverseType != "" {
		target, err := mm.Read(targetID)
		if err != nil {
			return fmt.Errorf("read target %s: %w", targetID, err)
		}

		reverseType := linkType
		if behavior.InverseType != "" {
			reverseType = behavior.InverseType
		}
		SetLinkSlice(target, reverseType, sliceRemove(GetLinkSlice(target, reverseType), sourceID))

		targetData, err := SerializeMote(target)
		if err != nil {
			return fmt.Errorf("serialize target: %w", err)
		}
		targetPath, err := mm.moteFilePath(targetID)
		if err != nil {
			return fmt.Errorf("get target file path: %w", err)
		}
		if err := AtomicWrite(targetPath, targetData, 0644); err != nil {
			return fmt.Errorf("write target: %w", err)
		}
	}

	// Remove index edges
	_ = im.RemoveEdge(sourceID, targetID, linkType)
	if behavior.Symmetric {
		_ = im.RemoveEdge(targetID, sourceID, linkType)
	} else if behavior.InverseType != "" {
		_ = im.RemoveEdge(targetID, sourceID, behavior.InverseType)
	}
	if behavior.IndexReverse != "" {
		_ = im.RemoveEdge(targetID, sourceID, behavior.IndexReverse)
	}

	return nil
}
