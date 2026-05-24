#!/usr/bin/env bash
# block-gh-watch.sh — Deny `gh run watch`, whose 3-second polling has burned
# entire teams through the 5000/hr GitHub API quota during releases.
#
# Adapted from beads §1 (gastownhall/beads) per docs/beads-recommendations.md.
# Installed by `mote onboard` into ~/.claude/hooks/.

set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "warning: block-gh-watch.sh: jq not found, allowing command through" >&2
  exit 0
fi

CMD=$(jq -r '.tool_input.command // empty')
[ -z "$CMD" ] && exit 0

# Match `gh run watch` with any args/flags. Leading whitespace allowed.
# Other `gh` subcommands (run view, run list, pr create, ...) pass through.
if [[ "$CMD" =~ ^[[:space:]]*gh[[:space:]]+run[[:space:]]+watch([[:space:]]|$) ]]; then
  reason="\`gh run watch\` polls the GitHub API every 3 seconds and has historically exhausted the 5000/hr quota for entire teams during releases. Use \`gh run view --log <id>\` for a one-shot fetch, or \`gh run list\` to poll cheaply."
  jq -nc --arg reason "$reason" \
    '{hookSpecificOutput:{permissionDecision:"deny",permissionDecisionReason:$reason}}'
  exit 0
fi

exit 0
