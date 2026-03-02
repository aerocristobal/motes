package strata

import (
	"strings"
	"testing"
)

func TestChunk_HeadingAware(t *testing.T) {
	content := `# Introduction

This is the introduction.

# Authentication

OAuth is used for authentication.

# Token Refresh

Tokens must be refreshed periodically.
`
	c := NewChunker("heading-aware", 512, 50)
	chunks := c.ChunkFile(content, "docs/auth.md", "test")

	if len(chunks) != 3 {
		t.Errorf("expected 3 chunks (one per heading), got %d", len(chunks))
		for i, ch := range chunks {
			t.Logf("  chunk %d: heading=%q text=%q", i, ch.Heading, ch.Text[:min(40, len(ch.Text))])
		}
	}

	if len(chunks) > 0 && chunks[0].Heading != "Introduction" {
		t.Errorf("first chunk heading: got %q, want 'Introduction'", chunks[0].Heading)
	}
}

func TestChunk_HeadingAware_LargeSection(t *testing.T) {
	// Create a section that exceeds maxTokens
	var sb strings.Builder
	sb.WriteString("# Big Section\n\n")
	for i := 0; i < 200; i++ {
		sb.WriteString("word ")
	}

	c := NewChunker("heading-aware", 50, 10) // Very small maxTokens
	chunks := c.ChunkFile(sb.String(), "big.md", "test")

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for oversized section, got %d", len(chunks))
	}
}

func TestChunk_FunctionLevel_Go(t *testing.T) {
	content := `package main

import "fmt"

func hello() {
	fmt.Println("hello")
}

func world() {
	fmt.Println("world")
}
`
	c := NewChunker("function-level", 512, 50)
	chunks := c.ChunkFile(content, "main.go", "test")

	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks (one per function), got %d", len(chunks))
	}
}

func TestChunk_SlidingWindow(t *testing.T) {
	words := make([]string, 200)
	for i := range words {
		words[i] = "word"
	}
	content := strings.Join(words, " ")

	c := NewChunker("sliding-window", 100, 20)
	chunks := c.ChunkFile(content, "data.txt", "test")

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks from sliding window, got %d", len(chunks))
	}
}

func TestChunk_EmptyContent(t *testing.T) {
	c := NewChunker("heading-aware", 512, 50)
	chunks := c.ChunkFile("", "empty.md", "test")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty content, got %d", len(chunks))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
