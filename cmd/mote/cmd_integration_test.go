package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

// setupIntegrationTest creates .memory/ in a temp dir and chdir's into it.
// Tests using this MUST NOT call t.Parallel().
func setupIntegrationTest(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, ".memory")
	os.MkdirAll(filepath.Join(memDir, "nodes"), 0755)

	// Initialize config
	cfg := core.DefaultConfig()
	core.SaveConfig(memDir, cfg)

	// Initialize index
	im := core.NewIndexManager(memDir)
	im.Rebuild(nil)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	return memDir, func() {
		os.Chdir(origDir)
	}
}

// captureStdout captures output written to os.Stdout during fn.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	return string(data)
}

type moteSpec struct {
	Type   string
	Title  string
	Status string
	Body   string
	Tags   []string
	Weight float64
}

// seedMotes creates motes from specs and rebuilds the index.
func seedMotes(t *testing.T, root string, specs []moteSpec) {
	t.Helper()
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	for _, s := range specs {
		opts := core.CreateOpts{Tags: s.Tags, Body: s.Body}
		if s.Weight > 0 {
			opts.Weight = s.Weight
		}
		m, err := mm.Create(s.Type, s.Title, opts)
		if err != nil {
			t.Fatalf("seed mote %q: %v", s.Title, err)
		}
		if s.Status != "" && s.Status != "active" {
			mm.Update(m.ID, map[string]interface{}{"status": s.Status})
		}
	}

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
}

// --- Prime ---

func TestPrime_Smoke(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Active task one", Tags: []string{"testing"}, Weight: 0.5},
	})

	output := captureStdout(func() {
		primeCmd.RunE(primeCmd, nil)
	})
	if !strings.Contains(output, "Active work") {
		t.Errorf("expected 'Active work' in prime output, got:\n%s", output)
	}
}

func TestPrime_JSON(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "JSON task", Tags: []string{"test"}, Weight: 0.5},
	})

	// Set --json flag
	primeJSON = true
	defer func() { primeJSON = false }()

	output := captureStdout(func() {
		primeCmd.RunE(primeCmd, nil)
	})

	// JSON output may include text before the JSON block; find the first '{'
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON found in output:\n%s", output)
	}
	var parsed PrimeOutput
	if err := json.Unmarshal([]byte(output[idx:]), &parsed); err != nil {
		t.Errorf("expected valid JSON output, got parse error: %v\nOutput:\n%s", err, output)
	}
}

// --- Context ---

func TestContext_Smoke(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "Auth decision", Tags: []string{"auth", "security"}, Body: "We chose OAuth."},
		{Type: "lesson", Title: "Security lesson", Tags: []string{"auth", "security"}, Body: "Always validate tokens."},
	})

	output := captureStdout(func() {
		contextCmd.RunE(contextCmd, []string{"auth"})
	})
	if !strings.Contains(output, "auth") && !strings.Contains(output, "Auth") {
		t.Errorf("expected auth-related content in context output, got:\n%s", output)
	}
}

func TestContext_Planning(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	mA, _ := mm.Create("task", "Parent task", core.CreateOpts{Tags: []string{"planning"}, Weight: 0.5})
	mB, _ := mm.Create("task", "Child task", core.CreateOpts{Tags: []string{"planning"}, Weight: 0.5})
	mm.Link(mB.ID, "depends_on", mA.ID, im)

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	contextPlanning = true
	defer func() { contextPlanning = false }()

	output := captureStdout(func() {
		contextCmd.RunE(contextCmd, []string{mA.ID})
	})
	// Should show some output (chain or "no dependents")
	if output == "" {
		t.Error("expected non-empty planning context output")
	}
}

// --- Strata ---

func TestStrata_LsEmpty(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	output := captureStdout(func() {
		runStrataLs(strataLsCmd, nil)
	})
	if !strings.Contains(output, "No strata corpora") {
		t.Errorf("expected empty state message, got:\n%s", output)
	}
}

func TestStrata_AddQueryRm(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create a test file to ingest
	testFile := filepath.Join(root, "..", "test_doc.md")
	os.WriteFile(testFile, []byte("# Test Document\n\nThis is about OAuth authentication patterns."), 0644)

	// Add
	strataCorpus = "test-corpus"
	defer func() { strataCorpus = "" }()

	err := runStrataAdd(strataAddCmd, []string{testFile})
	if err != nil {
		t.Fatalf("strata add: %v", err)
	}

	// Ls — should show corpus
	output := captureStdout(func() {
		runStrataLs(strataLsCmd, nil)
	})
	if !strings.Contains(output, "test-corpus") {
		t.Errorf("expected test-corpus in strata ls, got:\n%s", output)
	}

	// Query
	output = captureStdout(func() {
		runStrataQuery(strataQueryCmd, []string{"OAuth"})
	})
	// Should produce some output (even if no matches, it won't error)
	_ = output

	// Rm
	err = runStrataRm(strataRmCmd, []string{"test-corpus"})
	if err != nil {
		t.Fatalf("strata rm: %v", err)
	}

	// Verify removed
	output = captureStdout(func() {
		runStrataLs(strataLsCmd, nil)
	})
	if strings.Contains(output, "test-corpus") {
		t.Error("expected test-corpus to be removed after strata rm")
	}
}

// --- Stats ---

func TestStats_Smoke(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "Stats test", Tags: []string{"test"}},
	})

	output := captureStdout(func() {
		runStats(statsCmd, nil)
	})
	if !strings.Contains(output, "Nebula Stats") {
		t.Errorf("expected 'Nebula Stats' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Total motes") {
		t.Errorf("expected 'Total motes' in output, got:\n%s", output)
	}
}

// --- Session End ---

func TestSessionEnd_Empty(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := runSessionEnd(sessionEndCmd, nil)
	if err != nil {
		t.Fatalf("session-end with empty state: %v", err)
	}
}

// --- Constellation ---

func TestConstellation_ListEmpty(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	output := captureStdout(func() {
		runConstellationList(constellationListCmd, nil)
	})
	// With no motes, should either show empty table or "No" message
	_ = output // no error is sufficient
}

func TestConstellation_ListWithTags(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "Auth A", Tags: []string{"auth"}},
		{Type: "lesson", Title: "Auth B", Tags: []string{"auth"}},
	})

	output := captureStdout(func() {
		runConstellationList(constellationListCmd, nil)
	})
	if !strings.Contains(output, "auth") {
		t.Errorf("expected 'auth' tag in constellation list, got:\n%s", output)
	}
}

// --- Crystallize ---

func TestCrystallize_NoArgs(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Reset flag state
	crystallizeCandidates = false

	err := runCrystallize(crystallizeCmd, nil)
	if err == nil {
		t.Error("expected error for no args")
	}
	if err != nil && !strings.Contains(err.Error(), "mote ID required") {
		t.Errorf("expected 'mote ID required' error, got: %v", err)
	}
}

func TestCrystallize_Candidates(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Done task", Status: "completed", Tags: []string{"test"}},
	})

	crystallizeCandidates = true
	defer func() { crystallizeCandidates = false }()

	output := captureStdout(func() {
		err := runCrystallize(crystallizeCmd, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "crystallize error: %v\n", err)
		}
	})
	if !strings.Contains(output, "Done task") {
		t.Errorf("expected completed task in candidates output, got:\n%s", output)
	}
}

// --- Onboard ---

func TestOnboard_DryRun(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	onboardDryRun = true
	onboardGlobal = false
	defer func() { onboardDryRun = false }()

	err := runOnboard(onboardCmd, nil)
	if err != nil {
		t.Fatalf("onboard --dry-run: %v", err)
	}
}
