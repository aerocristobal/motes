package dream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"motes/internal/core"
	"motes/internal/security"
)

// execCommand wraps exec.Command for testability.
var execCommand = exec.Command

// VisionWriter manages reading and writing vision JSONL files.
type VisionWriter struct {
	dreamDir string
	writeMux sync.Mutex // Protects vision file writes
}

// NewVisionWriter creates a vision writer for the given dream directory.
func NewVisionWriter(dreamDir string) *VisionWriter {
	return &VisionWriter{dreamDir: dreamDir}
}

// WriteDrafts appends visions to the draft file.
func (vw *VisionWriter) WriteDrafts(visions []Vision) error {
	vw.writeMux.Lock()
	defer vw.writeMux.Unlock()

	path := filepath.Join(vw.dreamDir, "visions_draft.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, v := range visions {
		line, err := json.Marshal(v)
		if err != nil {
			continue // Skip if marshal fails
		}
		if _, err := f.Write(line); err != nil {
			continue // Skip if write fails
		}
		_, _ = f.Write([]byte{'\n'}) // Newline write is non-critical
	}
	return nil
}

// ReadDrafts reads all draft visions.
func (vw *VisionWriter) ReadDrafts() []Vision {
	return vw.readVisionFile(filepath.Join(vw.dreamDir, "visions_draft.jsonl"))
}

// WriteFinal writes the reconciled visions, replacing any existing file.
func (vw *VisionWriter) WriteFinal(visions []Vision) error {
	path := filepath.Join(vw.dreamDir, "visions.jsonl")
	var buf strings.Builder
	for _, v := range visions {
		line, err := json.Marshal(v)
		if err != nil {
			continue // Skip if marshal fails
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return core.AtomicWrite(path, []byte(buf.String()), 0644)
}

// ReadFinal reads all pending final visions.
func (vw *VisionWriter) ReadFinal() []Vision {
	return vw.readVisionFile(filepath.Join(vw.dreamDir, "visions.jsonl"))
}

// ClearDrafts removes the draft file.
func (vw *VisionWriter) ClearDrafts() {
	os.Remove(filepath.Join(vw.dreamDir, "visions_draft.jsonl"))
}

func (vw *VisionWriter) readVisionFile(path string) []Vision {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var visions []Vision
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var v Vision
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			continue
		}
		visions = append(visions, v)
	}
	return visions
}

// VisionReviewer presents visions for interactive terminal review.
type VisionReviewer struct {
	visions *VisionWriter
	mm      *core.MoteManager
	im      *core.IndexManager
	root    string
	cfg     *core.Config
	ic      *impactContext // scoring impact context, built once in Review()
}

// NewVisionReviewer creates a reviewer.
func NewVisionReviewer(vw *VisionWriter, mm *core.MoteManager, im *core.IndexManager) *VisionReviewer {
	return &VisionReviewer{visions: vw, mm: mm, im: im}
}

// NewVisionReviewerWithConfig creates a reviewer with config access for signal/constellation apply.
func NewVisionReviewerWithConfig(vw *VisionWriter, mm *core.MoteManager, im *core.IndexManager, root string, cfg *core.Config) *VisionReviewer {
	return &VisionReviewer{visions: vw, mm: mm, im: im, root: root, cfg: cfg}
}

