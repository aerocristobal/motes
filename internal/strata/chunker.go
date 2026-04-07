// SPDX-License-Identifier: AGPL-3.0-or-later
package strata

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Chunker splits reference material into searchable chunks.
type Chunker struct {
	strategy  string // heading-aware | function-level | sliding-window
	maxTokens int
	overlap   int
}

// NewChunker creates a Chunker from config values.
func NewChunker(strategy string, maxTokens, overlap int) *Chunker {
	return &Chunker{
		strategy:  strategy,
		maxTokens: maxTokens,
		overlap:   overlap,
	}
}

// ChunkFile chunks content, choosing strategy by file extension if strategy is "auto"
// or using the configured strategy.
func (c *Chunker) ChunkFile(content, sourcePath, corpus string) []Chunk {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	strategy := c.strategyForFile(sourcePath)
	switch strategy {
	case "heading-aware":
		return c.chunkHeadingAware(content, sourcePath, corpus)
	case "function-level":
		return c.chunkFunctionLevel(content, sourcePath, corpus)
	default:
		return c.chunkSlidingWindow(content, sourcePath, corpus)
	}
}

func (c *Chunker) strategyForFile(path string) string {
	if c.strategy != "heading-aware" && c.strategy != "" {
		return c.strategy
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".txt", "":
		return "heading-aware"
	case ".go", ".py", ".js", ".ts", ".rs", ".sh", ".rb", ".java", ".c", ".cpp":
		return "function-level"
	default:
		return "heading-aware"
	}
}

func (c *Chunker) chunkHeadingAware(content, sourcePath, corpus string) []Chunk {
	lines := strings.Split(content, "\n")
	var chunks []Chunk
	var currentText strings.Builder
	currentHeading := ""
	idx := 0
	base := filepath.Base(sourcePath)

	flush := func() {
		text := strings.TrimSpace(currentText.String())
		if text == "" {
			return
		}
		if estimateTokens(text) > c.maxTokens {
			// Split oversized sections with sliding window
			subChunks := c.chunkSlidingWindow(text, sourcePath, corpus)
			for _, sc := range subChunks {
				sc.ID = fmt.Sprintf("%s-%s-%03d", corpus, base, idx)
				sc.Heading = currentHeading
				chunks = append(chunks, sc)
				idx++
			}
		} else {
			chunks = append(chunks, Chunk{
				ID:         fmt.Sprintf("%s-%s-%03d", corpus, base, idx),
				Text:       text,
				SourcePath: sourcePath,
				Heading:    currentHeading,
			})
			idx++
		}
		currentText.Reset()
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			flush()
			currentHeading = strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
		currentText.WriteString(line)
		currentText.WriteString("\n")
	}
	flush()
	return chunks
}

var funcPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^func\s`),                         // Go
	regexp.MustCompile(`(?m)^def\s`),                           // Python
	regexp.MustCompile(`(?m)^(export\s+)?(async\s+)?function\s`), // JS/TS
	regexp.MustCompile(`(?m)^class\s`),                         // Python/JS/TS/Java
}

func (c *Chunker) chunkFunctionLevel(content, sourcePath, corpus string) []Chunk {
	lines := strings.Split(content, "\n")
	var boundaries []int
	base := filepath.Base(sourcePath)

	for i, line := range lines {
		for _, pat := range funcPatterns {
			if pat.MatchString(line) {
				boundaries = append(boundaries, i)
				break
			}
		}
	}

	// No function boundaries → fall back to sliding window
	if len(boundaries) == 0 {
		return c.chunkSlidingWindow(content, sourcePath, corpus)
	}

	var chunks []Chunk
	// Pre-function content
	if boundaries[0] > 0 {
		text := strings.TrimSpace(strings.Join(lines[:boundaries[0]], "\n"))
		if text != "" {
			chunks = append(chunks, Chunk{
				ID:         fmt.Sprintf("%s-%s-%03d", corpus, base, len(chunks)),
				Text:       text,
				SourcePath: sourcePath,
			})
		}
	}

	for i, start := range boundaries {
		end := len(lines)
		if i+1 < len(boundaries) {
			end = boundaries[i+1]
		}
		text := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		if text == "" {
			continue
		}
		heading := strings.TrimSpace(lines[start])
		chunks = append(chunks, Chunk{
			ID:         fmt.Sprintf("%s-%s-%03d", corpus, base, len(chunks)),
			Text:       text,
			SourcePath: sourcePath,
			Heading:    heading,
		})
	}

	return chunks
}

func (c *Chunker) chunkSlidingWindow(content, sourcePath, corpus string) []Chunk {
	words := strings.Fields(content)
	if len(words) == 0 {
		return nil
	}

	base := filepath.Base(sourcePath)
	maxWords := int(float64(c.maxTokens) / 1.3)
	overlapWords := int(float64(c.overlap) / 1.3)
	if maxWords < 1 {
		maxWords = 100
	}

	var chunks []Chunk
	idx := 0
	for start := 0; start < len(words); {
		end := start + maxWords
		if end > len(words) {
			end = len(words)
		}
		text := strings.Join(words[start:end], " ")
		chunks = append(chunks, Chunk{
			ID:         fmt.Sprintf("%s-%s-%03d", corpus, base, idx),
			Text:       text,
			SourcePath: sourcePath,
		})
		idx++
		start += maxWords - overlapWords
		if start >= end {
			break
		}
	}
	return chunks
}
