package strata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motes/internal/core"
)

// StrataManager coordinates corpus operations: ingest, query, list, remove.
type StrataManager struct {
	root    string // .memory/ root
	config  core.StrataConfig
	chunker *Chunker
}

// CorpusManifest holds metadata about a strata corpus.
type CorpusManifest struct {
	Name         string            `json:"name"`
	SourcePaths  []string          `json:"source_paths"`
	SourceHashes map[string]string `json:"source_hashes,omitempty"`
	ChunkCount   int               `json:"chunk_count"`
	CreatedAt    string            `json:"created_at"`
	LastUpdated  string            `json:"last_updated"`
}

// CorpusInfo is manifest + associated anchor mote ID.
type CorpusInfo struct {
	Manifest CorpusManifest
	AnchorID string
}

// QueryLogEntry records a strata query for dream cycle analysis.
type QueryLogEntry struct {
	Timestamp    string  `json:"timestamp"`
	Query        string  `json:"query"`
	Corpus       string  `json:"corpus"`
	ResultsCount int     `json:"results_count"`
	TopChunkID   string  `json:"top_chunk_id,omitempty"`
	TopScore     float64 `json:"top_score"`
}

// NewStrataManager creates a manager from the .memory root and strata config.
func NewStrataManager(root string, cfg core.StrataConfig) *StrataManager {
	return &StrataManager{
		root:   root,
		config: cfg,
		chunker: NewChunker(
			cfg.Chunking.Strategy,
			cfg.Chunking.MaxChunkTokens,
			cfg.Chunking.OverlapTokens,
		),
	}
}

func (sm *StrataManager) strataDir() string {
	return filepath.Join(sm.root, "strata")
}

func (sm *StrataManager) corpusDir(name string) string {
	return filepath.Join(sm.strataDir(), name)
}

// supportedExt returns true for file types we can ingest.
var supportedExts = map[string]bool{
	".md": true, ".txt": true, ".go": true, ".py": true,
	".js": true, ".ts": true, ".rs": true, ".sh": true,
	".rb": true, ".java": true, ".c": true, ".cpp": true,
	".h": true, ".css": true, ".html": true, ".yaml": true,
	".yml": true, ".json": true, ".toml": true, ".xml": true,
}

// AddCorpus ingests files from paths into a named corpus.
func (sm *StrataManager) AddCorpus(name string, paths []string, createAnchor bool, mm *core.MoteManager) error {
	corpusDir := sm.corpusDir(name)
	if err := os.MkdirAll(corpusDir, 0755); err != nil {
		return fmt.Errorf("create corpus dir: %w", err)
	}

	var allChunks []Chunk
	var sourcePaths []string

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("stat %s: %w", p, err)
		}

		if info.IsDir() {
			err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				ext := strings.ToLower(filepath.Ext(path))
				if !supportedExts[ext] {
					return nil
				}
				chunks, readErr := sm.chunkFile(path, name)
				if readErr != nil {
					return readErr
				}
				allChunks = append(allChunks, chunks...)
				sourcePaths = append(sourcePaths, path)
				return nil
			})
			if err != nil {
				return fmt.Errorf("walk %s: %w", p, err)
			}
		} else {
			chunks, err := sm.chunkFile(p, name)
			if err != nil {
				return err
			}
			allChunks = append(allChunks, chunks...)
			sourcePaths = append(sourcePaths, p)
		}
	}

	if len(allChunks) == 0 {
		return fmt.Errorf("no content found in provided paths")
	}

	// Write chunks.jsonl
	if err := sm.writeChunks(name, allChunks); err != nil {
		return err
	}

	// Build and write BM25 index
	bm25Idx := BuildBM25Index(allChunks)
	if err := sm.writeBM25(name, bm25Idx); err != nil {
		return err
	}

	// Compute source hashes
	sourceHashes := make(map[string]string, len(sourcePaths))
	for _, p := range sourcePaths {
		if h, err := fileHash(p); err == nil {
			sourceHashes[p] = h
		}
	}

	// Write manifest
	now := time.Now().UTC().Format(time.RFC3339)
	manifest := CorpusManifest{
		Name:         name,
		SourcePaths:  sourcePaths,
		SourceHashes: sourceHashes,
		ChunkCount:   len(allChunks),
		CreatedAt:    now,
		LastUpdated:  now,
	}

	// Check for existing manifest to preserve CreatedAt
	existing, err := sm.loadManifest(name)
	if err == nil {
		manifest.CreatedAt = existing.CreatedAt
	}

	if err := sm.writeManifest(name, manifest); err != nil {
		return err
	}

	// Create anchor mote
	if createAnchor && mm != nil {
		_, err := mm.Create("anchor", name+" reference", core.CreateOpts{
			Weight:       0.3,
			Tags:         []string{name},
			StrataCorpus: name,
			Body:         fmt.Sprintf("Anchor mote for strata corpus '%s'. %d chunks from %d sources.", name, len(allChunks), len(sourcePaths)),
		})
		if err != nil {
			return fmt.Errorf("create anchor mote: %w", err)
		}
	}

	return nil
}

func (sm *StrataManager) chunkFile(path, corpus string) ([]Chunk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return sm.chunker.ChunkFile(string(data), path, corpus), nil
}

