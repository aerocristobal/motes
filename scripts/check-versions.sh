#!/usr/bin/env bash
# scripts/check-versions.sh — verify that every tracked version-bearing
# location agrees with the canonical version constant in
# internal/version/version.go. Backs STORY-VERSIONS-001.
#
# Exit codes:
#   0  every tracked location equals the canonical value
#   1  one or more locations disagree (file:line: expected X, found Y)
#   2  operational error (missing canonical file, missing tracked file, etc.)
#
# Usage:
#   scripts/check-versions.sh [repo_root]
#
# repo_root defaults to the current working directory.
#
# Adding a new tracked location: append an entry to the `tracked` array
# below. Each entry is three fields joined by '|':
#   <relative_path>|<extended_regex_with_capture_group_1>|<heading_text>
# The checker locates `## <heading_text>` (or any deeper heading level)
# and compares the FIRST line after that heading matching the regex.
# Only the head-of-list entry is checked: historical entries are by
# design not policed (STORY-VERSIONS-001 Scenario 7).

set -euo pipefail

repo_root="${1:-.}"
canonical_file="${repo_root}/internal/version/version.go"

if [ ! -f "$canonical_file" ]; then
    echo "check-versions: missing canonical file $canonical_file" >&2
    exit 2
fi

canonical=$(grep -E 'const[[:space:]]+Value[[:space:]]*=[[:space:]]*"[^"]+"' "$canonical_file" \
    | head -n1 \
    | sed -E 's/.*const[[:space:]]+Value[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/')
if [ -z "$canonical" ]; then
    echo "check-versions: could not parse 'const Value = \"...\"' in $canonical_file" >&2
    exit 2
fi

# Hardcoded registry of tracked version-bearing locations.
# Format: <path>|<extended-regex with version in capture group 1>|<heading>
tracked=(
    "docs/version-history.md|^- \*\*v([0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?)\*\*|Version History"
)

violations=0

for entry in "${tracked[@]}"; do
    IFS='|' read -r path regex heading <<< "$entry"
    file_path="${repo_root}/${path}"
    if [ ! -f "$file_path" ]; then
        echo "${path}: tracked file not found" >&2
        violations=$((violations + 1))
        continue
    fi

    heading_lineno=$(grep -nE "^#+[[:space:]]+${heading}[[:space:]]*$" "$file_path" \
        | head -n1 | cut -d: -f1 || true)
    if [ -z "$heading_lineno" ]; then
        echo "${path}: heading '${heading}' not found" >&2
        violations=$((violations + 1))
        continue
    fi

    found=""
    found_lineno=""
    lineno=0
    while IFS= read -r line || [ -n "$line" ]; do
        lineno=$((lineno + 1))
        if [ "$lineno" -le "$heading_lineno" ]; then
            continue
        fi
        if [[ "$line" =~ $regex ]]; then
            found="${BASH_REMATCH[1]}"
            found_lineno=$lineno
            break
        fi
    done < "$file_path"

    if [ -z "$found" ]; then
        echo "${path}: no entry matching the version regex found after heading '${heading}'" >&2
        violations=$((violations + 1))
        continue
    fi

    if [ "$found" != "$canonical" ]; then
        echo "${path}:${found_lineno}: expected ${canonical}, found ${found}"
        violations=$((violations + 1))
    fi
done

if [ "$violations" -gt 0 ]; then
    echo "check-versions: ${violations} violation(s); canonical is ${canonical}" >&2
    exit 1
fi

echo "check-versions: ok (canonical=${canonical}, locations checked=${#tracked[@]})"
exit 0
