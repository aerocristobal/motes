package strata

import (
	"math"
	"sort"
)

// AdaptiveThresholds holds corpus-size-adapted score thresholds for BM25 filtering.
type AdaptiveThresholds struct {
	PromptContext     float64 `json:"prompt_context"`
	ContentSimilarity float64 `json:"content_similarity"`
	Calibrated        bool    `json:"calibrated"`
	CalibrationN      int     `json:"calibration_n"`
}

// BM25Index holds precomputed term frequencies and IDF values for BM25 search.
type BM25Index struct {
	DocCount   int                  `json:"doc_count"`
	AvgDocLen  float64              `json:"avg_doc_len"`
	IDF        map[string]float64   `json:"idf"`
	Docs       []BM25Doc            `json:"docs"`
	Thresholds *AdaptiveThresholds  `json:"thresholds,omitempty"`
}

// BM25Doc holds per-document term frequencies and length.
type BM25Doc struct {
	ChunkID  string             `json:"chunk_id"`
	TermFreq map[string]float64 `json:"tf"`
	DocLen   int                `json:"doc_len"`
}

const (
	bm25K1 = 1.2  // term frequency saturation
	bm25B  = 0.75 // document length normalization
)

// Chunk is a unit of searchable content.
type Chunk struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	SourcePath string `json:"source_path"`
	Heading    string `json:"heading,omitempty"`
}

// ChunkResult is a search result with score.
type ChunkResult struct {
	Chunk Chunk
	Score float64
}

// BuildBM25Index creates a BM25 index from chunks.
func BuildBM25Index(chunks []Chunk) *BM25Index {
	if len(chunks) == 0 {
		return &BM25Index{IDF: map[string]float64{}}
	}

	idx := &BM25Index{DocCount: len(chunks)}

	totalLen := 0
	docFreq := make(map[string]int)

	for _, chunk := range chunks {
		tokens := tokenize(chunk.Text)
		totalLen += len(tokens)

		tf := make(map[string]float64)
		seen := make(map[string]bool)
		for _, token := range tokens {
			tf[token]++
			if !seen[token] {
				docFreq[token]++
				seen[token] = true
			}
		}
		for term := range tf {
			tf[term] /= float64(len(tokens))
		}

		idx.Docs = append(idx.Docs, BM25Doc{
			ChunkID:  chunk.ID,
			TermFreq: tf,
			DocLen:   len(tokens),
		})
	}
	idx.AvgDocLen = float64(totalLen) / float64(len(chunks))

	idx.IDF = make(map[string]float64)
	for term, df := range docFreq {
		idx.IDF[term] = math.Log((float64(idx.DocCount)-float64(df)+0.5)/(float64(df)+0.5) + 1)
	}

	idx.Thresholds = computeAdaptiveThresholds(idx.DocCount)

	return idx
}

const (
	basePromptContext     = 2.0
	baseContentSimilarity = 1.0
	referenceCorpusSize   = 50
	floorPromptContext    = 0.3
	floorContentSimilarity = 0.15
	ceilingMultiplier     = 3.0
)

// maxIDF returns the maximum possible IDF for a corpus of size n
// (the IDF of a term appearing in exactly 1 document).
func maxIDF(n int) float64 {
	if n <= 0 {
		return 0
	}
	return math.Log((float64(n)-0.5)/1.5 + 1)
}

// computeAdaptiveThresholds scales BM25 thresholds proportionally to corpus size.
func computeAdaptiveThresholds(docCount int) *AdaptiveThresholds {
	if docCount <= 0 {
		return &AdaptiveThresholds{
			PromptContext:     floorPromptContext,
			ContentSimilarity: floorContentSimilarity,
		}
	}
	refIDF := maxIDF(referenceCorpusSize)
	curIDF := maxIDF(docCount)
	ratio := curIDF / refIDF

	pc := clampThreshold(basePromptContext*ratio, floorPromptContext, basePromptContext*ceilingMultiplier)
	cs := clampThreshold(baseContentSimilarity*ratio, floorContentSimilarity, baseContentSimilarity*ceilingMultiplier)

	return &AdaptiveThresholds{
		PromptContext:     pc,
		ContentSimilarity: cs,
	}
}

func clampThreshold(val, floor, ceiling float64) float64 {
	if val < floor {
		return floor
	}
	if val > ceiling {
		return ceiling
	}
	return val
}

// ThresholdFor returns the adaptive threshold for a given usage.
// Falls back to hardcoded defaults when Thresholds is nil.
func (idx *BM25Index) ThresholdFor(usage string) float64 {
	if idx.Thresholds == nil {
		switch usage {
		case "prompt_context":
			return basePromptContext
		case "content_similarity":
			return baseContentSimilarity
		default:
			return 1.0
		}
	}
	switch usage {
	case "prompt_context":
		return idx.Thresholds.PromptContext
	case "content_similarity":
		return idx.Thresholds.ContentSimilarity
	default:
		return 1.0
	}
}

