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
	cache          *ReadCache
	accessBatchMux sync.Mutex // Protects access batch operations
}

type CreateOpts struct {
	Tags          []string
	Weight        float64
	Origin        string
	Body          string
	StrataCorpus  string
	SourceIssue   string
	Parent        string
	Acceptance    []string
	Size          string
}

type ListFilters struct {
	Type   string
	Tag    string
	Status string
	Stale  bool
	Ready  bool
	Parent string
}

type AccessBatchEntry struct {
	MoteID     string `json:"mote_id"`
	AccessedAt string `json:"accessed_at"`
	AgentID    string `json:"agent_id,omitempty"`
}

func NewMoteManager(root string) *MoteManager {
	return &MoteManager{root: root, cache: NewReadCache()}
}

// Root returns the .memory root path.
func (mm *MoteManager) Root() string {
	return mm.root
}

// LockMote returns a FileLock for a specific mote.
func (mm *MoteManager) LockMote(moteID string) (*FileLock, error) {
	path, err := mm.moteFilePath(moteID)
	if err != nil {
		return nil, err
	}
	lock := NewFileLock(path + ".lock")
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock mote %s: %w", moteID, err)
	}
	return lock, nil
}

// LockBatch returns a FileLock for the access batch file.
func (mm *MoteManager) LockBatch() (*FileLock, error) {
	lock := NewFileLock(filepath.Join(mm.root, ".batch.lock"))
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock batch: %w", err)
	}
	return lock, nil
}

// LockOps returns a FileLock for multi-file operations.
func (mm *MoteManager) LockOps() (*FileLock, error) {
	lock := NewFileLock(filepath.Join(mm.root, ".ops.lock"))
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock ops: %w", err)
	}
	return lock, nil
}

// TryLockDream attempts a non-blocking lock for dream cycles.
func (mm *MoteManager) TryLockDream() (*FileLock, bool, error) {
	lock := NewFileLock(filepath.Join(mm.root, ".dream.lock"))
	acquired, err := lock.TryLock()
	if err != nil {
		return nil, false, fmt.Errorf("try lock dream: %w", err)
	}
	if !acquired {
		return nil, false, nil
	}
	return lock, true, nil
}

func (mm *MoteManager) nodesDir() string {
	return filepath.Join(mm.root, "nodes")
}

