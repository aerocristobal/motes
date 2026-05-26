// SPDX-License-Identifier: MIT
//
// Package jsonenv is the single source of truth for mote's versioned JSON
// envelope contract (STORY-JSCHEMA-001 / docs/beads-recommendations.md §23.7).
//
// The envelope is opt-in during v0.5.x via MOTE_JSON_ENVELOPE=1. In envelope
// mode every JSON-emitting read command wraps its payload as
//
//	{"schema_version": 1, "data": <existing payload>}
//
// on stdout for success, and main.go emits
//
//	{"schema_version": 1, "error": "<message>", "code": "<STABLE_CODE>"}
//
// on stderr for failure when a command returns an *EnvelopedError. In legacy
// mode the existing raw shape is preserved byte-for-byte; a one-line stderr
// deprecation notice fires exactly once per process so consumers know the
// shape is changing.
package jsonenv

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// SchemaVersion is the umbrella integer that bumps only when any documented
// shape breaks (rename, removal, type change). Additive changes (new optional
// field, new enum value, new top-level key) do NOT bump it. See
// docs/JSON_SCHEMA.md §2.
const SchemaVersion = 1

// EnvelopeMode is the binary decision returned by Mode().
type EnvelopeMode int

const (
	// ModeLegacy emits the unwrapped pre-envelope JSON shape on stdout.
	ModeLegacy EnvelopeMode = iota
	// ModeEnvelope wraps every JSON payload in {schema_version, data}.
	ModeEnvelope
)

// EnvVar is the environment variable consulted by Mode(). Exported so tests
// and docs can reference a single constant.
const EnvVar = "MOTE_JSON_ENVELOPE"

var (
	modeOnce sync.Once
	modeVal  EnvelopeMode

	deprecationOnce sync.Once
)

// ResetForTest clears the package-level sync.Once cache so a test can flip
// the env var or re-trigger the once-per-process deprecation notice. It is
// exported because cmd/mote integration tests live in a different package
// and need to drive the mode value across many sub-tests in one `go test`
// process. Do not call from production code.
func ResetForTest() {
	modeOnce = sync.Once{}
	deprecationOnce = sync.Once{}
}

// Mode reads MOTE_JSON_ENVELOPE once per process and caches the result. The
// caching matters: a single `mote` invocation can call into multiple JSON
// branches via internal helpers, and they must all agree on the mode.
//
//	"1"      → ModeEnvelope
//	unset    → ModeLegacy
//	""       → ModeLegacy
//	"0"      → ModeLegacy
//	anything → ModeLegacy with a one-shot stderr warning (so a typo of
//	           MOTE_JSON_ENVELOPE=true does not silently fall through to
//	           legacy and confuse the operator)
func Mode() EnvelopeMode {
	modeOnce.Do(func() {
		v := os.Getenv(EnvVar)
		switch v {
		case "1":
			modeVal = ModeEnvelope
		case "", "0":
			modeVal = ModeLegacy
		default:
			_, _ = fmt.Fprintf(os.Stderr,
				"warning: %s=%q is not recognized (expected \"1\" or \"0\"); falling back to legacy JSON\n",
				EnvVar, v)
			modeVal = ModeLegacy
		}
	})
	return modeVal
}

// envelope is the on-the-wire success shape. Named field order is intentional:
// schema_version first so a consumer doing a streaming parse sees the version
// before the (potentially large) data payload.
type envelope struct {
	SchemaVersion int `json:"schema_version"`
	Data          any `json:"data"`
}

// errorEnvelope is the on-the-wire failure shape. Same ordering rationale.
type errorEnvelope struct {
	SchemaVersion int    `json:"schema_version"`
	Error         string `json:"error"`
	Code          string `json:"code"`
}

// Wrap returns the envelope {schema_version, data}. The caller is responsible
// for marshalling — Wrap intentionally returns an interface value so it can be
// passed directly to encoding/json.MarshalIndent without an extra allocation.
func Wrap(data any) any {
	return envelope{SchemaVersion: SchemaVersion, Data: data}
}

// WrapError returns the failure envelope. It panics on empty code: every
// error site MUST register a stable code in docs/JSON_SCHEMA.md, and an
// empty code at runtime would silently break that contract. Failing fast
// in development is safer than emitting a malformed envelope.
func WrapError(code, msg string) any {
	if code == "" {
		panic("jsonenv.WrapError: code is required (see docs/JSON_SCHEMA.md §5)")
	}
	return errorEnvelope{SchemaVersion: SchemaVersion, Error: msg, Code: code}
}

// EnvelopedError is the error type main.go recognises to emit a JSON error
// envelope on stderr. RunE paths inside JSON-emitting commands return this
// (typically via the cmd/mote helper) when the caller requested --json and
// envelope mode is active.
//
// ExitCode is honoured by main() the same way *exitCodeError works today, so
// the convention `1 = error, 2 = contention` is preserved.
type EnvelopedError struct {
	Code     string
	Message  string
	ExitCode int
}

// Error satisfies the error interface. The plain message is used when the
// caller is in legacy mode and the wrapper falls back to printing prose.
func (e *EnvelopedError) Error() string { return e.Message }

// EmitDeprecationNotice writes a one-line notice to w exactly once per process,
// regardless of how many JSON-emitting commands run inside that process. Used
// at the top of every legacy-mode JSON branch so the operator sees the signal
// without flooding stderr in tight loops.
func EmitDeprecationNotice(w io.Writer) {
	deprecationOnce.Do(func() {
		_, _ = fmt.Fprintf(w,
			"notice: legacy JSON shape is deprecated; opt in with %s=1 (default-on in v0.6.x; legacy removed in v0.7.x)\n",
			EnvVar)
	})
}

// RegisteredShapes returns the compile-time list of stable JSON shape names.
// docs/JSON_SCHEMA.md MUST document every name in this list; the mote doctor
// drift check (cmd/mote/cmd_doctor.go) flags any registered shape that is
// missing from the docs file, and any missing docs file.
//
// To add a new --json-emitting command:
//
//  1. Append the new shape name here (e.g. "search.list.v1").
//  2. Add the corresponding subsection to docs/JSON_SCHEMA.md §4.
//  3. Wire the command's JSON branch through Wrap(...) in envelope mode.
//
// Renames or breaking changes to existing shapes require bumping SchemaVersion
// and adding a new entry (e.g. ls.list.v2) rather than mutating an old one.
func RegisteredShapes() []string {
	return []string{
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
}
