package main

import (
	"encoding/json"
	"testing"

	"motes/internal/compliance"
)

func TestComplianceExport(t *testing.T) {
	doc, err := compliance.GenerateComponentDefinition()
	if err != nil {
		t.Fatalf("GenerateComponentDefinition: %v", err)
	}

	// Verify 3 control IDs present
	reqs := doc.ComponentDefinition.Components[0].ControlImplementations[0].ImplementedRequirements
	controlIDs := make(map[string]bool)
	for _, r := range reqs {
		controlIDs[r.ControlID] = true
	}
	for _, expected := range []string{"si-10", "sc-28", "ac-3"} {
		if !controlIDs[expected] {
			t.Errorf("missing control %s in output", expected)
		}
	}

	// Verify valid structure
	errs := compliance.ValidateComponentDefinition(doc)
	if len(errs) > 0 {
		t.Errorf("validation errors: %v", errs)
	}

	// Verify JSON marshal produces valid output
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var roundTripped compliance.OSCALComponentDefinition
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify OSCAL version
	if roundTripped.ComponentDefinition.OSCALVersion != "1.1.2" {
		t.Errorf("expected oscal-version=1.1.2, got %s", roundTripped.ComponentDefinition.OSCALVersion)
	}
}

func TestComplianceExportInvalidFormat(t *testing.T) {
	err := runComplianceExport(complianceExportCmd, nil)
	// complianceFormat is empty at this point, but let's test with a bad value
	origFormat := complianceFormat
	complianceFormat = "csv"
	defer func() { complianceFormat = origFormat }()

	err = runComplianceExport(complianceExportCmd, nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}
