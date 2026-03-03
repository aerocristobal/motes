package dream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"motes/internal/core"
)

// VisionWriter manages reading and writing vision JSONL files.
type VisionWriter struct {
	dreamDir string
}

// NewVisionWriter creates a vision writer for the given dream directory.
func NewVisionWriter(dreamDir string) *VisionWriter {
	return &VisionWriter{dreamDir: dreamDir}
}

// WriteDrafts appends visions to the draft file.
func (vw *VisionWriter) WriteDrafts(visions []Vision) error {
	path := filepath.Join(vw.dreamDir, "visions_draft.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, v := range visions {
		line, _ := json.Marshal(v)
		f.Write(line)
		f.Write([]byte{'\n'})
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
		line, _ := json.Marshal(v)
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
}

// NewVisionReviewer creates a reviewer.
func NewVisionReviewer(vw *VisionWriter, mm *core.MoteManager, im *core.IndexManager) *VisionReviewer {
	return &VisionReviewer{visions: vw, mm: mm, im: im}
}

// Review runs the interactive review loop.
func (vr *VisionReviewer) Review() (*ReviewResult, error) {
	visions := vr.visions.ReadFinal()
	if len(visions) == 0 {
		fmt.Println("No pending visions.")
		return &ReviewResult{}, nil
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
	fmt.Printf("  Sources:  %s\n", strings.Join(v.SourceMotes, ", "))
	if len(v.TargetMotes) > 0 {
		fmt.Printf("  Targets:  %s\n", strings.Join(v.TargetMotes, ", "))
	}
	if v.LinkType != "" {
		fmt.Printf("  Link:     %s\n", v.LinkType)
	}
	fmt.Printf("  Reason:   %s\n", v.Rationale)
}

func (vr *VisionReviewer) apply(v Vision) error {
	switch v.Type {
	case "link_suggestion":
		if len(v.SourceMotes) == 0 || len(v.TargetMotes) == 0 || v.LinkType == "" {
			return fmt.Errorf("link vision missing required fields")
		}
		return vr.mm.Link(v.SourceMotes[0], v.LinkType, v.TargetMotes[0], vr.im)
	case "staleness":
		if v.Action == "deprecate" && len(v.SourceMotes) > 0 {
			return vr.mm.Deprecate(v.SourceMotes[0], "")
		}
	case "tag_refinement":
		// Tag changes require manual review — just log for now
		fmt.Printf("  -> Tag refinement noted. Manual action required.\n")
	case "contradiction":
		fmt.Printf("  -> Contradiction flagged. Manual resolution required.\n")
	case "compression":
		fmt.Printf("  -> Compression suggested. Manual action required.\n")
	case "signal":
		fmt.Printf("  -> Signal pattern noted for config update.\n")
	}
	return nil
}
