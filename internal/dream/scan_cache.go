package dream

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"motes/internal/core"
)

// ScanCache tracks content hashes for incremental prescanning.
type ScanCache struct {
	Hashes map[string]string `json:"hashes"` // mote ID -> content hash
}

// ScanCachePath returns the path to the scan state file.
func ScanCachePath(root string) string {
	return filepath.Join(root, "dream", "scan_state.json")
}

// LoadScanCache loads the scan cache from disk.
func LoadScanCache(root string) *ScanCache {
	path := ScanCachePath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ScanCache{Hashes: map[string]string{}}
	}
	var sc ScanCache
	if err := json.Unmarshal(data, &sc); err != nil {
		return &ScanCache{Hashes: map[string]string{}}
	}
	if sc.Hashes == nil {
		sc.Hashes = map[string]string{}
	}
	return &sc
}

// SaveScanCache writes the scan cache to disk.
func SaveScanCache(root string, sc *ScanCache) error {
	path := ScanCachePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, data, 0644)
}

// ComputeMoteHash returns a deterministic hash of a mote's content.
func ComputeMoteHash(m *core.Mote) string {
	data, err := core.SerializeMote(m)
	if err != nil {
		// Fallback: hash title+body
		h := sha256.Sum256([]byte(m.Title + m.Body))
		return fmt.Sprintf("%x", h[:16])
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:16])
}

// FilterChanged returns motes whose content hash differs from the cache.
// It also updates the cache with current hashes and prunes deleted motes.
func FilterChanged(motes []*core.Mote, cache *ScanCache) []*core.Mote {
	currentIDs := make(map[string]bool, len(motes))
	var changed []*core.Mote

	for _, m := range motes {
		currentIDs[m.ID] = true
		hash := ComputeMoteHash(m)
		if cache.Hashes[m.ID] != hash {
			changed = append(changed, m)
		}
		cache.Hashes[m.ID] = hash
	}

	// Prune deleted motes
	for id := range cache.Hashes {
		if !currentIDs[id] {
			delete(cache.Hashes, id)
		}
	}

	return changed
}
