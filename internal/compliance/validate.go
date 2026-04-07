// SPDX-License-Identifier: AGPL-3.0-or-later
package compliance

// ValidateComponentDefinition checks required fields in an OSCAL component-definition.
// Returns a slice of error strings (empty means valid).
func ValidateComponentDefinition(doc *OSCALComponentDefinition) []string {
	var errs []string
	if doc == nil {
		return []string{"document is nil"}
	}

	cd := doc.ComponentDefinition
	if cd.OSCALVersion == "" {
		errs = append(errs, "missing oscal-version")
	}
	if cd.Metadata.Title == "" {
		errs = append(errs, "missing metadata.title")
	}
	if cd.UUID == "" {
		errs = append(errs, "missing component-definition UUID")
	}
	if len(cd.Components) == 0 {
		errs = append(errs, "no components defined")
	}
	for i, comp := range cd.Components {
		if comp.UUID == "" {
			errs = append(errs, "component missing UUID")
		}
		if comp.Type == "" {
			errs = append(errs, "component missing type")
		}
		if len(comp.ControlImplementations) == 0 {
			errs = append(errs, "component has no control-implementations")
		}
		for _, ci := range comp.ControlImplementations {
			if ci.UUID == "" {
				errs = append(errs, "control-implementation missing UUID")
			}
			if len(ci.ImplementedRequirements) == 0 {
				errs = append(errs, "control-implementation has no implemented-requirements")
			}
			_ = i // suppress unused warning
		}
	}
	return errs
}
