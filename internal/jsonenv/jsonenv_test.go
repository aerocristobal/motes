// SPDX-License-Identifier: MIT
package jsonenv

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// --- Mode() ----------------------------------------------------------------

func TestMode_envUnset_returnsLegacy(t *testing.T) {
	ResetForTest()
	t.Setenv(EnvVar, "")
	if got := Mode(); got != ModeLegacy {
		t.Fatalf("Mode() = %v, want ModeLegacy", got)
	}
}

func TestMode_envOne_returnsEnvelope(t *testing.T) {
	ResetForTest()
	t.Setenv(EnvVar, "1")
	if got := Mode(); got != ModeEnvelope {
		t.Fatalf("Mode() = %v, want ModeEnvelope", got)
	}
}

func TestMode_envZero_returnsLegacy(t *testing.T) {
	ResetForTest()
	t.Setenv(EnvVar, "0")
	if got := Mode(); got != ModeLegacy {
		t.Fatalf("Mode() = %v, want ModeLegacy", got)
	}
}

func TestMode_isCached(t *testing.T) {
	// Once Mode() resolves a value it must not flip mid-process, because
	// downstream commands may have already serialized partial output.
	ResetForTest()
	t.Setenv(EnvVar, "1")
	if Mode() != ModeEnvelope {
		t.Fatalf("first Mode() call did not pick up MOTE_JSON_ENVELOPE=1")
	}
	t.Setenv(EnvVar, "0")
	if Mode() != ModeEnvelope {
		t.Fatalf("Mode() flipped after first resolution; once-cache is broken")
	}
}

// --- Wrap() ----------------------------------------------------------------

func TestWrap_listPayload_includesSchemaVersionAndData(t *testing.T) {
	payload := map[string]any{"motes": []any{}}
	out := Wrap(payload)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		SchemaVersion int            `json:"schema_version"`
		Data          map[string]any `json:"data"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d, want %d", got.SchemaVersion, SchemaVersion)
	}
	if got.Data["motes"] == nil {
		t.Fatalf("data.motes missing in %s", b)
	}
}

func TestWrap_objectPayload_includesSchemaVersionAndData(t *testing.T) {
	type mote struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	out := Wrap(mote{ID: "motes-abc", Title: "hi"})
	b, _ := json.Marshal(out)
	var got struct {
		SchemaVersion int  `json:"schema_version"`
		Data          mote `json:"data"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Data.ID != "motes-abc" {
		t.Fatalf("data.id = %q, want motes-abc", got.Data.ID)
	}
}

func TestWrap_preservesEmptyArrayNotNull(t *testing.T) {
	// Sprint-2 §23.16 contract: an empty list is `[]`, never `null`.
	type list struct {
		Motes []string `json:"motes"`
	}
	out := Wrap(list{Motes: []string{}})
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), `"motes":[]`) {
		t.Fatalf("empty list serialized as %s; want motes:[]", b)
	}
}

// --- WrapError() -----------------------------------------------------------

func TestWrapError_includesAllThreeFields(t *testing.T) {
	out := WrapError("MOTE_NOT_FOUND", "no mote with id node-abc123")
	b, _ := json.Marshal(out)
	var got struct {
		SchemaVersion int    `json:"schema_version"`
		Error         string `json:"error"`
		Code          string `json:"code"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d, want %d", got.SchemaVersion, SchemaVersion)
	}
	if got.Code != "MOTE_NOT_FOUND" {
		t.Fatalf("code = %q, want MOTE_NOT_FOUND", got.Code)
	}
	if got.Error == "" {
		t.Fatal("error message must be non-empty")
	}
}

func TestWrapError_panicsOnEmptyCode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("WrapError with empty code did not panic")
		}
	}()
	_ = WrapError("", "msg")
}

// --- EnvelopedError --------------------------------------------------------

func TestEnvelopedError_satisfiesErrorInterface(t *testing.T) {
	var err error = &EnvelopedError{Code: "X", Message: "boom", ExitCode: 1}
	if err.Error() != "boom" {
		t.Fatalf("Error() = %q, want boom", err.Error())
	}
}

// --- EmitDeprecationNotice -------------------------------------------------

func TestEmitDeprecationNotice_firesExactlyOncePerProcess(t *testing.T) {
	ResetForTest()
	var buf bytes.Buffer
	EmitDeprecationNotice(&buf)
	EmitDeprecationNotice(&buf)
	EmitDeprecationNotice(&buf)
	got := buf.String()
	if !strings.Contains(got, EnvVar) {
		t.Fatalf("notice should name %s, got: %q", EnvVar, got)
	}
	// The text mentions the env var name twice (header and instruction) in a
	// SINGLE notice line. Count newlines instead — the notice writes exactly
	// one line per process.
	if got != "" && strings.Count(got, "\n") != 1 {
		t.Fatalf("notice should emit exactly one line per process, got %d lines: %q",
			strings.Count(got, "\n"), got)
	}
}

func TestEmitDeprecationNotice_concurrent_safe(t *testing.T) {
	ResetForTest()
	var buf bytes.Buffer
	var mu sync.Mutex
	w := lockedWriter{w: &buf, mu: &mu}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			EmitDeprecationNotice(w)
		}()
	}
	wg.Wait()
	if strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("expected exactly one notice across 50 goroutines, got: %q", buf.String())
	}
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// --- RegisteredShapes ------------------------------------------------------

func TestRegisteredShapes_isStable(t *testing.T) {
	// Defensive: a future PR that accidentally drops or reorders a shape
	// should trip this test. The list IS the contract.
	want := []string{
		"ls.list.v1",
		"pulse.list.v1",
		"stats.object.v1",
		"show.object.v1",
		"show.short.v1",
		"show.long.v1",
		"show.execution-only.v1",
		"context.list.v1",
		"error.v1",
	}
	got := RegisteredShapes()
	if len(got) != len(want) {
		t.Fatalf("RegisteredShapes len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RegisteredShapes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
