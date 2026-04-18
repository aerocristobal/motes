// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"motes/internal/core"
)

// DreamOrchestrator coordinates the 4-stage dream pipeline.
type DreamOrchestrator struct {
	root              string
	config            core.DreamConfig
	scanner           *PreScanner
	batcher           *BatchConstructor
	prompts           *PromptBuilder
	invoker           *ClaudeInvoker
	parser            *ResponseParser
	lucidLog          *LucidLog
	visions           *VisionWriter
	logger            *DreamLogger
	lastAvgConfidence float64 // Set by AutoApply
}

// NewDreamOrchestrator creates an orchestrator with all components wired.
func NewDreamOrchestrator(root string, cfg *core.Config) *DreamOrchestrator {
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	dreamDir := filepath.Join(root, "dream")

	reader := func(id string) (*core.Mote, error) {
		return mm.Read(id)
	}

	scanner := NewPreScanner(root, mm, im, cfg.Dream)
	scanner.SetScoringConfig(cfg.Scoring)

	return &DreamOrchestrator{
		root:     root,
		config:   cfg.Dream,
		scanner:  scanner,
		batcher:  NewBatchConstructor(cfg.Dream.Batching, reader),
		prompts:  NewPromptBuilder(reader),
		invoker:  NewClaudeInvoker(cfg.Dream.Provider),
		parser:   NewResponseParser(),
		lucidLog: NewLucidLog(cfg.Dream.Journal.MaxTokens),
		visions:  NewVisionWriter(dreamDir),
		logger:   NewDreamLogger(nil, false),
	}
}

// SetMoteLoader overrides the default mote loading for cross-scope dream scanning.
func (do *DreamOrchestrator) SetMoteLoader(loader func() ([]*core.Mote, error)) {
	do.scanner.SetMoteLoader(loader)
}

// LastAvgConfidence returns the average confidence from the most recent AutoApply call.
func (do *DreamOrchestrator) LastAvgConfidence() float64 {
	return do.lastAvgConfidence
}

// SetLogger configures the structured logger for machine-parseable output.
func (do *DreamOrchestrator) SetLogger(logger *DreamLogger) {
	if logger != nil {
		do.logger = logger
	}
}

