package main

import (
	"testing"
	"time"

	"motes/internal/core"
)

func TestComputeFlowStats(t *testing.T) {
	now := time.Now()

	ago := func(days int) time.Time { return now.Add(-time.Duration(days) * 24 * time.Hour) }
	agoPtr := func(days int) *time.Time { t2 := ago(days); return &t2 }

	motes := []*core.Mote{
		// Active motes at various ages
		{CreatedAt: ago(3), Status: "active"},   // 7d, 30d, 90d
		{CreatedAt: ago(15), Status: "active"},  // 30d, 90d
		{CreatedAt: ago(60), Status: "active"},  // 90d only
		{CreatedAt: ago(120), Status: "active"}, // none

		// Deprecated motes — count in Created AND Deprecated windows
		{CreatedAt: ago(5), Status: "deprecated", StatusChangedAt: agoPtr(2)},   // created: 30d,90d; dep 7d,30d,90d
		{CreatedAt: ago(20), Status: "deprecated", StatusChangedAt: agoPtr(20)}, // created: 30d,90d; dep 30d,90d
		{CreatedAt: ago(80), Status: "deprecated", StatusChangedAt: agoPtr(95)}, // created: 90d; dep none

		// Deprecated but nil StatusChangedAt — excluded from deprecated windows
		{CreatedAt: ago(8), Status: "deprecated"}, // created: 30d, 90d; dep none
	}

	// Created counts all motes regardless of status:
	// 7d:  3d, 5d                       = 2
	// 30d: 3d, 15d, 5d, 20d, 8d        = 5
	// 90d: 3d, 15d, 60d, 5d, 20d, 80d, 8d = 7 (only 120d is outside)
	//
	// Deprecated counts motes with StatusChangedAt set:
	// 7d:  2d                = 1
	// 30d: 2d, 20d           = 2
	// 90d: 2d, 20d           = 2 (95d is outside)

	fs := computeFlowStats(motes)

	if fs.Created7d != 2 {
		t.Errorf("Created7d: want 1, got %d", fs.Created7d)
	}
	if fs.Created30d != 5 {
		t.Errorf("Created30d: want 5, got %d", fs.Created30d)
	}
	if fs.Created90d != 7 {
		t.Errorf("Created90d: want 7, got %d", fs.Created90d)
	}
	if fs.Deprecated7d != 1 {
		t.Errorf("Deprecated7d: want 1, got %d", fs.Deprecated7d)
	}
	if fs.Deprecated30d != 2 {
		t.Errorf("Deprecated30d: want 2, got %d", fs.Deprecated30d)
	}
	if fs.Deprecated90d != 2 {
		t.Errorf("Deprecated90d: want 2, got %d", fs.Deprecated90d)
	}
}
