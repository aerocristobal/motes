// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// buildConfigNode marshals cfg through yaml.v3 and decorates the tree with
// head comments on user-facing fields (notably the dream provider backends and
// auth values). Comments survive write-back because yaml.v3 preserves them on
// Marshal of *yaml.Node — a property that the plain struct→bytes round-trip
// in pre-multi-provider versions of SaveConfig did not have.
//
// Schema-coupled lookups are intentionally narrow: only the dream.provider
// subtree is decorated. Adding new commented fields means extending this
// function — and only this function.
func buildConfigNode(cfg *Config) (*yaml.Node, error) {
	bytes, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("pre-marshal config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(bytes, &doc); err != nil {
		return nil, fmt.Errorf("re-parse config: %w", err)
	}

	// doc is a DocumentNode whose first child is the root mapping.
	doc.HeadComment = "Motes configuration. See docs/configuration.md for the full reference."

	root := documentRoot(&doc)
	if root == nil {
		return &doc, nil
	}

	if dream := mappingValue(root, "dream"); dream != nil {
		if provider := mappingValue(dream, "provider"); provider != nil {
			decorateProviderEntry(provider, "batch")
			decorateProviderEntry(provider, "reconciliation")
		}
	}

	return &doc, nil
}

// documentRoot returns the root mapping of a yaml document node, or nil if the
// document is empty.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

// mappingValue looks up `key` in a mapping node and returns the value node.
// Returns nil if the node is not a mapping or the key is absent.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	// Mapping content alternates: key0, value0, key1, value1, ...
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// mappingKey returns the key node for `key` in a mapping (so comments can be
// attached to the key itself, which is where yaml.v3 emits them).
func mappingKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i]
		}
	}
	return nil
}

// decorateProviderEntry attaches per-field head comments to one stage's
// provider entry (batch or reconciliation). No-op when the stage is missing.
func decorateProviderEntry(provider *yaml.Node, stage string) {
	stageNode := mappingValue(provider, stage)
	if stageNode == nil {
		return
	}
	if k := mappingKey(stageNode, "backend"); k != nil {
		k.HeadComment = "Backend: claude-cli | openai | gemini. See docs/configuration.md#provider-backends."
	}
	if k := mappingKey(stageNode, "auth"); k != nil {
		k.HeadComment = "Auth: env var name (recommended) holding the credential, or 'vertex-ai' for Gemini ADC, or 'oauth' for claude-cli."
	}
	if k := mappingKey(stageNode, "model"); k != nil {
		k.HeadComment = "Model identifier as the backend expects it (e.g. claude-sonnet-4-6, gpt-4o, gemini-2.5-flash)."
	}
	if stageKey := mappingKey(provider, stage); stageKey != nil && stageKey.HeadComment == "" {
		switch stage {
		case "batch":
			stageKey.HeadComment = "Batch reasoning stage — runs once per mote batch (cheap/fast model)."
		case "reconciliation":
			stageKey.HeadComment = "Reconciliation stage — single high-capability call that filters batch visions."
		}
	}
}
