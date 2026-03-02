package strata

import (
	"math"
	"sort"
)

// BM25Index holds precomputed term frequencies and IDF values for BM25 search.
type BM25Index struct {
	DocCount  int                `json:"doc_count"`
	AvgDocLen float64            `json:"avg_doc_len"`
	IDF       map[string]float64 `json:"idf"`
	Docs      []BM25Doc          `json:"docs"`
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

	return idx
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