// SetCalibration computes percentile-based thresholds from observed scores.
// Uses p75 for prompt_context and p50 for content_similarity.
func (idx *BM25Index) SetCalibration(scores []float64) {
	if len(scores) == 0 {
		return
	}
	sort.Float64s(scores)

	p50 := percentile(scores, 0.50)
	p75 := percentile(scores, 0.75)

	if idx.Thresholds == nil {
		idx.Thresholds = computeAdaptiveThresholds(idx.DocCount)
	}

	// Only override if calibration produces meaningful values
	if p75 > 0 {
		idx.Thresholds.PromptContext = clampThreshold(p75, floorPromptContext, basePromptContext*ceilingMultiplier)
	}
	if p50 > 0 {
		idx.Thresholds.ContentSimilarity = clampThreshold(p50, floorContentSimilarity, baseContentSimilarity*ceilingMultiplier)
	}
	idx.Thresholds.Calibrated = true
	idx.Thresholds.CalibrationN = len(scores)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// Search finds the topK most relevant chunks for a query.
func (idx *BM25Index) Search(query string, topK int) []ChunkResult {
	if idx.DocCount == 0 {
		return nil
	}

	queryTerms := tokenize(query)
	scores := make([]float64, len(idx.Docs))

	for _, term := range queryTerms {
		idf, exists := idx.IDF[term]
		if !exists {
			continue
		}
		for i, doc := range idx.Docs {
			tf := doc.TermFreq[term]
			if tf == 0 {
				continue
			}
			numerator := tf * (bm25K1 + 1)
			denominator := tf + bm25K1*(1-bm25B+bm25B*float64(doc.DocLen)/idx.AvgDocLen)
			scores[i] += idf * (numerator / denominator)
		}
	}

	return topKResults(scores, idx.Docs, topK)
}

// SimilarResult represents a document similar to a query document.
type SimilarResult struct {
	DocID string
	Score float64
}

// FindSimilar finds documents most similar to the given docID using its top distinctive terms.
// It extracts the top maxTerms terms by TF×IDF from the source document and queries the index.
// Returns up to topK results above minScore, excluding the source document.
func (idx *BM25Index) FindSimilar(docID string, topK int, minScore float64, maxTerms int) []SimilarResult {
	if idx.DocCount == 0 {
		return nil
	}

	// Find the source document
	var srcDoc *BM25Doc
	var srcIdx int
	for i := range idx.Docs {
		if idx.Docs[i].ChunkID == docID {
			srcDoc = &idx.Docs[i]
			srcIdx = i
			break
		}
	}
	if srcDoc == nil {
		return nil
	}

	// Extract top terms by TF × IDF
	queryTerms := idx.distinctiveTerms(srcDoc, maxTerms)
	if len(queryTerms) == 0 {
		return nil
	}

	// Score all other docs using these terms as a BM25 query
	scores := make([]float64, len(idx.Docs))
	for _, term := range queryTerms {
		idf, exists := idx.IDF[term]
		if !exists {
			continue
		}
		for i, doc := range idx.Docs {
			if i == srcIdx {
				continue
			}
			tf := doc.TermFreq[term]
			if tf == 0 {
				continue
			}
			numerator := tf * (bm25K1 + 1)
			denominator := tf + bm25K1*(1-bm25B+bm25B*float64(doc.DocLen)/idx.AvgDocLen)
			scores[i] += idf * (numerator / denominator)
		}
	}

	// Collect results above minScore
	type scored struct {
		idx   int
		score float64
	}
	var items []scored
	for i, s := range scores {
		if i != srcIdx && s >= minScore {
			items = append(items, scored{idx: i, score: s})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})
	if len(items) > topK {
		items = items[:topK]
	}

	results := make([]SimilarResult, len(items))
	for i, item := range items {
		results[i] = SimilarResult{
			DocID: idx.Docs[item.idx].ChunkID,
			Score: item.score,
		}
	}
	return results
}

// DistinctiveTerms returns the top N distinctive terms from a document identified by ChunkID.
// Returns nil if the document is not found.
func (idx *BM25Index) DistinctiveTerms(docID string, n int) []string {
	for i := range idx.Docs {
		if idx.Docs[i].ChunkID == docID {
			return idx.distinctiveTerms(&idx.Docs[i], n)
		}
	}
	return nil
}

// distinctiveTerms returns the top N terms from a document ranked by TF × IDF.
func (idx *BM25Index) distinctiveTerms(doc *BM25Doc, n int) []string {
	type termScore struct {
		term  string
		score float64
	}
	var items []termScore
	for term, tf := range doc.TermFreq {
		idf, ok := idx.IDF[term]
		if !ok {
			continue
		}
		items = append(items, termScore{term: term, score: tf * idf})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})
	if len(items) > n {
		items = items[:n]
	}
	terms := make([]string, len(items))
	for i, item := range items {
		terms[i] = item.term
	}
	return terms
}

func topKResults(scores []float64, docs []BM25Doc, topK int) []ChunkResult {
	type scored struct {
		idx   int
		score float64
	}

	var items []scored
	for i, s := range scores {
		if s > 0 {
			items = append(items, scored{idx: i, score: s})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	if len(items) > topK {
		items = items[:topK]
	}

	result := make([]ChunkResult, len(items))
	for i, item := range items {
		result[i] = ChunkResult{
			Chunk: Chunk{ID: docs[item.idx].ChunkID},
			Score: item.score,
		}
	}
	return result
}
