#!/bin/bash
# Compare two benchmark result files.
# Usage: bash bench/compare.sh <baseline> <candidate>
#
# Uses benchstat if installed (recommended).
# Install: go install golang.org/x/perf/cmd/benchstat@latest
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <baseline> <candidate>"
  exit 1
fi

BASELINE="$1"
CANDIDATE="$2"

if [ ! -f "$BASELINE" ]; then
  echo "Error: baseline file not found: $BASELINE"
  exit 1
fi
if [ ! -f "$CANDIDATE" ]; then
  echo "Error: candidate file not found: $CANDIDATE"
  exit 1
fi

if command -v benchstat &>/dev/null; then
  benchstat "$BASELINE" "$CANDIDATE"
else
  echo "benchstat not found — showing raw diff."
  echo "Install: go install golang.org/x/perf/cmd/benchstat@latest"
  echo ""
  diff "$BASELINE" "$CANDIDATE" || true
fi
