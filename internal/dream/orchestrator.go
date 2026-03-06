package dream

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motes/internal/core"
)

// DreamOrchestrator coordinates the 4-stage dream pipeline.
type DreamOrchestrator struct {
	root     string
	config   core.DreamConfig
	scanner  *PreScanner
	batcher  *BatchConstructor
	prompts  *PromptBuilder
	invoker  *ClaudeInvoker
	parser   *ResponseParser
	lucidLog *LucidLog
	visions  *VisionWriter
}

// NewDreamOrchestrator creates an orchestrator with all components wired.
func NewDreamOrchestrator(root string, cfg *core.Config) *DreamOrchestrator {
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	dreamDir := filepath.Join(root, "dream")

	reader := func(id string) (*core.Mote, error) {
		return mm.Read(id)
	}

	return &DreamOrchestrator{
		root:     root,
		config:   cfg.Dream,
		scanner:  NewPreScanner(root, mm, im, cfg.Dream),
		batcher:  NewBatchConstructor(cfg.Dream.Batching, reader),
		prompts:  NewPromptBuilder(reader),
		invoker:  NewClaudeInvoker(cfg.Dream.Provider),
		parser:   NewResponseParser(),
		lucidLog: NewLucidLog(cfg.Dream.Journal.MaxTokens),
		visions:  NewVisionWriter(dreamDir),
	}
}

// Run executes the dream cycle. If dryRun is true, stops after pre-scan.
func (do *DreamOrchestrator) Run(dryRun bool) (*DreamResult, error) {
	dreamDir := filepath.Join(do.root, "dream")
	if err := os.MkdirAll(dreamDir, 0755); err != nil {
		return nil, fmt.Errorf("create dream dir: %w", err)
	}

	start := time.Now()

	// Stage 1: Pre-scan (deterministic, no LLM)
	candidates, err := do.scanner.Scan()
	if err != nil {
		return nil, fmt.Errorf("pre-scan failed: %w", err)
	}
	if !candidates.HasWork() {
		result := &DreamResult{Status: "clean", Visions: 0}
		do.writeRunLog(result, time.Since(start))
		return result, nil
	}

	batches := do.batcher.Build(candidates)
	do.lucidLog.Initialize()

	if dryRun {
		do.printDryRun(candidates, batches)
		return &DreamResult{Status: "dry-run", Batches: len(batches)}, nil
	}

	// Clear previous draft visions
	do.visions.ClearDrafts()

	// Stage 2: Batch reasoning (Claude Sonnet)
	for i, batch := range batches {
		fmt.Printf("  Batch %d/%d (%s, %d motes)...\n", i+1, len(batches), batch.Phase, len(batch.MoteIDs))
		prompt := do.prompts.BuildBatchPrompt(batch, do.lucidLog)
		response, err := do.invoker.Invoke(prompt, "sonnet")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: batch %d failed: %v\n", i+1, err)
			do.lucidLog.RecordBatchFailure(i, err.Error())
			continue
		}
		batchVisions, logUpdates, err := do.parser.ParseBatchResponse(response)
		if err != nil && strings.Contains(err.Error(), "no JSON found") {
			fmt.Fprintf(os.Stderr, "  warning: batch %d no JSON, retrying...\n", i+1)
			response, err = do.invoker.Invoke(prompt, "sonnet")
			if err == nil {
				batchVisions, logUpdates, err = do.parser.ParseBatchResponse(response)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: batch %d parse error: %v\n", i+1, err)
			do.lucidLog.RecordBatchFailure(i, fmt.Sprintf("parse: %v", err))
			continue
		}
		if err := do.visions.WriteDrafts(batchVisions); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: failed to write draft visions: %v\n", err)
		}
		do.lucidLog.Update(logUpdates)
		fmt.Printf("  Batch %d: %d visions\n", i+1, len(batchVisions))
	}

	// Stage 3: Reconciliation (Claude Opus)
	var finalVisions []Vision
	if do.config.Reconciliation.Enabled {
		fmt.Println("  Reconciliation...")
		reconPrompt := do.prompts.BuildReconciliationPrompt(do.lucidLog)
		reconResponse, err := do.invoker.Invoke(reconPrompt, "opus")
		if err == nil {
			finalVisions, _ = do.parser.ParseReconciliationResponse(reconResponse)
		}
		if finalVisions == nil {
			finalVisions = do.visions.ReadDrafts()
		}
	} else {
		finalVisions = do.visions.ReadDrafts()
	}

	// Stage 4: Write final + log
	if err := do.visions.WriteFinal(finalVisions); err != nil {
		return nil, fmt.Errorf("write final visions: %w", err)
	}

	// Save lucid log
	llPath := filepath.Join(dreamDir, "lucid_log.json")
	_ = do.lucidLog.Save(llPath)

	result := &DreamResult{
		Status:  "complete",
		Batches: len(batches),
		Visions: len(finalVisions),
	}
	do.writeRunLog(result, time.Since(start))
	return result, nil
}

