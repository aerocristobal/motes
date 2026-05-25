// SPDX-License-Identifier: MIT
package core

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// STORY-DISC-001 Scenario 1: discovered_from round-trips through YAML using
// the exact key name "discovered_from".
func TestMote_DiscoveredFrom_YAMLRoundTrip(t *testing.T) {
	in := &Mote{
		ID:             "motes-BBB",
		Type:           "task",
		Status:         "active",
		Title:          "Follow-up",
		DiscoveredFrom: []string{"motes-AAA", "motes-CCC"},
	}
	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "discovered_from:") {
		t.Errorf("YAML output missing discovered_from key:\n%s", data)
	}
	var out Mote
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.DiscoveredFrom) != 2 || out.DiscoveredFrom[0] != "motes-AAA" || out.DiscoveredFrom[1] != "motes-CCC" {
		t.Errorf("round-trip: got %v, want [motes-AAA motes-CCC]", out.DiscoveredFrom)
	}
}

// STORY-DISC-001 Scenario 8: when discovered_from is empty after unlink, the
// YAML must omit the field entirely (omitempty).
func TestMote_DiscoveredFrom_EmptyOmitted(t *testing.T) {
	m := &Mote{
		ID:     "motes-XXX",
		Type:   "task",
		Status: "active",
		Title:  "No discoveries",
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "discovered_from") {
		t.Errorf("empty DiscoveredFrom must be omitted from YAML, got:\n%s", data)
	}
}