// Query searches a specific corpus for relevant chunks.
func (sm *StrataManager) Query(topic, corpus string, topK int) ([]ChunkResult, error) {
	bm25Idx, err := sm.loadBM25(corpus)
	if err != nil {
		return nil, fmt.Errorf("load bm25 for %s: %w", corpus, err)
	}

	chunks, err := sm.loadChunks(corpus)
	if err != nil {
		return nil, fmt.Errorf("load chunks for %s: %w", corpus, err)
	}

	results := bm25Idx.Search(topic, topK)

	// Hydrate chunk text from chunks.jsonl
	chunkMap := make(map[string]Chunk, len(chunks))
	for _, c := range chunks {
		chunkMap[c.ID] = c
	}
	for i := range results {
		if full, ok := chunkMap[results[i].Chunk.ID]; ok {
			results[i].Chunk = full
		}
	}

	// Log query
	sm.logQuery(topic, corpus, results)

	return results, nil
}

// QueryAll searches all corpora and interleaves results by score.
func (sm *StrataManager) QueryAll(topic string, topK int) ([]ChunkResult, error) {
	corpora, err := sm.ListCorpora()
	if err != nil {
		return nil, err
	}
	if len(corpora) == 0 {
		return nil, nil
	}

	var allResults []ChunkResult
	for _, c := range corpora {
		results, err := sm.Query(topic, c.Manifest.Name, topK)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}

	// Sort by score, cap at topK
	for i := range allResults {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[j].Score > allResults[i].Score {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}
	if len(allResults) > topK {
		allResults = allResults[:topK]
	}
	return allResults, nil
}

// ListCorpora returns info about all available corpora.
func (sm *StrataManager) ListCorpora() ([]CorpusInfo, error) {
	strataDir := sm.strataDir()
	entries, err := os.ReadDir(strataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var corpora []CorpusInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := sm.loadManifest(entry.Name())
		if err != nil {
			continue
		}
		corpora = append(corpora, CorpusInfo{Manifest: *manifest})
	}
	return corpora, nil
}

// UpdateCorpus re-ingests changed files for an existing corpus.
// Unchanged files (by SHA256 hash) are skipped.
func (sm *StrataManager) UpdateCorpus(name string) (changed int, err error) {
	manifest, err := sm.loadManifest(name)
	if err != nil {
		return 0, fmt.Errorf("load manifest for %s: %w", name, err)
	}

	var allChunks []Chunk
	var newPaths []string
	newHashes := make(map[string]string)

	for _, p := range manifest.SourcePaths {
		info, statErr := os.Stat(p)
		if statErr != nil {
			continue // deleted file — skip
		}
		if info.IsDir() {
			continue
		}
		h, _ := fileHash(p)
		newHashes[p] = h

		// Skip unchanged
		if manifest.SourceHashes != nil && manifest.SourceHashes[p] == h {
			// Reload existing chunks for this file
			existingChunks, _ := sm.loadChunks(name)
			base := filepath.Base(p)
			for _, c := range existingChunks {
				if filepath.Base(c.SourcePath) == base {
					allChunks = append(allChunks, c)
				}
			}
			newPaths = append(newPaths, p)
			continue
		}

		chunks, readErr := sm.chunkFile(p, name)
		if readErr != nil {
			continue
		}
		allChunks = append(allChunks, chunks...)
		newPaths = append(newPaths, p)
		changed++
	}

	if len(allChunks) == 0 {
		return 0, fmt.Errorf("no content after update")
	}

	if err := sm.writeChunks(name, allChunks); err != nil {
		return 0, err
	}
	bm25Idx := BuildBM25Index(allChunks)
	if err := sm.writeBM25(name, bm25Idx); err != nil {
		return 0, err
	}

	manifest.SourcePaths = newPaths
	manifest.SourceHashes = newHashes
	manifest.ChunkCount = len(allChunks)
	manifest.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	return changed, sm.writeManifest(name, *manifest)
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// RemoveCorpus deletes a corpus and its files.
func (sm *StrataManager) RemoveCorpus(name string) error {
	dir := sm.corpusDir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("corpus %q not found", name)
	}
	return os.RemoveAll(dir)
}

// File I/O helpers

func (sm *StrataManager) writeChunks(corpus string, chunks []Chunk) error {
	path := filepath.Join(sm.corpusDir(corpus), "chunks.jsonl")
	var buf strings.Builder
	for _, c := range chunks {
		line, _ := json.Marshal(c)
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return core.AtomicWrite(path, []byte(buf.String()), 0644)
}

func (sm *StrataManager) loadChunks(corpus string) ([]Chunk, error) {
	path := filepath.Join(sm.corpusDir(corpus), "chunks.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var chunks []Chunk
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var c Chunk
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

func (sm *StrataManager) writeBM25(corpus string, idx *BM25Index) error {
	path := filepath.Join(sm.corpusDir(corpus), "bm25.json")
	data, _ := json.Marshal(idx)
	return core.AtomicWrite(path, data, 0644)
}

func (sm *StrataManager) loadBM25(corpus string) (*BM25Index, error) {
	path := filepath.Join(sm.corpusDir(corpus), "bm25.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx BM25Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

func (sm *StrataManager) writeManifest(corpus string, m CorpusManifest) error {
	path := filepath.Join(sm.corpusDir(corpus), "manifest.json")
	data, _ := json.MarshalIndent(m, "", "  ")
	return core.AtomicWrite(path, data, 0644)
}

func (sm *StrataManager) loadManifest(corpus string) (*CorpusManifest, error) {
	path := filepath.Join(sm.corpusDir(corpus), "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m CorpusManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (sm *StrataManager) logQuery(topic, corpus string, results []ChunkResult) {
	logPath := filepath.Join(sm.strataDir(), "query_log.jsonl")
	entry := QueryLogEntry{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Query:        topic,
		Corpus:       corpus,
		ResultsCount: len(results),
	}
	if len(results) > 0 {
		entry.TopChunkID = results[0].Chunk.ID
		entry.TopScore = results[0].Score
	}
	line, _ := json.Marshal(entry)
	line = append(line, '\n')
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(line)
}
