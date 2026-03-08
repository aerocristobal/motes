package dream

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	// Stage 2: Batch reasoning (Claude Sonnet, parallel)
	type batchResult struct {
		index   int
		visions []Vision
		updates LucidLogUpdates
		err     error
	}

	results := make([]batchResult, len(batches))
	maxConcurrent := do.config.Batching.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, batch := range batches {
		wg.Add(1)
		go func(i int, batch Batch) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("  Batch %d/%d (%s, %d motes)...\n", i+1, len(batches), batch.Phase, len(batch.MoteIDs))
			prompt := do.prompts.BuildBatchPrompt(batch, do.lucidLog)
			response, err := do.invoker.Invoke(prompt, "sonnet")
			if err != nil {
				results[i] = batchResult{index: i, err: err}
				return
			}
			visions, updates, err := do.parser.ParseBatchResponse(response)
			if err != nil && strings.Contains(err.Error(), "no JSON found") {
				do.logFailedResponse(i+1, response)
				fmt.Fprintf(os.Stderr, "  warning: batch %d no JSON, retrying...\n", i+1)
				response, err = do.invoker.Invoke(prompt, "sonnet")
				if err == nil {
					visions, updates, err = do.parser.ParseBatchResponse(response)
				}
			}
			results[i] = batchResult{index: i, visions: visions, updates: updates, err: err}
		}(i, batch)
	}
	wg.Wait()

	// Merge results sequentially
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "  warning: batch %d failed: %v\n", r.index+1, r.err)
			do.lucidLog.RecordBatchFailure(r.index, r.err.Error())
			continue
		}
		if err := do.visions.WriteDrafts(r.visions); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: failed to write draft visions: %v\n", err)
		}
		do.lucidLog.Update(r.updates)
		fmt.Printf("  Batch %d: %d visions\n", r.index+1, len(r.visions))
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

// AutoApply applies pending visions above the confidence threshold, deferring the rest.
func (do *DreamOrchestrator) AutoApply(cfg *core.Config) (applied int, failed int, deferred int, err error) {
	mm := core.NewMoteManager(do.root)
	im := core.NewIndexManager(do.root)
	if _, err := im.Load(); err != nil {
		return 0, 0, 0, fmt.Errorf("load index: %w", err)
	}

	visions := do.visions.ReadFinal()
	if len(visions) == 0 {
		return 0, 0, 0, nil
	}

	// Load feedback stats for confidence scoring
	stats := GetStats(do.root)

	threshold := cfg.Dream.ConfidenceThreshold
	if threshold <= 0 {
		threshold = 0.6
	}

	// Score and split visions
	var toApply, toDefer []Vision
	for i := range visions {
		affectedIDs := AffectedMoteIDs(visions[i])
		preScores := SnapshotScores(mm, cfg, affectedIDs)
		visions[i].Confidence = ScoreConfidence(visions[i], stats, preScores)

		if visions[i].Confidence >= threshold {
			toApply = append(toApply, visions[i])
		} else {
			toDefer = append(toDefer, visions[i])
		}
	}

	// Apply high-confidence visions
	for _, v := range toApply {
		affectedIDs := AffectedMoteIDs(v)
		preScores := SnapshotScores(mm, cfg, affectedIDs)

		if applyErr := ApplyVision(v, mm, im, do.root, cfg); applyErr != nil {
			fmt.Fprintf(os.Stderr, "  warning: auto-apply failed: %v\n", applyErr)
			failed++
		} else {
			applied++
			RecordApplied(do.root, v, preScores)
		}
	}

	deferred = len(toDefer)

	// Write deferred visions back, or remove file if none remain
	if len(toDefer) > 0 {
		_ = do.visions.WriteFinal(toDefer)
	} else {
		os.Remove(filepath.Join(do.root, "dream", "visions.jsonl"))
	}

	return applied, failed, deferred, nil
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

func (do *DreamOrchestrator) logFailedResponse(batch int, raw string) {
	logPath := filepath.Join(do.root, "dream", "failed_responses.jsonl")
	preview := raw
	if len(preview) > 2000 {
		preview = preview[:2000]
	}
	entry := struct {
		Timestamp       string `json:"timestamp"`
		Batch           int    `json:"batch"`
		ResponsePreview string `json:"response_preview"`
		ResponseLen     int    `json:"response_len"`
	}{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Batch:           batch,
		ResponsePreview: preview,
		ResponseLen:     len(raw),
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
