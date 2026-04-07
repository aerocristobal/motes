// SPDX-License-Identifier: AGPL-3.0-or-later
package compliance

import (
	"crypto/rand"
	"fmt"
)

// OSCALComponentDefinition is the top-level OSCAL component-definition document.
type OSCALComponentDefinition struct {
	ComponentDefinition ComponentDefinition `json:"component-definition"`
}

// ComponentDefinition contains metadata and component details.
type ComponentDefinition struct {
	UUID         string      `json:"uuid"`
	Metadata     Metadata    `json:"metadata"`
	Components   []Component `json:"components"`
	OSCALVersion string      `json:"oscal-version"`
}

// Metadata provides document-level information.
type Metadata struct {
	Title        string `json:"title"`
	Version      string `json:"version"`
	OSCALVersion string `json:"oscal-version"`
	LastModified string `json:"last-modified"`
}

// Component describes a software component and its control implementations.
type Component struct {
	UUID                   string                  `json:"uuid"`
	Type                   string                  `json:"type"`
	Title                  string                  `json:"title"`
	Description            string                  `json:"description"`
	ControlImplementations []ControlImplementation `json:"control-implementations"`
}

// ControlImplementation groups implemented requirements under a source framework.
type ControlImplementation struct {
	UUID                     string                    `json:"uuid"`
	Source                   string                    `json:"source"`
	Description              string                    `json:"description"`
	ImplementedRequirements  []ImplementedRequirement  `json:"implemented-requirements"`
}

// ImplementedRequirement maps a control to its implementation description.
type ImplementedRequirement struct {
	UUID        string `json:"uuid"`
	ControlID   string `json:"control-id"`
	Description string `json:"description"`
}

// generateUUID produces a random UUID v4 string.
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