// Review runs the interactive review loop.
func (vr *VisionReviewer) Review() (*ReviewResult, error) {
	visions := vr.visions.ReadFinal()
	if len(visions) == 0 {
		fmt.Println("No pending visions.")
		return &ReviewResult{}, nil
	}

	// Build scoring impact context once for use across all visions.
	if vr.cfg != nil {
		if motes, err := vr.mm.ReadAllWithGlobal(); err == nil {
			idx := vr.im.BuildInMemory(motes)
			scorer := core.NewScoreEngine(vr.cfg.Scoring, idx.ConceptStats)
			vr.ic = &impactContext{motes: motes, idx: idx, scorer: scorer, cfg: vr.cfg}
		}
	}

	result := &ReviewResult{}
	var deferred []Vision
	reader := bufio.NewReader(os.Stdin)

	for i, v := range visions {
		fmt.Printf("\n=== Vision %d/%d ===\n", i+1, len(visions))
		vr.display(v)
		fmt.Print("\n[a]ccept / [e]dit / [r]eject / [d]efer: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "a":
			if err := vr.apply(v); err != nil {
				fmt.Fprintf(os.Stderr, "warning: apply failed: %v\n", err)
				deferred = append(deferred, v)
			} else {
				result.Accepted++
			}
		case "e":
			edited, err := vr.editVision(v)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: edit failed: %v\n", err)
				deferred = append(deferred, v)
				result.Deferred++
			} else if err := vr.apply(edited); err != nil {
				fmt.Fprintf(os.Stderr, "warning: apply edited vision failed: %v\n", err)
				deferred = append(deferred, v)
				result.Deferred++
			} else {
				result.Accepted++
			}
		case "r":
			result.Rejected++
		case "d", "":
			deferred = append(deferred, v)
			result.Deferred++
		default:
			// Treat unknown as defer
			deferred = append(deferred, v)
			result.Deferred++
		}
	}

	// Write remaining deferred visions back
	if len(deferred) > 0 {
		_ = vr.visions.WriteFinal(deferred)
	} else {
		os.Remove(filepath.Join(vr.visions.dreamDir, "visions.jsonl"))
	}

	return result, nil
}

func (vr *VisionReviewer) display(v Vision) {
	fmt.Printf("  Type:     %s\n", v.Type)
	fmt.Printf("  Action:   %s\n", v.Action)
	fmt.Printf("  Severity: %s\n", v.Severity)
	if v.Confidence > 0 {
		fmt.Printf("  Confidence: %.0f%%\n", v.Confidence*100)
	}
	fmt.Printf("  Sources:  %s\n", strings.Join(v.SourceMotes, ", "))
	if len(v.TargetMotes) > 0 {
		fmt.Printf("  Targets:  %s\n", strings.Join(v.TargetMotes, ", "))
	}
	if v.LinkType != "" {
		fmt.Printf("  Link:     %s\n", v.LinkType)
	}
	fmt.Printf("  Reason:   %s\n", v.Rationale)
	if vr.ic != nil {
		if impact := computeVisionImpact(v, vr.ic); impact != "" {
			fmt.Printf("  Impact:   %s\n", impact)
		}
	}
}

func (vr *VisionReviewer) editVision(v Vision) (Vision, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return v, fmt.Errorf("marshal vision: %w", err)
	}

	tmp, err := os.CreateTemp("", "vision-*.json")
	if err != nil {
		return v, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return v, fmt.Errorf("write temp: %w", err)
	}
	tmp.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	if err := security.ValidateCommand(editor); err != nil {
		return v, fmt.Errorf("invalid EDITOR command: %w", err)
	}
	cmd := execCommand(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return v, fmt.Errorf("editor: %w", err)
	}

	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return v, fmt.Errorf("read edited: %w", err)
	}

	var result Vision
	if err := json.Unmarshal(edited, &result); err != nil {
		return v, fmt.Errorf("parse edited vision: %w", err)
	}
	return result, nil
}

func (vr *VisionReviewer) apply(v Vision) error {
	return ApplyVision(v, vr.mm, vr.im, vr.root, vr.cfg)
}

