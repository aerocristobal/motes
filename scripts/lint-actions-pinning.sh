#!/usr/bin/env bash
# Enforce STORY-CIHYG-001 §17: every `uses:` reference in .github/workflows/*.y{,a}ml
# must be pinned to a 40-character lowercase hex commit SHA followed on the same
# line by a trailing `# <tag>` comment.
#
# Exit codes:
#   0  every uses: line is SHA-pinned with a tag comment
#   1  one or more violations found (details on stdout)
#   2  operational error (no workflow files present, bad invocation)
#
# Usage:
#   scripts/lint-actions-pinning.sh [repo_root]
#
# repo_root defaults to the current working directory.

set -euo pipefail

repo_root="${1:-.}"
workflows_dir="${repo_root}/.github/workflows"

if [ ! -d "$workflows_dir" ]; then
    echo "lint-actions-pinning: no $workflows_dir directory" >&2
    exit 2
fi

shopt -s nullglob
workflow_files=("$workflows_dir"/*.yml "$workflows_dir"/*.yaml)
shopt -u nullglob

if [ "${#workflow_files[@]}" -eq 0 ]; then
    echo "lint-actions-pinning: no workflow files in $workflows_dir" >&2
    exit 2
fi

violations=0

for f in "${workflow_files[@]}"; do
    # Read the file line-by-line, preserving line numbers.
    lineno=0
    while IFS= read -r line || [ -n "$line" ]; do
        lineno=$((lineno + 1))

        # Match `uses:` lines (with optional list marker and leading whitespace).
        # Captures everything after `uses:` as the value.
        if [[ ! "$line" =~ ^[[:space:]]*-?[[:space:]]*uses:[[:space:]]+(.*)$ ]]; then
            continue
        fi
        value="${BASH_REMATCH[1]}"

        # Strip surrounding quotes if present.
        value="${value#\"}"; value="${value%\"}"
        value="${value#\'}"; value="${value%\'}"

        # Local composite actions (./path or ../path) are not subject to the
        # SHA-pinning rule because they live in the repo at a known commit.
        if [[ "$value" =~ ^\.{1,2}/ ]]; then
            continue
        fi

        # Split into ref portion and trailing comment (if any).
        ref_part="$value"
        comment_part=""
        if [[ "$value" == *" #"* ]]; then
            ref_part="${value%% #*}"
            # Everything from the first " #" onward is the comment.
            comment_part="${value#* #}"
        elif [[ "$value" == *$'\t#'* ]]; then
            ref_part="${value%%	#*}"
            comment_part="${value#*	#}"
        fi
        # Trim trailing whitespace on ref.
        ref_part="${ref_part%"${ref_part##*[![:space:]]}"}"

        # Must contain @ to separate action name from ref.
        if [[ "$ref_part" != *@* ]]; then
            echo "${f}:${lineno}: uses: value has no '@<ref>' (uses: ${ref_part})"
            violations=$((violations + 1))
            continue
        fi

        ref="${ref_part##*@}"

        # Ref must be exactly 40 lowercase hex characters.
        if [[ ! "$ref" =~ ^[a-f0-9]{40}$ ]]; then
            echo "${f}:${lineno}: uses: ref is not a 40-char SHA (uses: ${ref_part})"
            violations=$((violations + 1))
            continue
        fi

        # A trailing tag comment is required for human-readable provenance.
        # Trim leading whitespace on the comment to detect emptiness.
        comment_trimmed="${comment_part#"${comment_part%%[![:space:]]*}"}"
        if [ -z "$comment_trimmed" ]; then
            echo "${f}:${lineno}: missing tag comment after SHA (uses: ${ref_part})"
            violations=$((violations + 1))
            continue
        fi
    done < "$f"
done

if [ "$violations" -gt 0 ]; then
    echo "lint-actions-pinning: ${violations} violation(s) found" >&2
    exit 1
fi

exit 0