func (mm *MoteManager) trashDir() string {
	return filepath.Join(mm.root, "trash")
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
	// Validate inputs
	if err := security.ValidateEnum(moteType, ValidTypes, "type"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	for _, tag := range opts.Tags {
		if err := security.ValidateTag(tag); err != nil {
			return nil, fmt.Errorf("invalid tag: %w", err)
		}
	}
	if opts.Weight != 0 {
		if err := security.ValidateWeight(opts.Weight); err != nil {
			return nil, err
		}
	}
	if opts.Origin != "" {
		if err := security.ValidateEnum(opts.Origin, ValidOrigins, "origin"); err != nil {
			return nil, err
		}
	}
	if opts.Size != "" {
		if err := security.ValidateEnum(opts.Size, ValidSizes, "size"); err != nil {
			return nil, err
		}
	}
	if opts.Body != "" {
		if err := security.ValidateBodySize(opts.Body); err != nil {
			return nil, err
		}
	}

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
	agentID := ResolveAgentID()
	m := &Mote{
		ID:            id,
		Type:          moteType,
		Status:        "active",
		Title:         title,
		Tags:          opts.Tags,
		Weight:        weight,
		Origin:        origin,
		CreatedAt:     now,
		AccessCount:   0,
		Body:          opts.Body,
		StrataCorpus:  opts.StrataCorpus,
		SourceIssue:   opts.SourceIssue,
		Parent:        opts.Parent,
		Acceptance:    opts.Acceptance,
		Size:          opts.Size,
		CreatedBy:     agentID,
		ModifiedBy:    agentID,
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

// Read loads a mote by ID, using the in-memory cache when the file is unchanged.
func (mm *MoteManager) Read(moteID string) (*Mote, error) {
	path, err := mm.moteFilePath(moteID)
	if err != nil {
		return nil, fmt.Errorf("get file path: %w", err)
	}
	if m, ok := mm.cache.Get(moteID, path); ok {
		return m, nil
	}
	m, err := ParseMote(path)
	if err != nil {
		return nil, err
	}
	mm.cache.Put(moteID, path, m)
	return m, nil
}

// Update applies field changes to a mote and persists them.
// Acquires a per-mote lock to prevent lost updates from concurrent agents.
func (mm *MoteManager) Update(moteID string, fields map[string]interface{}) error {
	lock, err := mm.LockMote(moteID)
	if err != nil {
		return err
	}
	defer lock.Unlock()

	return mm.updateUnlocked(moteID, fields)
}

// updateUnlocked applies field changes without acquiring the per-mote lock.
// Caller must hold the lock (or ops lock) before calling.
func (mm *MoteManager) updateUnlocked(moteID string, fields map[string]interface{}) error {
	m, err := mm.Read(moteID)
	if err != nil {
		return err
	}
	for k, v := range fields {
		switch k {
		case "status":
			s := v.(string)
			if err := security.ValidateEnum(s, ValidStatuses, "status"); err != nil {
				return err
			}
			m.Status = s
		case "title":
			s := v.(string)
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("title cannot be empty")
			}
			m.Title = s
		case "weight":
			w := v.(float64)
			if err := security.ValidateWeight(w); err != nil {
				return err
			}
			m.Weight = w
		case "tags":
			tags := v.([]string)
			for _, tag := range tags {
				if err := security.ValidateTag(tag); err != nil {
					return fmt.Errorf("invalid tag: %w", err)
				}
			}
			m.Tags = tags
		case "last_accessed":
			t := v.(time.Time)
			m.LastAccessed = &t
		case "access_count":
			m.AccessCount = v.(int)
		case "body":
			s := v.(string)
			if err := security.ValidateBodySize(s); err != nil {
				return err
			}
			m.Body = s
		case "deprecated_by":
			m.DeprecatedBy = v.(string)
		case "parent":
			m.Parent = v.(string)
		case "acceptance":
			m.Acceptance = v.([]string)
		case "acceptance_met":
			m.AcceptanceMet = v.([]bool)
		case "size":
			s := v.(string)
			if s != "" {
				if err := security.ValidateEnum(s, ValidSizes, "size"); err != nil {
					return err
				}
			}
			m.Size = s
		}
	}
	m.ModifiedBy = ResolveAgentID()
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
		if filters.Parent != "" && m.Parent != filters.Parent {
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

// Children returns all motes whose Parent field matches parentID.
func (mm *MoteManager) Children(parentID string) ([]*Mote, error) {
	motes, err := mm.ReadAllParallel()
	if err != nil {
		return nil, err
	}
	var children []*Mote
	for _, m := range motes {
		if m.Parent == parentID {
			children = append(children, m)
		}
	}
	return children, nil
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
			moteID := strings.TrimSuffix(name, ".md")
			if m, ok := mm.cache.Get(moteID, path); ok {
				results[idx] = result{mote: m, name: name}
				return
			}
			m, parseErr := ParseMote(path)
			if parseErr == nil {
				mm.cache.Put(moteID, path, m)
			}
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
// Uses flock to serialize across processes.
func (mm *MoteManager) AppendAccessBatch(moteID string) error {
	batchLock, err := mm.LockBatch()
	if err != nil {
		return err
	}
	defer batchLock.Unlock()

	mm.accessBatchMux.Lock()
	defer mm.accessBatchMux.Unlock()

	entry := AccessBatchEntry{
		MoteID:     moteID,
		AccessedAt: time.Now().UTC().Format(time.RFC3339),
		AgentID:    ResolveAgentID(),
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
// Uses flock + atomic rename to prevent TOCTOU races with concurrent appends.
func (mm *MoteManager) FlushAccessBatch() error {
	processingPath, err := mm.prepareBatchForProcessing()
	if err != nil {
		return err
	}
	if processingPath == "" {
		return nil
	}
	mm.processAndApplyBatch(processingPath)
	return nil
}

// FlushAccessBatchStats flushes the batch and returns stats (access count, mote count).
// Uses flock + atomic rename to prevent TOCTOU races.
func (mm *MoteManager) FlushAccessBatchStats() (accessCount, moteCount int, err error) {
	processingPath, err := mm.prepareBatchForProcessing()
	if err != nil {
		return 0, 0, err
	}
	if processingPath == "" {
		return 0, 0, nil
	}
	ac, mc := mm.processAndApplyBatch(processingPath)
	return ac, mc, nil
}

// prepareBatchForProcessing handles the lock/rename preamble shared by both flush methods.
// Returns the processing file path, or "" if there's nothing to process.
func (mm *MoteManager) prepareBatchForProcessing() (string, error) {
	batchPath := filepath.Join(mm.root, ".access_batch.jsonl")
	processingPath := batchPath + ".processing"

	// Process any leftover .processing file from a previous crash
	mm.processAndApplyBatch(processingPath)

	// Acquire batch lock, rename to .processing, release lock
	batchLock, err := mm.LockBatch()
	if err != nil {
		return "", err
	}

	if _, statErr := os.Stat(batchPath); os.IsNotExist(statErr) {
		batchLock.Unlock()
		return "", nil
	}

	if err := os.Rename(batchPath, processingPath); err != nil {
		batchLock.Unlock()
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	batchLock.Unlock() // New appends go to fresh file

	return processingPath, nil
}

// processAndApplyBatch reads a batch file, groups entries by moteID, updates motes,
// and removes the file. Returns (totalAccessCount, uniqueMoteCount).
func (mm *MoteManager) processAndApplyBatch(path string) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}

	type counts struct {
		lastAccessed time.Time
		count        int
	}
	grouped := map[string]*counts{}
	accessCount := 0

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
		if filePath, pathErr := mm.moteFilePath(moteID); pathErr == nil {
			_ = AtomicWrite(filePath, out, 0644)
		}
	}

	os.Remove(path)
	return accessCount, len(grouped)
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

// Delete soft-deletes a mote by moving it to trash/ and cleaning up edges.
func (mm *MoteManager) Delete(moteID string, im *IndexManager) error {
	m, err := mm.Read(moteID)
	if err != nil {
		return fmt.Errorf("read mote %s: %w", moteID, err)
	}

	// Set deleted_at
	now := time.Now().UTC()
	m.DeletedAt = &now

	data, err := SerializeMote(m)
	if err != nil {
		return fmt.Errorf("serialize mote: %w", err)
	}

	// Ensure trash/ exists
	if err := os.MkdirAll(mm.trashDir(), 0755); err != nil {
		return fmt.Errorf("create trash dir: %w", err)
	}

	// Write to trash/
	trashPath := filepath.Join(mm.trashDir(), moteID+".md")
	if err := AtomicWrite(trashPath, data, 0644); err != nil {
		return fmt.Errorf("write to trash: %w", err)
	}

	// Remove from nodes/
	nodePath, err := mm.moteFilePath(moteID)
	if err != nil {
		return fmt.Errorf("get node path: %w", err)
	}
	if err := os.Remove(nodePath); err != nil {
		return fmt.Errorf("remove from nodes: %w", err)
	}

	// Remove all edges involving this mote from the index
	mm.removeAllEdges(moteID, im)

	// Remove references to this mote from other motes' link slices
	mm.removeReferencesFromOtherMotes(moteID)

	return nil
}

// removeAllEdges removes all index edges where moteID is source or target.
func (mm *MoteManager) removeAllEdges(moteID string, im *IndexManager) {
	idx, err := im.Load()
	if err != nil || idx == nil {
		return
	}

	// Collect edges to remove (both directions)
	var toRemove []Edge
	for _, e := range idx.Edges {
		if e.Source == moteID || e.Target == moteID {
			toRemove = append(toRemove, e)
		}
	}
	for _, e := range toRemove {
		_ = im.RemoveEdge(e.Source, e.Target, e.EdgeType)
	}
}

// removeReferencesFromOtherMotes removes moteID from all link slices of other motes.
func (mm *MoteManager) removeReferencesFromOtherMotes(moteID string) {
	motes, err := mm.ReadAllParallel()
	if err != nil {
		return
	}

	linkTypes := []string{"depends_on", "blocks", "relates_to", "builds_on", "contradicts", "supersedes", "caused_by", "informed_by"}

	for _, m := range motes {
		modified := false
		for _, lt := range linkTypes {
			slice := GetLinkSlice(m, lt)
			if sliceContains(slice, moteID) {
				SetLinkSlice(m, lt, sliceRemove(slice, moteID))
				modified = true
			}
		}
		if modified {
			if data, err := SerializeMote(m); err == nil {
				if path, err := mm.moteFilePath(m.ID); err == nil {
					_ = AtomicWrite(path, data, 0644)
				}
			}
		}
	}
}

// Restore moves a mote from trash/ back to nodes/.
func (mm *MoteManager) Restore(moteID string) error {
	if err := security.ValidateMoteID(moteID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	trashPath := filepath.Join(mm.trashDir(), moteID+".md")
	m, err := ParseMote(trashPath)
	if err != nil {
		return fmt.Errorf("read from trash: %w", err)
	}

	// Clear deleted_at
	m.DeletedAt = nil

	data, err := SerializeMote(m)
	if err != nil {
		return fmt.Errorf("serialize mote: %w", err)
	}

	nodePath, err := mm.moteFilePath(moteID)
	if err != nil {
		return fmt.Errorf("get node path: %w", err)
	}
	if err := AtomicWrite(nodePath, data, 0644); err != nil {
		return fmt.Errorf("write to nodes: %w", err)
	}

	return os.Remove(trashPath)
}

// ListTrash returns all motes in the trash directory.
func (mm *MoteManager) ListTrash() ([]*Mote, error) {
	entries, err := os.ReadDir(mm.trashDir())
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
		m, err := ParseMote(filepath.Join(mm.trashDir(), entry.Name()))
		if err != nil {
			continue
		}
		motes = append(motes, m)
	}
	return motes, nil
}

// PurgeTrash permanently deletes trashed motes past the retention period.
// If all is true, deletes everything regardless of age.
// Returns the list of purged mote IDs.
func (mm *MoteManager) PurgeTrash(retentionDays int, all bool) ([]string, error) {
	motes, err := mm.ListTrash()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var purged []string

	for _, m := range motes {
		if !all {
			if m.DeletedAt == nil {
				continue
			}
			expiry := m.DeletedAt.Add(time.Duration(retentionDays) * 24 * time.Hour)
			if now.Before(expiry) {
				continue
			}
		}
		trashPath := filepath.Join(mm.trashDir(), m.ID+".md")
		if err := os.Remove(trashPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not purge %s: %v\n", m.ID, err)
			continue
		}
		purged = append(purged, m.ID)
	}

	return purged, nil
}

// LinkLocked acquires the ops lock, then per-mote locks (in alphabetical ID order
// to prevent deadlocks), before performing the link operation.
func (mm *MoteManager) LinkLocked(sourceID, linkType, targetID string, im *IndexManager) error {
	opsLock, err := mm.LockOps()
	if err != nil {
		return err
	}
	defer opsLock.Unlock()

	return mm.Link(sourceID, linkType, targetID, im)
}

// UnlinkLocked acquires the ops lock before performing the unlink operation.
func (mm *MoteManager) UnlinkLocked(sourceID, linkType, targetID string, im *IndexManager) error {
	opsLock, err := mm.LockOps()
	if err != nil {
		return err
	}
	defer opsLock.Unlock()

	return mm.Unlink(sourceID, linkType, targetID, im)
}
