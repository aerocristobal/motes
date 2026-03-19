package compliance

import (
	"encoding/json"
	"testing"
)

func TestGenerateComponentDefinition(t *testing.T) {
	doc, err := GenerateComponentDefinition()
	if err != nil {
		t.Fatalf("GenerateComponentDefinition: %v", err)
	}

	// Verify all 3 controls present
	reqs := doc.ComponentDefinition.Components[0].ControlImplementations[0].ImplementedRequirements
	controlIDs := make(map[string]bool)
	for _, r := range reqs {
		controlIDs[r.ControlID] = true
	}
	for _, expected := range []string{"si-10", "sc-28", "ac-3"} {
		if !controlIDs[expected] {
			t.Errorf("missing control %s", expected)
		}
	}

	// Verify metadata
	if doc.ComponentDefinition.Metadata.Title != "Motes" {
		t.Errorf("expected title=Motes, got %s", doc.ComponentDefinition.Metadata.Title)
	}
	if doc.ComponentDefinition.OSCALVersion != "1.1.2" {
		t.Errorf("expected oscal-version=1.1.2, got %s", doc.ComponentDefinition.OSCALVersion)
	}
	if doc.ComponentDefinition.Components[0].Type != "software" {
		t.Errorf("expected component type=software, got %s", doc.ComponentDefinition.Components[0].Type)
	}
}

func TestValidateComponentDefinition(t *testing.T) {
	doc, err := GenerateComponentDefinition()
	if err != nil {
		t.Fatalf("GenerateComponentDefinition: %v", err)
	}

	errs := ValidateComponentDefinition(doc)
	if len(errs) > 0 {
		t.Errorf("validation errors on generated doc: %v", errs)
	}
}

func TestValidateComponentDefinitionNil(t *testing.T) {
	errs := ValidateComponentDefinition(nil)
	if len(errs) != 1 || errs[0] != "document is nil" {
		t.Errorf("expected nil error, got %v", errs)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	doc, err := GenerateComponentDefinition()
	if err != nil {
		t.Fatalf("GenerateComponentDefinition: %v", err)
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var roundTripped OSCALComponentDefinition
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify structure preserved
	if roundTripped.ComponentDefinition.Metadata.Title != doc.ComponentDefinition.Metadata.Title {
		t.Error("title mismatch after round-trip")
	}
	origReqs := doc.ComponentDefinition.Components[0].ControlImplementations[0].ImplementedRequirements
	rtReqs := roundTripped.ComponentDefinition.Components[0].ControlImplementations[0].ImplementedRequirements
	if len(rtReqs) != len(origReqs) {
		t.Errorf("requirements count mismatch: %d vs %d", len(rtReqs), len(origReqs))
	}
	for i, r := range rtReqs {
		if r.ControlID != origReqs[i].ControlID {
			t.Errorf("control-id mismatch at %d: %s vs %s", i, r.ControlID, origReqs[i].ControlID)
		}
	}
}
