// SPDX-License-Identifier: MIT
//
// STORY-JSCHEMA-001 — envelope coverage for `mote context <topic> --json`.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/jsonenv"
)

func TestContext_EnvelopeMode_WrapsListUnderData(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("decision", "context envelope target rare-keyword-xyz", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Rebuild the BM25 index so the freshly-seeded mote is searchable.
	all, err := mm.ReadAllWithGlobal()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if err := rebuildMoteBM25(root, all); err != nil {
		t.Fatalf("rebuild bm25: %v", err)
	}
	// Build the edge index too so context's traversal has fresh data.
	im := core.NewIndexManager(root)
	if err := im.Rebuild(all); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	t.Setenv(jsonenv.EnvVar, "1")
	resetJsonenvForTest(t)

	contextJSON = true
	defer func() { contextJSON = false }()

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"context", "rare-keyword-xyz", "--json"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})
	if rerr != nil {
		t.Fatalf("unexpected error: %v (stderr=%q)", rerr, stderr)
	}
	if stderr != "" {
		t.Errorf("envelope mode must NOT emit notice on stderr, got %q", stderr)
	}

	var got struct {
		SchemaVersion int           `json:"schema_version"`
		Data          ContextOutput `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("envelope JSON must parse, got %v over %q", err, stdout)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if got.Data.Topic != "rare-keyword-xyz" {
		t.Errorf("data.topic = %q, want rare-keyword-xyz", got.Data.Topic)
	}
}

func TestContext_LegacyMode_PreservesShapeAndEmitsNotice(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("decision", "context legacy target other-rare-yyy", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	all, _ := mm.ReadAllWithGlobal()
	_ = rebuildMoteBM25(root, all)
	im := core.NewIndexManager(root)
	_ = im.Rebuild(all)

	t.Setenv(jsonenv.EnvVar, "")
	resetJsonenvForTest(t)

	contextJSON = true
	defer func() { contextJSON = false }()

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"context", "other-rare-yyy", "--json"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rerr = rootCmd.Execute()
	})
	if rerr != nil {
		t.Fatalf("unexpected error: %v", rerr)
	}
	if strings.Contains(stdout, "schema_version") {
		t.Errorf("legacy stdout must NOT carry schema_version, got %q", stdout)
	}
	if !strings.Contains(stderr, jsonenv.EnvVar) {
		t.Errorf("legacy stderr must name %s, got %q", jsonenv.EnvVar, stderr)
	}
}
