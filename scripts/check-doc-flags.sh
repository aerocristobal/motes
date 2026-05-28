#!/usr/bin/env bash
# scripts/check-doc-flags.sh — thin shim around the in-tree
# `mote check-doc-flags` subcommand. Exists so the contract surface
# named by STORY-DOCFLAGS-001 (a runnable script at a stable path)
# stays callable even when the subcommand binary is reorganized.
#
# All real logic lives in cmd/mote/cmd_check_doc_flags.go and the
# internal/docflags/ package; this script only wires up the binary
# from the local source tree and forwards arguments.
#
# Exit codes match the subcommand:
#   0  every doc-referenced flag is valid (or allowlisted/suppressed)
#   1  one or more violations
#   2  operational error (missing files, bad invocation)
#
# Usage:
#   scripts/check-doc-flags.sh [extra mote-check-doc-flags args]

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
binary="${repo_root}/mote"

if [ ! -x "$binary" ]; then
    echo "check-doc-flags: building mote binary..." >&2
    (cd "$repo_root" && go build -o "$binary" ./cmd/mote)
fi

exec "$binary" check-doc-flags --root "$repo_root" "$@"