// ApplyVision applies a single vision to the knowledge graph.
func ApplyVision(v Vision, mm *core.MoteManager, im *core.IndexManager, root string, cfg *core.Config) error {
	switch v.Type {
	case "link_suggestion":
		if len(v.SourceMotes) == 0 || len(v.TargetMotes) == 0 || v.LinkType == "" {
			return fmt.Errorf("link vision missing required fields")
		}
		if err := mm.Link(v.SourceMotes[0], v.LinkType, v.TargetMotes[0], im); err != nil {
			return err
		}
		return insertBodyRef(mm, v.SourceMotes[0], v.TargetMotes[0])
	case "staleness":
		if v.Action == "deprecate" && len(v.SourceMotes) > 0 {
			return mm.Deprecate(v.SourceMotes[0], "")
		}
	case "contradiction":
		if len(v.SourceMotes) < 2 {
			return fmt.Errorf("contradiction vision needs at least 2 source motes")
		}
		return mm.Link(v.SourceMotes[0], "contradicts", v.SourceMotes[1], im)
	case "tag_refinement":
		if len(v.SourceMotes) == 0 || len(v.Tags) == 0 {
			return fmt.Errorf("tag_refinement vision needs source motes and tags")
		}
		m, err := mm.Read(v.SourceMotes[0])
		if err != nil {
			return fmt.Errorf("read mote %s: %w", v.SourceMotes[0], err)
		}
		m.Tags = v.Tags
		data, err := core.SerializeMote(m)
		if err != nil {
			return fmt.Errorf("serialize: %w", err)
		}
		path, err := mm.MoteFilePath(v.SourceMotes[0])
		if err != nil {
			return fmt.Errorf("get file path: %w", err)
		}
		return core.AtomicWrite(path, data, 0644)
	case "compression":
		if len(v.SourceMotes) == 0 || v.Rationale == "" {
			return fmt.Errorf("compression vision needs source mote and rationale as compressed body")
		}
		m, err := mm.Read(v.SourceMotes[0])
		if err != nil {
			return fmt.Errorf("read mote %s: %w", v.SourceMotes[0], err)
		}
		m.Body = v.Rationale
		data, err := core.SerializeMote(m)
		if err != nil {
			return fmt.Errorf("serialize: %w", err)
		}
		path, err := mm.MoteFilePath(v.SourceMotes[0])
		if err != nil {
			return fmt.Errorf("get file path: %w", err)
		}
		return core.AtomicWrite(path, data, 0644)
	case "constellation":
		if len(v.Tags) == 0 || len(v.SourceMotes) == 0 {
			return fmt.Errorf("constellation vision needs tags and source motes")
		}
		tag := v.Tags[0]
		title := fmt.Sprintf("Constellation: %s", tag)
		body := fmt.Sprintf("Hub for the **%s** theme.\n\nMembers:\n", tag)
		for _, id := range v.SourceMotes {
			body += fmt.Sprintf("- [[%s]]\n", id)
		}
		hub, err := mm.Create("constellation", title, core.CreateOpts{
			Tags:   []string{tag},
			Weight: 0.6,
			Body:   body,
		})
		if err != nil {
			return fmt.Errorf("create constellation: %w", err)
		}
		for _, memberID := range v.SourceMotes {
			_ = mm.Link(hub.ID, "relates_to", memberID, im)
		}
		fmt.Printf("  -> Created constellation %s for tag %q\n", hub.ID, tag)
	case "merge_suggestion":
		if len(v.SourceMotes) < 3 {
			return fmt.Errorf("merge_suggestion needs at least 3 source motes, got %d", len(v.SourceMotes))
		}
		if v.Rationale == "" {
			return fmt.Errorf("merge_suggestion needs rationale as merged body text")
		}
		title, body := splitTitleBody(v.Rationale)

		// Determine majority type from source motes
		typeCounts := map[string]int{}
		for _, id := range v.SourceMotes {
			m, err := mm.Read(id)
			if err != nil {
				continue
			}
			typeCounts[m.Type]++
		}
		majorityType := "lesson"
		maxCount := 0
		for t, c := range typeCounts {
			if c > maxCount {
				maxCount = c
				majorityType = t
			}
		}

		hub, err := mm.Create(majorityType, title, core.CreateOpts{
			Tags:   v.Tags,
			Weight: 0.7,
			Body:   body,
		})
		if err != nil {
			return fmt.Errorf("create merged mote: %w", err)
		}

		// Build cluster set for link migration filtering
		clusterSet := make(map[string]bool, len(v.SourceMotes))
		for _, id := range v.SourceMotes {
			clusterSet[id] = true
		}

		// Load edge index for link migration
		idx, _ := im.Load()

		// Supersede each source mote (auto-deprecates) and migrate links
		for _, srcID := range v.SourceMotes {
			if err := mm.Link(hub.ID, "supersedes", srcID, im); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: supersede link %s->%s failed: %v\n", hub.ID, srcID, err)
			}
			if idx != nil {
				migrateLinks(mm, im, idx, srcID, hub.ID, clusterSet)
			}
		}
		fmt.Printf("  -> Merged %d motes into %s\n", len(v.SourceMotes), hub.ID)
	case "summarize":
		if len(v.SourceMotes) < 3 {
			return fmt.Errorf("summarize needs at least 3 source motes, got %d", len(v.SourceMotes))
		}
		if v.Rationale == "" {
			return fmt.Errorf("summarize needs rationale as summary body text")
		}
		title, body := splitTitleBody(v.Rationale)

		hub, err := mm.Create("context", title, core.CreateOpts{
			Tags:   v.Tags,
			Weight: 0.6,
			Body:   body,
		})
		if err != nil {
			return fmt.Errorf("create summary mote: %w", err)
		}
		for _, srcID := range v.SourceMotes {
			_ = mm.Link(hub.ID, "builds_on", srcID, im)
			// Archive source mote
			_ = mm.Update(srcID, core.UpdateOpts{Status: core.StringPtr("archived")})
		}
		fmt.Printf("  -> Summarized %d motes into %s\n", len(v.SourceMotes), hub.ID)
	case "action_extraction":
		if len(v.SourceMotes) == 0 || v.Rationale == "" {
			return fmt.Errorf("action_extraction vision needs source mote and rationale as action text")
		}
		return mm.Update(v.SourceMotes[0], core.UpdateOpts{
			Action: core.StringPtr(v.Rationale),
		})
	case "signal":
		if cfg == nil || root == "" {
			return fmt.Errorf("signal apply requires config access (use NewVisionReviewerWithConfig)")
		}
		signal := core.SignalConfig{
			Name:        v.Action,
			Type:        "co_access",
			Description: v.Rationale,
			TriggerTags: v.Tags,
			BoostTags:   v.Tags,
			BoostAmount: 0.3,
		}
		cfg.Priming.Signals = append(cfg.Priming.Signals, signal)
		if err := core.SaveConfig(root, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("  -> Added co_access signal %q to config\n", signal.Name)
	case "dominant_mote_review", "decay_risk":
		// Informational advisory visions — no automated mutation.
	}
	return nil
}

