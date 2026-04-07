// SPDX-License-Identifier: AGPL-3.0-or-later
package compliance

import "time"

// GenerateComponentDefinition builds an OSCAL component-definition document
// describing motes' security controls.
func GenerateComponentDefinition() (*OSCALComponentDefinition, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	var requirements []ImplementedRequirement
	for _, cm := range controlMappings() {
		requirements = append(requirements, ImplementedRequirement{
			UUID:        generateUUID(),
			ControlID:   cm.ControlID,
			Description: cm.Description,
		})
	}

	doc := &OSCALComponentDefinition{
		ComponentDefinition: ComponentDefinition{
			UUID: generateUUID(),
			Metadata: Metadata{
				Title:        "Motes",
				Version:      "1.0.0",
				OSCALVersion: "1.1.2",
				LastModified: now,
			},
			OSCALVersion: "1.1.2",
			Components: []Component{
				{
					UUID:        generateUUID(),
					Type:        "software",
					Title:       "Motes",
					Description: "AI-native context and memory system with atomic mote storage",
					ControlImplementations: []ControlImplementation{
						{
							UUID:                    generateUUID(),
							Source:                   "NIST_SP-800-53_rev5",
							Description:             "NIST 800-53 Rev 5 control implementations",
							ImplementedRequirements: requirements,
						},
					},
				},
			},
		},
	}

	return doc, nil
}
