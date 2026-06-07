#!/usr/bin/env bash
# scripts/test.sh — single canonical test invocation for mote. Honors
# .test-skip (with mandatory `# mote-<id>` rationale), applies a 5m default
# timeout, and maps TEST_VERBOSE / TEST_RUN / TEST_TIMEOUT env vars to the
# matching `go test` flags. Backs STORY-TWRAP-001.
#
# Exit codes:
#   0  go test succeeded (every test passed or was skipped)
#   1  go test reported one or more failing tests
#   2  malformed .test-skip (missing `# mote-<id>` rationale) OR `go test`
#      itself returned a non-1/non-0 status (e.g. compile error)
#
# Usage:
#   scripts/test.sh [repo_root]
#
# repo_root defaults to the current working directory. The wrapper looks
# for `.test-skip` at `${repo_root}/.test-skip`.
#
# Env vars:
#   TEST_TIMEOUT  override the default 5m (passed as `-timeout=<value>`)
#   TEST_VERBOSE  =1 adds `-v`
#   TEST_RUN      regex passed as `-run <value>`
#
# Contract:
#   - stdout / stderr from `go test` are forwarded unmodified.
#   - The `go test` exit code propagates verbatim.
#   - A `.test-skip` entry that matches no real test emits a post-run
#     WARNING on stderr but does NOT change the exit code.
#   - A `.test-skip` line without a `# mote-<id>` rationale comment exits
#     with code 2 BEFORE running `go test`.

set -euo pipefail

repo_root="${1:-.}"
default_timeout="${TEST_TIMEOUT:-5m}"
skip_file="${repo_root}/.test-skip"

args=( "test" "-timeout=${default_timeout}" )
if [ "${TEST_VERBOSE:-}" = "1" ]; then
    args+=( "-v" )
fi
if [ -n "${TEST_RUN:-}" ]; then
    args+=( "-run" "${TEST_RUN}" )
fi

skip_names=()
if [ -f "$skip_file" ]; then
    lineno=0
    while IFS= read -r raw || [ -n "$raw" ]; do
        lineno=$((lineno + 1))

        # Trim leading whitespace.
        line="${raw#"${raw%%[![:space:]]*}"}"

        # Skip blank lines and full-line comments.
        if [ -z "$line" ]; then
            continue
        fi
        if [[ "$line" == \#* ]]; then
            continue
        fi

        # Enforced shape:
        #   <TestName>[whitespace]#[whitespace]mote-<id>[ - <reason>]
        # TestName matches Go's identifier rule for test funcs.
        if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*\#[[:space:]]*mote-[A-Za-z0-9]+ ]]; then
            skip_names+=( "${BASH_REMATCH[1]}" )
        else
            echo ".test-skip line ${lineno}: missing rationale." >&2
            echo "Format: <TestName>  # mote-<id> — <reason>" >&2
            echo "(Use \`mote add --type=task\` if no mote tracks this yet.)" >&2
            exit 2
        fi
    done < "$skip_file"
fi

if [ ${#skip_names[@]} -gt 0 ]; then
    if [ ${#skip_names[@]} -eq 1 ]; then
        skip_regex="^${skip_names[0]}\$"
    else
        joined="${skip_names[0]}"
        i=1
        while [ "$i" -lt "${#skip_names[@]}" ]; do
            joined="${joined}|${skip_names[$i]}"
            i=$((i + 1))
        done
        skip_regex="^(${joined})\$"
    fi
    args+=( "-skip" "$skip_regex" )

    summary=""
    for n in "${skip_names[@]}"; do
        if [ -z "$summary" ]; then
            summary="$n"
        else
            summary="${summary}, $n"
        fi
    done
    echo "skipping ${#skip_names[@]} tests per .test-skip: ${summary}" >&2
fi

args+=( "./..." )

# Run the suite. Do NOT mask `go test`'s exit code — compile errors must
# propagate (exit 2 from `go test` on TYPECHECK / IMPORT failures), as
# must regular test failures (exit 1).
set +e
go "${args[@]}"
rc=$?
set -e

# Stale-entry warning (Scenario 8). Only runs when:
#   - .test-skip had entries (otherwise nothing to validate), AND
#   - the main run actually compiled (rc 0 = all pass, rc 1 = some fail).
#     rc 2 means `go test` itself failed to build; skip the listing pass
#     to avoid stacking noise on top of the real compile diagnostic.
if [ ${#skip_names[@]} -gt 0 ] && { [ "$rc" -eq 0 ] || [ "$rc" -eq 1 ]; }; then
    listing=$(go test -list '.*' ./... 2>/dev/null \
              | grep -E '^Test[A-Za-z0-9_]*$' || true)
    for n in "${skip_names[@]}"; do
        if ! grep -qxF -- "$n" <<<"$listing"; then
            echo "WARNING: .test-skip entry ${n} matched 0 tests" >&2
        fi
    done
fi

exit "$rc"