// Run executes the dream cycle. If dryRun is true, stops after pre-scan.
func (do *DreamOrchestrator) Run(dryRun bool) (*DreamResult, error) {
	dreamDir := filepath.Join(do.root, "dream")
	if err := os.MkdirAll(dreamDir, 0755); err != nil {
		return nil, fmt.Errorf("create dream dir: %w", err)
	}

	start := time.Now()

	// Validate lens config before doing any work
	if err := validateLensConfig(do.config.Batching.LensMode); err != nil {
		return nil, err
	}

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
		index        int
		visions      []Vision
		updates      LucidLogUpdates
		inputTokens  int
		outputTokens int
		err          error
	}

	results := make([]batchResult, len(batches))
	maxConcurrent := do.config.Batching.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	scRuns := do.config.Batching.SelfConsistencyRuns
	if scRuns <= 0 {
		scRuns = 1
	}
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, batch := range batches {
		wg.Add(1)
		go func(i int, batch Batch) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("  Batch %d/%d (%s, %d motes", i+1, len(batches), batch.Phase, len(batch.MoteIDs))
			if scRuns > 1 {
				fmt.Printf(", %dx voting", scRuns)
			}
			fmt.Println(")...")

			do.logger.Log(LogEntry{
				Level:      "info",
				Phase:      batch.Phase,
				BatchIndex: i + 1,
				Message:    "batch start",
				MoteCount:  len(batch.MoteIDs),
			})

			batchStart := time.Now()

			lensMode := do.config.Batching.LensMode
			if lensMode.Enabled {
				// Lens mode: one run per lens, collect tagged union (no voting)
				type lensRunResult struct {
					lens         string
					visions      []Vision
					updates      LucidLogUpdates
					inputTokens  int
					outputTokens int
					err          error
				}
				lensResults := make([]lensRunResult, len(lensMode.Lenses))
				var lensWg sync.WaitGroup
				for l, lensName := range lensMode.Lenses {
					lensWg.Add(1)
					go func(l int, lensName string) {
						defer lensWg.Done()
						prompt := do.prompts.BuildBatchPrompt(batch, do.lucidLog, lensName)
						ir, err := do.invoker.Invoke(prompt, "sonnet")
						if err != nil {
							lensResults[l] = lensRunResult{lens: lensName, err: err}
							return
						}
						visions, updates, parseErr := do.parser.ParseBatchResponse(ir.Response)
						if parseErr != nil && strings.Contains(parseErr.Error(), "no JSON found") {
							do.logFailedResponse(i+1, ir.Response)
						}
						for vi := range visions {
							visions[vi].LensSource = lensName
						}
						lensResults[l] = lensRunResult{lens: lensName, visions: visions, updates: updates, inputTokens: ir.InputTokens, outputTokens: ir.OutputTokens}
					}(l, lensName)
				}
				lensWg.Wait()

				var allLensVisions [][]Vision
				var mergedUpdates LucidLogUpdates
				var batchInput, batchOutput int
				for _, lr := range lensResults {
					if lr.err != nil {
						fmt.Fprintf(os.Stderr, "  warning: lens %q failed for batch %d: %v\n", lr.lens, i+1, lr.err)
						continue
					}
					allLensVisions = append(allLensVisions, lr.visions)
					batchInput += lr.inputTokens
					batchOutput += lr.outputTokens
					mergedUpdates.ObservedPatterns = append(mergedUpdates.ObservedPatterns, lr.updates.ObservedPatterns...)
					mergedUpdates.Tensions = append(mergedUpdates.Tensions, lr.updates.Tensions...)
					mergedUpdates.VisionsSummary = append(mergedUpdates.VisionsSummary, lr.updates.VisionsSummary...)
					mergedUpdates.Interrupts = append(mergedUpdates.Interrupts, lr.updates.Interrupts...)
					mergedUpdates.StrataHealth = append(mergedUpdates.StrataHealth, lr.updates.StrataHealth...)
				}

				merged := MergeLensResults(allLensVisions)
				do.logger.Log(LogEntry{
					Level:        "info",
					Phase:        batch.Phase,
					BatchIndex:   i + 1,
					Message:      "batch complete (lens mode)",
					VisionCount:  len(merged),
					DurationMs:   time.Since(batchStart).Milliseconds(),
					InputTokens:  batchInput,
					OutputTokens: batchOutput,
				})
				results[i] = batchResult{index: i, visions: merged, updates: mergedUpdates, inputTokens: batchInput, outputTokens: batchOutput}

			} else if scRuns == 1 {
				// Single run (no voting, legacy mode)
				prompt := do.prompts.BuildBatchPrompt(batch, do.lucidLog, "")
				ir, err := do.invoker.Invoke(prompt, "sonnet")
				if err != nil {
					do.logger.Log(LogEntry{
						Level:      "error",
						Phase:      batch.Phase,
						BatchIndex: i + 1,
						Message:    "batch invoke failed",
						Error:      err.Error(),
						PromptLen:  len(prompt),
					})
					results[i] = batchResult{index: i, err: err}
					return
				}
				visions, updates, err := do.parser.ParseBatchResponse(ir.Response)
				if err != nil && strings.Contains(err.Error(), "no JSON found") {
					do.logFailedResponse(i+1, ir.Response)
				}
				do.logger.Log(LogEntry{
					Level:        "info",
					Phase:        batch.Phase,
					BatchIndex:   i + 1,
					Message:      "batch complete",
					VisionCount:  len(visions),
					DurationMs:   time.Since(batchStart).Milliseconds(),
					InputTokens:  ir.InputTokens,
					OutputTokens: ir.OutputTokens,
				})
				results[i] = batchResult{index: i, visions: visions, updates: updates, inputTokens: ir.InputTokens, outputTokens: ir.OutputTokens, err: err}
			} else {
				// Self-consistency: invoke N times, vote on results (legacy mode)
				prompt := do.prompts.BuildBatchPrompt(batch, do.lucidLog, "")
				type runResult struct {
					visions      []Vision
					updates      LucidLogUpdates
					inputTokens  int
					outputTokens int
					err          error
				}
				runResults := make([]runResult, scRuns)
				var runWg sync.WaitGroup
				for r := 0; r < scRuns; r++ {
					runWg.Add(1)
					go func(r int) {
						defer runWg.Done()
						ir, err := do.invoker.Invoke(prompt, "sonnet")
						if err != nil {
							runResults[r] = runResult{err: err}
							return
						}
						visions, updates, err := do.parser.ParseBatchResponse(ir.Response)
						if err != nil && strings.Contains(err.Error(), "no JSON found") {
							do.logFailedResponse(i+1, ir.Response)
						}
						runResults[r] = runResult{visions: visions, updates: updates, inputTokens: ir.InputTokens, outputTokens: ir.OutputTokens, err: err}
					}(r)
				}
				runWg.Wait()

				// Collect successful vision lists for voting
				var candidates [][]Vision
				var mergedUpdates LucidLogUpdates
				var batchInputTokens, batchOutputTokens int
				for _, rr := range runResults {
					if rr.err != nil {
						continue
					}
					candidates = append(candidates, rr.visions)
					batchInputTokens += rr.inputTokens
					batchOutputTokens += rr.outputTokens
					mergedUpdates.ObservedPatterns = append(mergedUpdates.ObservedPatterns, rr.updates.ObservedPatterns...)
					mergedUpdates.Tensions = append(mergedUpdates.Tensions, rr.updates.Tensions...)
					mergedUpdates.VisionsSummary = append(mergedUpdates.VisionsSummary, rr.updates.VisionsSummary...)
					mergedUpdates.Interrupts = append(mergedUpdates.Interrupts, rr.updates.Interrupts...)
					mergedUpdates.StrataHealth = append(mergedUpdates.StrataHealth, rr.updates.StrataHealth...)
				}

				if len(candidates) == 0 {
					results[i] = batchResult{index: i, err: fmt.Errorf("all %d self-consistency runs failed", scRuns)}
					return
				}

				voted := VoteVisions(candidates, 0.5)
				do.logger.Log(LogEntry{
					Level:        "info",
					Phase:        batch.Phase,
					BatchIndex:   i + 1,
					Message:      "batch complete",
					VisionCount:  len(voted),
					DurationMs:   time.Since(batchStart).Milliseconds(),
					InputTokens:  batchInputTokens,
					OutputTokens: batchOutputTokens,
				})
				results[i] = batchResult{index: i, visions: voted, updates: mergedUpdates, inputTokens: batchInputTokens, outputTokens: batchOutputTokens}
			}
		}(i, batch)
	}
	wg.Wait()

	// Merge results sequentially; tally per-lens vision counts for quality ledger
	var totalInput, totalOutput, batchVisionCount int
	lensBreakdown := make(map[string]int)
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
		totalInput += r.inputTokens
		totalOutput += r.outputTokens
		batchVisionCount += len(r.visions)
		for _, v := range r.visions {
			if v.LensSource != "" {
				lensBreakdown[v.LensSource]++
			}
		}
		fmt.Printf("  Batch %d: %d visions\n", r.index+1, len(r.visions))
	}

	// Stage 3: Reconciliation
	// Skip for very small batch counts; use Sonnet instead of Opus for moderate counts.
	var finalVisions []Vision
	batchCount := len(batches)
	skipThreshold := do.config.Reconciliation.SkipThreshold
	sonnetThreshold := do.config.Reconciliation.SonnetThreshold

	shouldReconcile := do.config.Reconciliation.Enabled && batchCount > skipThreshold
	if shouldReconcile {
		reconModel := "opus"
		if sonnetThreshold > 0 && batchCount <= sonnetThreshold {
			reconModel = "sonnet"
		}
		fmt.Printf("  Reconciliation (%s, %d batches)...\n", reconModel, batchCount)
		do.logger.Log(LogEntry{Level: "info", Phase: "reconcile", Message: fmt.Sprintf("reconciliation start (model=%s)", reconModel)})
		reconStart := time.Now()
		var reconPrompt string
		if do.config.Batching.LensMode.Enabled {
			reconPrompt = do.prompts.BuildReconciliationPrompt(do.lucidLog, do.config.Batching.LensMode.Lenses...)
		} else {
			reconPrompt = do.prompts.BuildReconciliationPrompt(do.lucidLog)
		}
		reconResult, err := do.invoker.Invoke(reconPrompt, reconModel)
		if err == nil {
			finalVisions, _ = do.parser.ParseReconciliationResponse(reconResult.Response)
			totalInput += reconResult.InputTokens
			totalOutput += reconResult.OutputTokens
		} else {
			do.logger.Log(LogEntry{Level: "error", Phase: "reconcile", Message: "reconciliation failed", Error: err.Error()})
		}
		if finalVisions == nil {
			finalVisions = do.visions.ReadDrafts()
		}
		do.logger.Log(LogEntry{
			Level:       "info",
			Phase:       "reconcile",
			Message:     "reconciliation complete",
			VisionCount: len(finalVisions),
			DurationMs:  time.Since(reconStart).Milliseconds(),
		})
	} else {
		if do.config.Reconciliation.Enabled && batchCount <= skipThreshold {
			fmt.Printf("  Reconciliation skipped (%d batches <= skip threshold %d)\n", batchCount, skipThreshold)
		}
		finalVisions = do.visions.ReadDrafts()
	}

	// Stage 4: Write final + log
	// Append deterministic advisory visions (no LLM cost)
	for _, dm := range candidates.DominantMotes {
		finalVisions = append(finalVisions, Vision{
			Type:        "dominant_mote_review",
			SourceMotes: []string{dm.MoteID},
			Rationale:   fmt.Sprintf("Primed in %d/10 sessions with AccessCount=%d (at cap). Hit rate below 60%%. Review whether this mote still earns its prime slot or is coasting on accumulated retrieval strength.", dm.PrimeFreq, dm.AccessCount),
			Severity:    "low",
		})
	}
	for _, dr := range candidates.DecayRiskMotes {
		finalVisions = append(finalVisions, Vision{
			Type:        "decay_risk",
			SourceMotes: []string{dr.MoteID},
			Rationale:   fmt.Sprintf("weight=%.2f, recency=%.2f, score=%.3f within %.3f of min_relevance_threshold. High-value mote approaching irrelevance. Re-access to refresh recency, or archive if no longer applicable.", dr.Weight, dr.RecencyFactor, dr.Score, dr.ScoreGap),
			Severity:    "medium",
		})
	}
	if err := do.visions.WriteFinal(finalVisions); err != nil {
		return nil, fmt.Errorf("write final visions: %w", err)
	}

	// Save lucid log
	llPath := filepath.Join(dreamDir, "lucid_log.json")
	_ = do.lucidLog.Save(llPath)

	// Estimate cost using batch model pricing (dominant cost; recon is a single call)
	estimatedCost := EstimateCost(do.invoker.batchModel, totalInput, totalOutput)

	result := &DreamResult{
		Status:        "complete",
		Batches:       len(batches),
		Visions:       len(finalVisions),
		InputTokens:   totalInput,
		OutputTokens:  totalOutput,
		EstimatedCost: estimatedCost,
		BatchVisions:  batchVisionCount,
	}
	if len(lensBreakdown) > 0 {
		result.LensBreakdown = lensBreakdown
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
	var totalConfidence float64
	for i := range visions {
		affectedIDs := AffectedMoteIDs(visions[i])
		preScores := SnapshotScores(mm, cfg, affectedIDs)
		visions[i].Confidence = ScoreConfidence(visions[i], stats, preScores)
		totalConfidence += visions[i].Confidence

		if visions[i].Confidence >= threshold {
			toApply = append(toApply, visions[i])
		} else {
			toDefer = append(toDefer, visions[i])
		}
	}
	if len(visions) > 0 {
		do.lastAvgConfidence = totalConfidence / float64(len(visions))
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

// UpdateRunLogAutoApply updates the most recent local run log entry with auto-apply stats.
func (do *DreamOrchestrator) UpdateRunLogAutoApply(applied, deferred int, avgConfidence float64) {
	logPath := filepath.Join(do.root, "dream", "log.jsonl")
	entries := readRunLog(logPath)
	if len(entries) == 0 {
		return
	}
	last := &entries[len(entries)-1]
	last.AutoApplied = applied
	last.Deferred = deferred
	last.AvgConfidence = avgConfidence

	var buf strings.Builder
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	_ = core.AtomicWrite(logPath, []byte(buf.String()), 0644)
}

func readRunLog(path string) []RunLogEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var entries []RunLogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e RunLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries
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
	fmt.Printf("  Merge candidates:         %d\n", len(sr.MergeCandidates))
	fmt.Printf("  Dominant mote candidates: %d\n", len(sr.DominantMotes))
	fmt.Printf("  Decay risk candidates:    %d\n", len(sr.DecayRiskMotes))
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

// validateLensConfig checks that all named lenses are recognized before the dream cycle starts.
func validateLensConfig(lm core.LensModeConfig) error {
	if !lm.Enabled {
		return nil
	}
	for _, name := range lm.Lenses {
		if !KnownLenses[name] {
			return fmt.Errorf("unknown lens %q: valid lenses are %v", name, sortedLensNames())
		}
	}
	return nil
}

func sortedLensNames() []string {
	names := make([]string, 0, len(KnownLenses))
	for k := range KnownLenses {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// scopeFromRoot derives the project name from the .memory root path.
func scopeFromRoot(root string) string {
	return filepath.Base(filepath.Dir(root))
}

func (do *DreamOrchestrator) writeRunLog(result *DreamResult, elapsed time.Duration) {
	now := time.Now().UTC().Format(time.RFC3339)
	votingCfg := VotingConfigLabel(do.config)

	var reconFilterRate float64
	if result.BatchVisions > 0 {
		reconFilterRate = 1.0 - float64(result.Visions)/float64(result.BatchVisions)
	}

	// Compute average agreement from final visions
	var avgAgreement float64
	finalVisions := do.visions.ReadFinal()
	if len(finalVisions) > 0 {
		var totalAgreement float64
		for _, v := range finalVisions {
			totalAgreement += v.Agreement
		}
		avgAgreement = totalAgreement / float64(len(finalVisions))
	}

	logPath := filepath.Join(do.root, "dream", "log.jsonl")
	entry := RunLogEntry{
		Timestamp:       now,
		Status:          result.Status,
		Batches:         result.Batches,
		Visions:         result.Visions,
		DurationS:       elapsed.Seconds(),
		InputTokens:     result.InputTokens,
		OutputTokens:    result.OutputTokens,
		EstimatedCost:   result.EstimatedCost,
		VotingConfig:    votingCfg,
		BatchVisions:    result.BatchVisions,
		ReconVisions:    result.Visions,
		ReconFilterRate: reconFilterRate,
		AvgAgreement:    avgAgreement,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	line = append(line, '\n')

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line)

	// Write to global quality ledger (best-effort)
	project := scopeFromRoot(do.root)
	var costPerVision float64
	if result.Visions > 0 {
		costPerVision = result.EstimatedCost / float64(result.Visions)
	}
	qe := QualityEntry{
		Timestamp:       now,
		Project:         project,
		VotingConfig:    votingCfg,
		Batches:         result.Batches,
		BatchVisions:    result.BatchVisions,
		ReconVisions:    result.Visions,
		ReconFilterRate: reconFilterRate,
		AvgAgreement:    avgAgreement,
		DurationS:       elapsed.Seconds(),
		InputTokens:     result.InputTokens,
		OutputTokens:    result.OutputTokens,
		EstimatedCost:   result.EstimatedCost,
		CostPerVision:   costPerVision,
	}
	if len(result.LensBreakdown) > 0 {
		qe.LensBreakdown = result.LensBreakdown
	}
	_ = AppendQualityEntry(qe) // Non-critical
}