// AutoApply applies low-risk visions automatically, returning remaining high-risk ones.
func (do *DreamOrchestrator) AutoApply() (applied int, deferred int, err error) {
	mm := core.NewMoteManager(do.root)
	im := core.NewIndexManager(do.root)
	if _, err := im.Load(); err != nil {
		return 0, 0, fmt.Errorf("load index: %w", err)
	}

	visions := do.visions.ReadFinal()
	if len(visions) == 0 {
		return 0, 0, nil
	}

	var remaining []Vision
	for _, v := range visions {
		if isLowRisk(v) {
			if applyErr := applyVision(v, mm, im); applyErr != nil {
				fmt.Fprintf(os.Stderr, "  warning: auto-apply failed: %v\n", applyErr)
				remaining = append(remaining, v)
			} else {
				applied++
				do.logAutoApplied(v)
			}
		} else {
			remaining = append(remaining, v)
			deferred++
		}
	}

	// Write remaining visions back
	if len(remaining) > 0 {
		if err := do.visions.WriteFinal(remaining); err != nil {
			return applied, deferred, err
		}
	} else {
		os.Remove(filepath.Join(do.root, "dream", "visions.jsonl"))
	}

	return applied, deferred, nil
}

func isLowRisk(v Vision) bool {
	if v.Type != "link_suggestion" {
		return false
	}
	return v.LinkType == "relates_to" || v.LinkType == "informed_by" || v.LinkType == "builds_on"
}

func applyVision(v Vision, mm *core.MoteManager, im *core.IndexManager) error {
	if len(v.SourceMotes) == 0 || len(v.TargetMotes) == 0 || v.LinkType == "" {
		return fmt.Errorf("link vision missing required fields")
	}
	return mm.Link(v.SourceMotes[0], v.LinkType, v.TargetMotes[0], im)
}

func (do *DreamOrchestrator) logAutoApplied(v Vision) {
	logPath := filepath.Join(do.root, "dream", "auto_applied.jsonl")
	entry := struct {
		Timestamp string `json:"timestamp"`
		Vision    Vision `json:"vision"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Vision:    v,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte{'\n'})
}

// printDryRun outputs the scan results and planned batches without executing.
func (do *DreamOrchestrator) printDryRun(sr *ScanResult, batches []Batch) {
	fmt.Println("Dream cycle dry run:")
	fmt.Println()
	fmt.Printf("  Link candidates:          %d\n", len(sr.LinkCandidates))
	fmt.Printf("  Contradiction candidates: %d\n", len(sr.ContradictionCandidates))
	fmt.Printf("  Overloaded tags:          %d\n", len(sr.OverloadedTags))
	fmt.Printf("  Stale motes:              %d\n", len(sr.StaleMotes))
	fmt.Printf("  Constellation evolution:  %d\n", len(sr.ConstellationEvolution))
	fmt.Printf("  Compression candidates:   %d\n", len(sr.CompressionCandidates))
	fmt.Printf("  Uncrystallized issues:    %d\n", len(sr.UncrystallizedIssues))
	fmt.Printf("  Strata crystallization:   %d\n", len(sr.StrataCrystallization))
	fmt.Printf("  Signal candidates:        %d\n", len(sr.SignalCandidates))
	fmt.Println()
	fmt.Printf("  Planned batches: %d\n", len(batches))
	for i, b := range batches {
		fmt.Printf("    Batch %d: %s (%d motes, tasks: %v)\n",
			i+1, b.Phase, len(b.MoteIDs), b.Tasks)
	}
}

func (do *DreamOrchestrator) writeRunLog(result *DreamResult, elapsed time.Duration) {
	logPath := filepath.Join(do.root, "dream", "log.jsonl")
	entry := RunLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    result.Status,
		Batches:   result.Batches,
		Visions:   result.Visions,
		DurationS: elapsed.Seconds(),
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return // Skip logging if marshal fails
	}
	line = append(line, '\n')

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line) // Dream logging is non-critical
}
