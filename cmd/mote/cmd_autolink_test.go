package main

import (
	"os"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestAutoLinkOnMoteCreation(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)

	// Create several motes about authentication
	authTopics := []struct {
		title string
		body  string
	}{
		{"OAuth2 token handling", "How we handle OAuth2 bearer tokens in the API gateway"},
		{"JWT validation middleware", "Middleware that validates JWT tokens on each request"},
		{"Session management", "Server-side session storage using encrypted cookies and JWT claims"},
	}
	for _, at := range authTopics {
		_, err := mm.Create("lesson", at.title, core.CreateOpts{Body: at.body})
		if err != nil {
			t.Fatalf("create auth mote: %v", err)
		}
	}

	// Read all motes for auto-linking
	allMotes, err := mm.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}

	// Create the new mote about JWT rotation
	newMote, err := mm.Create("lesson", "JWT token rotation", core.CreateOpts{
		Body: "Rotate JWT tokens every 24h to limit exposure window",
	})
	if err != nil {
		t.Fatalf("create new mote: %v", err)
	}

	// Include the new mote in allMotes
	allMotes = append(allMotes, newMote)

	cfg := core.DefaultConfig()
	err = appendAutoLinks(mm, newMote, allMotes, cfg)
	if err != nil {
		t.Fatalf("appendAutoLinks: %v", err)
	}

	if !strings.Contains(newMote.Body, "See also:") {
		t.Fatalf("expected 'See also:' in body, got: %s", newMote.Body)
	}
	if !strings.Contains(newMote.Body, "[[") {
		t.Fatalf("expected wikilinks in body, got: %s", newMote.Body)
	}

	// Verify persisted to disk
	reread, err := mm.Read(newMote.ID)
	if err != nil {
		t.Fatalf("re-read mote: %v", err)
	}
	if !strings.Contains(reread.Body, "See also:") {
		t.Errorf("persisted body missing 'See also:', got: %s", reread.Body)
	}
}

func TestAutoLinkBelowThresholdExcluded(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)

	// Create motes about cooking
	_, err := mm.Create("lesson", "Sourdough bread recipe", core.CreateOpts{
		Body: "Mix flour water salt starter, bulk ferment 6 hours",
	})
	if err != nil {
		t.Fatalf("create mote: %v", err)
	}

	allMotes, _ := mm.ReadAll()

	// Create a mote about quantum computing - completely unrelated
	newMote, err := mm.Create("explore", "Quantum error correction codes", core.CreateOpts{
		Body: "Topological qubits with surface code stabilizer measurements",
	})
	if err != nil {
		t.Fatalf("create mote: %v", err)
	}

	allMotes = append(allMotes, newMote)

	cfg := core.DefaultConfig()
	cfg.Linking.MinScore = 5.0 // High threshold to ensure no matches
	err = appendAutoLinks(mm, newMote, allMotes, cfg)
	if err != nil {
		t.Fatalf("appendAutoLinks: %v", err)
	}

	if strings.Contains(newMote.Body, "See also:") {
		t.Errorf("expected no wikilinks for unrelated mote, got: %s", newMote.Body)
	}
}

func TestAutoLinkSelfReferenceFiltered(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)

	newMote, err := mm.Create("lesson", "Self reference test", core.CreateOpts{
		Body: "This mote should not link to itself",
	})
	if err != nil {
		t.Fatalf("create mote: %v", err)
	}

	allMotes := []*core.Mote{newMote}

	cfg := core.DefaultConfig()
	cfg.Linking.MinScore = 0.0 // Very low threshold
	err = appendAutoLinks(mm, newMote, allMotes, cfg)
	if err != nil {
		t.Fatalf("appendAutoLinks: %v", err)
	}

	// With only one mote (itself), no links should be added
	if strings.Contains(newMote.Body, "See also:") {
		t.Errorf("expected no self-links, got: %s", newMote.Body)
	}
}

