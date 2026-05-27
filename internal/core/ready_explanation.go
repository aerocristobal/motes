// SPDX-License-Identifier: MIT
//
// STORY-EXPLAIN-001 — per-mote justification surfaced by `mote ls --ready
// --explain`. BuildReadyExplanation is a pure function: it takes a mote, the
// full mote graph, and the current time, and returns a structured explanation
// for *why* the mote is ready, *when* its blockers cleared, what its parent
// epic is, and how recently it was touched. No I/O, no globals, no clock —
// the command layer passes everything in.
package core

import (
	"fmt"
	"strings"
	"time"

	"motes/internal/format"
)

// DefaultFreshnessThreshold is the boundary beyond which a ready mote is
// flagged "stale" in --explain output. 14 days was chosen to align with a
// fortnightly sprint cadence: a ready mote no agent has touched in over a
// sprint is probably stable but worth a fresh look. Confirmed with PO during
// STORY-EXPLAIN-001 sprint-5 grooming.
const DefaultFreshnessThreshold = 14 * 24 * time.Hour

// ReadyExplanation is the structured per-mote justification emitted under
// each row of `mote ls --ready --explain`. The four fields are independent:
// the reason describes blocker history, ClearedBlockers enumerates them,
// Parent describes the epic context, Freshness describes engagement recency.
type ReadyExplanation struct {
	// Reason is a human-readable summary suitable for the `ready because:`
	// line. "no blockers" when DependsOn is empty; otherwise of the form
	// "N of N blocking deps closed (id1 3d ago, id2 21d ago)".
	Reason string `json:"reason"`

	// ClearedBlockers lists the *direct* DependsOn ids in their original
	// order. A ready mote's direct blockers are all non-live by definition
	// (the readiness filter enforces this transitively). Empty when the mote
	// has no DependsOn.
	ClearedBlockers []ClearedBlocker `json:"cleared_blockers"`

	// Parent is the resolved parent epic context, or nil when the mote has
	// no Parent or the parent id can't be found in the supplied graph.
	Parent *ParentRef `json:"parent,omitempty"`

	// Freshness is always populated. NeverAccessed is true when LastAccessed
	// is nil; in that case the mote is also marked Stale because the agent
	// has no signal that anyone has touched it.
	Freshness *FreshnessRef `json:"freshness"`
}

// ClearedBlocker is a single direct dependency that has cleared (status is
// not live). ClearedAt is nil for legacy motes that predate the
// StatusChangedAt field — the renderer omits the "Nd ago" suffix in that case.
type ClearedBlocker struct {
	ID        string     `json:"id"`
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
}

// ParentRef captures the parent epic context for a ready mote. IsClosed is
// surfaced separately so the renderer can highlight a closed parent without
// needing to enumerate status enum values itself.
type ParentRef struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	IsClosed bool   `json:"is_closed"`
}

// FreshnessRef is the engagement-recency signal. SecondsSinceLastAccess is 0
// when NeverAccessed is true. Stale is a derived boolean — true when the
// mote has not been accessed within DefaultFreshnessThreshold (or has never
// been accessed at all).
type FreshnessRef struct {
	SecondsSinceLastAccess int64 `json:"seconds_since_last_access"`
	Stale                  bool  `json:"stale"`
	NeverAccessed          bool  `json:"never_accessed"`
}

// IsStale reports whether lastAccessed is older than threshold relative to
// now. A nil lastAccessed is NOT stale by this function — callers that want
// the "never accessed → stale" rule should check that case separately
// (BuildReadyExplanation does this).
func IsStale(lastAccessed *time.Time, now time.Time, threshold time.Duration) bool {
	if lastAccessed == nil {
		return false
	}
	return now.Sub(*lastAccessed) >= threshold
}

// BuildReadyExplanation constructs the full explanation for a single ready
// mote. The caller must supply the full mote graph (all motes loaded from
// the store) so blocker and parent resolution is in-memory; the function
// performs no I/O and reads no globals.
//
// Determinism: identical inputs produce byte-identical output. Blocker order
// follows m.DependsOn ordering (the on-disk order, which is the order the
// user established when linking).
func BuildReadyExplanation(m *Mote, all []*Mote, now time.Time) *ReadyExplanation {
	exp := &ReadyExplanation{
		ClearedBlockers: []ClearedBlocker{},
		Freshness:       buildFreshness(m, now, DefaultFreshnessThreshold),
	}

	// Build the lookup map once for parent + blocker resolution.
	index := make(map[string]*Mote, len(all))
	for _, am := range all {
		index[am.ID] = am
	}

	// Cleared blockers: iterate direct DependsOn in declared order. A ready
	// mote's blockers are all non-live by the --ready filter contract, so we
	// don't re-check liveness — but we do tolerate missing blockers (graph
	// gaps) by recording the id without a ClearedAt.
	for _, depID := range m.DependsOn {
		cb := ClearedBlocker{ID: depID}
		if dep, ok := index[depID]; ok && dep.StatusChangedAt != nil {
			t := *dep.StatusChangedAt
			cb.ClearedAt = &t
		}
		exp.ClearedBlockers = append(exp.ClearedBlockers, cb)
	}

	exp.Reason = buildReason(exp.ClearedBlockers, now)

	// Parent: nil when m has no Parent OR the id isn't in the graph.
	if m.Parent != "" {
		if parent, ok := index[m.Parent]; ok {
			exp.Parent = &ParentRef{
				ID:       parent.ID,
				Status:   parent.Status,
				IsClosed: format.IsClosed(parent.Status),
			}
		}
	}

	return exp
}

// buildReason renders the `ready because:` summary line. "no blockers" for
// a zero-DependsOn mote, otherwise "N of N blocking deps closed (...)" with
// inline relative-time suffixes for each blocker that has a known
// StatusChangedAt.
func buildReason(blockers []ClearedBlocker, now time.Time) string {
	n := len(blockers)
	if n == 0 {
		return "no blockers"
	}
	parts := make([]string, 0, n)
	for _, b := range blockers {
		if b.ClearedAt != nil {
			parts = append(parts, fmt.Sprintf("%s %s ago", b.ID, format.RelativeTime(now.Sub(*b.ClearedAt))))
		} else {
			parts = append(parts, b.ID)
		}
	}
	return fmt.Sprintf("%d of %d blocking deps closed (%s)", n, n, strings.Join(parts, ", "))
}

// buildFreshness folds m.LastAccessed into a FreshnessRef. A nil
// LastAccessed is treated as "never accessed" and also stale — the agent has
// no engagement signal at all, which is operationally worse than an old one.
func buildFreshness(m *Mote, now time.Time, threshold time.Duration) *FreshnessRef {
	if m.LastAccessed == nil {
		return &FreshnessRef{NeverAccessed: true, Stale: true}
	}
	delta := now.Sub(*m.LastAccessed)
	if delta < 0 {
		delta = 0
	}
	return &FreshnessRef{
		SecondsSinceLastAccess: int64(delta / time.Second),
		Stale:                  IsStale(m.LastAccessed, now, threshold),
	}
}
