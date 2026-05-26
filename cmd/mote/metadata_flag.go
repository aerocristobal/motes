// SPDX-License-Identifier: MIT
//
// STORY-MQRY-001 — shared flag-parsing and validation for --metadata-field
// and --has-metadata-key. Both `mote ls` and `mote search` use this code so
// the CLI surface stays in lock-step.
package main

import (
	"fmt"
	"strings"

	"motes/internal/security"
)

// parseMetadataFieldFlag splits a single --metadata-field argument of the
// form "key=value" on the FIRST '=' only. A value may contain further '='
// characters; they are retained as-is. Missing '=' returns a user-facing
// error pointing at the expected format.
func parseMetadataFieldFlag(raw string) (key, value string, err error) {
	idx := strings.IndexByte(raw, '=')
	if idx < 0 {
		return "", "", fmt.Errorf("--metadata-field: expected format key=value, got %q", raw)
	}
	return raw[:idx], raw[idx+1:], nil
}

// resolveMetadataFlags parses and validates the raw --metadata-field /
// --has-metadata-key flag values. On the first failure it returns the
// validator's error verbatim so the CLI surfaces e.g. "invalid metadata key".
// On success it returns a map suitable for ListFilters.MetadataFields and the
// validated slice for ListFilters.HasMetadataKeys.
func resolveMetadataFlags(rawFields []string, hasKeys []string) (map[string]string, []string, error) {
	var fields map[string]string
	if len(rawFields) > 0 {
		fields = make(map[string]string, len(rawFields))
		for _, raw := range rawFields {
			k, v, err := parseMetadataFieldFlag(raw)
			if err != nil {
				return nil, nil, err
			}
			if err := security.ValidateMetadataKey(k); err != nil {
				return nil, nil, err
			}
			if err := security.ValidateMetadataValue(v); err != nil {
				return nil, nil, err
			}
			fields[k] = v
		}
	}
	for _, k := range hasKeys {
		if err := security.ValidateMetadataKey(k); err != nil {
			return nil, nil, err
		}
	}
	return fields, hasKeys, nil
}