func TestAutoLinkRespectsMaxAutoLinks(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)

	// Create 10 motes about the same topic
	for i := 0; i < 10; i++ {
		_, err := mm.Create("lesson", "Go testing patterns", core.CreateOpts{
			Body: "Table-driven tests with subtests in Go",
		})
		if err != nil {
			t.Fatalf("create mote %d: %v", i, err)
		}
	}

	allMotes, _ := mm.ReadAll()

	newMote, err := mm.Create("lesson", "Go test helpers", core.CreateOpts{
		Body: "Writing test helper functions in Go with t.Helper()",
	})
	if err != nil {
		t.Fatalf("create mote: %v", err)
	}
	allMotes = append(allMotes, newMote)

	cfg := core.DefaultConfig()
	cfg.Linking.MaxAutoLinks = 3
	cfg.Linking.MinScore = 0.0
	err = appendAutoLinks(mm, newMote, allMotes, cfg)
	if err != nil {
		t.Fatalf("appendAutoLinks: %v", err)
	}

	// Count wikilinks
	linkCount := strings.Count(newMote.Body, "[[")
	if linkCount > 3 {
		t.Errorf("expected at most 3 wikilinks, got %d: %s", linkCount, newMote.Body)
	}
}

func TestAutoLinkDisabledWhenMaxZero(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	_ = root

	mm := core.NewMoteManager(root)

	_, err := mm.Create("lesson", "Some topic", core.CreateOpts{Body: "Content here"})
	if err != nil {
		t.Fatalf("create mote: %v", err)
	}

	allMotes, _ := mm.ReadAll()

	newMote, err := mm.Create("lesson", "Same topic", core.CreateOpts{Body: "Same content here"})
	if err != nil {
		t.Fatalf("create mote: %v", err)
	}
	allMotes = append(allMotes, newMote)

	cfg := core.DefaultConfig()
	cfg.Linking.MaxAutoLinks = 0
	err = appendAutoLinks(mm, newMote, allMotes, cfg)
	if err != nil {
		t.Fatalf("appendAutoLinks: %v", err)
	}

	if strings.Contains(newMote.Body, "See also:") {
		t.Errorf("expected no wikilinks when disabled, got: %s", newMote.Body)
	}
}

func TestAutoLinkIntegrationViaCmdAdd(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer func() { os.Stdout = oldStdout }()

	// Create seed motes via cmd
	addType = "lesson"
	addTags = []string{"auth"}
	addWeight = 0.5
	addOrigin = "normal"
	addStatus = ""
	addParent = ""
	addAccept = nil
	addSize = ""
	addLocal = false
	addRefs = nil

	addTitle = "OAuth2 token handling"
	addBody = "How we handle OAuth2 bearer tokens in the API gateway"
	if err := runAdd(nil, nil); err != nil {
		t.Fatalf("runAdd seed 1: %v", err)
	}

	addTitle = "JWT validation middleware"
	addBody = "Middleware that validates JWT tokens on each request"
	if err := runAdd(nil, nil); err != nil {
		t.Fatalf("runAdd seed 2: %v", err)
	}

	addTitle = "Session management with JWT"
	addBody = "Server-side session storage using encrypted cookies and JWT claims"
	if err := runAdd(nil, nil); err != nil {
		t.Fatalf("runAdd seed 3: %v", err)
	}

	// Now add a related mote - should get auto-linked
	addTitle = "JWT token rotation strategy"
	addBody = "Rotate JWT tokens every 24h to limit exposure window"
	if err := runAdd(nil, nil); err != nil {
		t.Fatalf("runAdd new: %v", err)
	}

	// Find the last created mote and check its body
	root, _ := findMemoryRoot()
	mm := core.NewMoteManager(root)
	allMotes, _ := mm.ReadAll()

	var found *core.Mote
	for _, m := range allMotes {
		if m.Title == "JWT token rotation strategy" {
			found = m
			break
		}
	}
	if found == nil {
		t.Fatal("could not find newly created mote")
	}

	if !strings.Contains(found.Body, "See also:") {
		t.Errorf("expected auto-linked 'See also:' in body after runAdd, got: %s", found.Body)
	}
}
