package dream

import (
	"sort"

	"motes/internal/core"
)

// BatchConstructor implements hybrid batching: tag-clustered then interleaved.
type BatchConstructor struct {
	config     core.BatchingConfig
	moteReader func(string) (*core.Mote, error)
}

// NewBatchConstructor creates a batch constructor.
func NewBatchConstructor(cfg core.BatchingConfig, reader func(string) (*core.Mote, error)) *BatchConstructor {
	return &BatchConstructor{config: cfg, moteReader: reader}
}

// Build creates batches from scan results using hybrid batching.
func (bc *BatchConstructor) Build(candidates *ScanResult) []Batch {
	maxPerBatch := bc.config.MaxMotesPerBatch
	if maxPerBatch <= 0 {
		maxPerBatch = 10
	}
	clusteredFrac := bc.config.ClusteredFraction
	if clusteredFrac <= 0 {
		clusteredFrac = 0.6
	}

	// Collect all unique mote IDs and their associated tasks
	moteTaskMap := map[string]map[string]bool{}
	addMotes := func(ids []string, task string) {
		for _, id := range ids {
			if moteTaskMap[id] == nil {
				moteTaskMap[id] = map[string]bool{}
			}
			moteTaskMap[id][task] = true
		}
	}
	addPairs := func(pairs []MotePair, task string) {
		for _, p := range pairs {
			if moteTaskMap[p.A] == nil {
				moteTaskMap[p.A] = map[string]bool{}
			}
			moteTaskMap[p.A][task] = true
			if moteTaskMap[p.B] == nil {
				moteTaskMap[p.B] = map[string]bool{}
			}
			moteTaskMap[p.B][task] = true
		}
	}

	addPairs(candidates.LinkCandidates, "link_inference")
	addPairs(candidates.ContradictionCandidates, "contradiction_detection")
	addMotes(candidates.StaleMotes, "staleness_evaluation")
	addMotes(candidates.CompressionCandidates, "compression")
	addMotes(candidates.UncrystallizedIssues, "crystallization")
	for _, ce := range candidates.ConstellationEvolution {
		if moteTaskMap[ce.ConstellationID] == nil {
			moteTaskMap[ce.ConstellationID] = map[string]bool{}
		}
		moteTaskMap[ce.ConstellationID]["constellation_evolution"] = true
	}
	for _, sc := range candidates.SignalCandidates {
		if moteTaskMap[sc.MoteID] == nil {
			moteTaskMap[sc.MoteID] = map[string]bool{}
		}
		moteTaskMap[sc.MoteID]["signal_discovery"] = true
	}
	for _, ot := range candidates.OverloadedTags {
		// Tag overload doesn't map directly to motes, but we include for batch prompt context
		_ = ot
	}

	if len(moteTaskMap) == 0 {
		return nil
	}

	// Build mote-to-tag map for clustering
	moteIDs := make([]string, 0, len(moteTaskMap))
	for id := range moteTaskMap {
		moteIDs = append(moteIDs, id)
	}

	// Read motes for tag info
	moteTags := map[string][]string{}
	for _, id := range moteIDs {
		m, err := bc.moteReader(id)
		if err != nil {
			continue
		}
		moteTags[id] = m.Tags
	}

	// Phase A: Tag-clustered batches
	clusteredCount := int(float64(len(moteIDs)) * clusteredFrac)
	if clusteredCount > len(moteIDs) {
		clusteredCount = len(moteIDs)
	}

	// Cluster by primary (most common) tag
	tagGroups := map[string][]string{}
	assigned := map[string]bool{}
	for _, id := range moteIDs {
		tags := moteTags[id]
		if len(tags) > 0 {
			tagGroups[tags[0]] = append(tagGroups[tags[0]], id)
		}
	}

	// Sort tag groups by size descending
	type tagGroup struct {
		tag string
		ids []string
	}
	var sortedGroups []tagGroup
	for tag, ids := range tagGroups {
		sortedGroups = append(sortedGroups, tagGroup{tag, ids})
	}
	sort.Slice(sortedGroups, func(i, j int) bool {
		return len(sortedGroups[i].ids) > len(sortedGroups[j].ids)
	})

	var batches []Batch
	clusteredAssigned := 0
	for _, g := range sortedGroups {
		if clusteredAssigned >= clusteredCount {
			break
		}
		// Build batches from this tag group
		var batchIDs []string
		for _, id := range g.ids {
			if assigned[id] || clusteredAssigned >= clusteredCount {
				continue
			}
			batchIDs = append(batchIDs, id)
			assigned[id] = true
			clusteredAssigned++
			if len(batchIDs) >= maxPerBatch {
				batches = append(batches, Batch{
					Phase:          "clustered",
					PrimaryCluster: g.tag,
					MoteIDs:        batchIDs,
					Tasks:          collectTasks(batchIDs, moteTaskMap),
				})
				batchIDs = nil
			}
		}
		if len(batchIDs) > 0 {
			batches = append(batches, Batch{
				Phase:          "clustered",
				PrimaryCluster: g.tag,
				MoteIDs:        batchIDs,
				Tasks:          collectTasks(batchIDs, moteTaskMap),
			})
		}
	}

	// Phase B: Interleaved batches from remaining motes
	var remaining []string
	for _, id := range moteIDs {
		if !assigned[id] {
			remaining = append(remaining, id)
		}
	}
	for i := 0; i < len(remaining); i += maxPerBatch {
		end := i + maxPerBatch
		if end > len(remaining) {
			end = len(remaining)
		}
		chunk := remaining[i:end]
		batches = append(batches, Batch{
			Phase:   "interleaved",
			MoteIDs: chunk,
			Tasks:   collectTasks(chunk, moteTaskMap),
		})
	}

	return batches
}

func collectTasks(ids []string, moteTaskMap map[string]map[string]bool) []string {
	taskSet := map[string]bool{}
	for _, id := range ids {
		for task := range moteTaskMap[id] {
			taskSet[task] = true
		}
	}
	tasks := make([]string, 0, len(taskSet))
	for t := range taskSet {
		tasks = append(tasks, t)
	}
	sort.Strings(tasks)
	return tasks
}