// splitTitleBody splits rationale text into title (first line) and body (rest).
func splitTitleBody(text string) (string, string) {
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+1:])
	}
	return text, ""
}

// migrateLinks re-creates inbound/outbound links from oldID onto newID,
// skipping edges where the other end is in clusterIDs (intra-cluster) or supersedes edges.
func migrateLinks(mm *core.MoteManager, im *core.IndexManager, idx *core.EdgeIndex, oldID, newID string, clusterIDs map[string]bool) {
	// Migrate outgoing links
	for _, e := range idx.Neighbors(oldID, nil) {
		if clusterIDs[e.Target] || e.EdgeType == "supersedes" || e.EdgeType == "superseded_by" {
			continue
		}
		_ = mm.Link(newID, e.EdgeType, e.Target, im)
	}
	// Migrate incoming links
	for _, e := range idx.Incoming(oldID, nil) {
		if clusterIDs[e.Source] || e.EdgeType == "supersedes" || e.EdgeType == "superseded_by" {
			continue
		}
		_ = mm.Link(e.Source, e.EdgeType, newID, im)
	}
}

// insertBodyRef appends a wiki-link to the source mote body if not already present.
func insertBodyRef(mm *core.MoteManager, sourceID, targetID string) error {
	m, err := mm.Read(sourceID)
	if err != nil {
		return fmt.Errorf("read mote %s: %w", sourceID, err)
	}
	ref := "[[" + targetID + "]]"
	if strings.Contains(m.Body, ref) {
		return nil
	}
	m.Body += "\nSee also: " + ref + "\n"
	data, err := core.SerializeMote(m)
	if err != nil {
		return fmt.Errorf("serialize: %w", err)
	}
	path, err := mm.MoteFilePath(sourceID)
	if err != nil {
		return fmt.Errorf("get file path: %w", err)
	}
	return core.AtomicWrite(path, data, 0644)
}
