#!/bin/bash
# Capture benchmark results with a metadata header.
# Usage: bash bench/run.sh [output-file]
#
# Outputs to bench/run_<timestamp>.txt by default.
# To establish a new baseline: cp bench/run_<timestamp>.txt bench/baseline.txt
set -euo pipefail

cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUT="${1:-bench/run_${TIMESTAMP}.txt}"

mkdir -p bench

{
  echo "# motes benchmark — $(date)"
  echo "# commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  echo "# go: $(go version)"
  echo ""
  go test -bench=. -benchmem -count=3 ./internal/core/
} | tee "$OUT"

echo ""
echo "Saved: $OUT"
if [ -f "bench/baseline.txt" ]; then
  echo "Compare: bash bench/compare.sh bench/baseline.txt $OUT"
else
  echo "To set baseline: cp $OUT bench/baseline.txt"
fi
