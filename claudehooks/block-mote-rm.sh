#!/usr/bin/env bash
# block-mote-rm.sh — Protect mote's append-mostly storage from raw deletes
# and protect derived index files from direct writes.
#
# Mote-specific; adapted from docs/beads-recommendations.md §1.
# Installed by `mote onboard` into ~/.claude/hooks/.
#
# Denies:
#   - rm / trash against .memory/nodes/**          → use `mote delete <id>`
#   - writes to .memory/index.jsonl                → run `mote index rebuild`
#   - writes to .memory/mote_bm25.json             → run `mote index rebuild`
#   - writes to .memory/strata/<corpus>/bm25.json  → use mote strata tooling

set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "warning: block-mote-rm.sh: jq not found, allowing command through" >&2
  exit 0
fi

CMD=$(jq -r '.tool_input.command // empty')
[ -z "$CMD" ] && exit 0

emit_deny() {
  jq -nc --arg reason "$1" \
    '{hookSpecificOutput:{permissionDecision:"deny",permissionDecisionReason:$reason}}'
  exit 0
}

first_word=$(awk '{print $1}' <<<"$CMD")

# Case 1: rm / trash touching .memory/nodes/**.
case "$first_word" in
  rm|trash)
    if [[ "$CMD" =~ \.memory/nodes ]]; then
      emit_deny "Mote nodes are append-mostly; raw deletes break the edge index. Use \`mote delete <id>\` to soft-delete into .memory/trash/, or \`mote trash restore <id>\` if you change your mind."
    fi
    ;;
esac

# Case 2: writes (>, >>, or tee) to derived index files.

# .memory/index.jsonl
if [[ "$CMD" =~ \>[[:space:]]*\.memory/index\.jsonl ]] \
   || [[ "$CMD" =~ tee[[:space:]]+[^|\;]*\.memory/index\.jsonl ]]; then
  emit_deny ".memory/index.jsonl is derived from .memory/nodes/. Run \`mote index rebuild\` to regenerate it instead of editing it directly."
fi

# .memory/mote_bm25.json
if [[ "$CMD" =~ \>[[:space:]]*\.memory/mote_bm25\.json ]] \
   || [[ "$CMD" =~ tee[[:space:]]+[^|\;]*\.memory/mote_bm25\.json ]]; then
  emit_deny ".memory/mote_bm25.json is a derived BM25 index. Run \`mote index rebuild\` (or let \`mote dream\` regenerate it) instead of editing it directly."
fi

# .memory/strata/<corpus>/bm25.json
if [[ "$CMD" =~ \>[[:space:]]*\.memory/strata/[^[:space:]/]+/bm25\.json ]] \
   || [[ "$CMD" =~ tee[[:space:]]+[^|\;]*\.memory/strata/[^[:space:]/]+/bm25\.json ]]; then
  emit_deny "Strata BM25 indexes under .memory/strata/<corpus>/ are derived. Rebuild them through mote rather than editing the JSON directly."
fi

exit 0
