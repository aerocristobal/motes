#!/usr/bin/env bash
# block-interactive-cmds.sh — Deny rm/cp/mv invocations that may hang an
# unattended agent on a macOS shell-alias prompt (rm -i, cp -i, mv -i).
#
# Adapted from beads §1 (gastownhall/beads) per docs/beads-recommendations.md.
# Installed by `mote onboard` into ~/.claude/hooks/ and wired into
# ~/.claude/settings.json under hooks.PreToolUse[].matcher: "Bash".
#
# Allowed bypasses:
#   - flag bundle containing f: -f, -rf, -fr, -fv, ...
#   - long flag --force
#   - absolute path:  /bin/rm, /usr/bin/cp, ...
#   - command prefix: `command rm somefile`

set -euo pipefail

# Fail-open if jq is missing — never break a session because the hook
# itself can't run.
if ! command -v jq >/dev/null 2>&1; then
  echo "warning: block-interactive-cmds.sh: jq not found, allowing command through" >&2
  exit 0
fi

CMD=$(jq -r '.tool_input.command // empty')
[ -z "$CMD" ] && exit 0

# First whitespace-delimited token — the program being invoked.
first_word=$(awk '{print $1}' <<<"$CMD")

case "$first_word" in
  rm|cp|mv)
    padded=" $CMD "
    # Any short flag bundle containing 'f'.
    if [[ "$padded" =~ [[:space:]]-[a-zA-Z]*f[a-zA-Z]*[[:space:]] ]]; then
      exit 0
    fi
    # --force long flag.
    if [[ "$padded" =~ [[:space:]]--force[[:space:]] ]]; then
      exit 0
    fi
    reason="\`$first_word\` may be shell-aliased to \`$first_word -i\` (common on macOS), which silently hangs an agent waiting for a y/n prompt. Add \`-f\` (or \`--force\`), use an absolute path (e.g. \`/bin/$first_word\`), or prefix with \`command \` to bypass the alias."
    jq -nc --arg reason "$reason" \
      '{hookSpecificOutput:{permissionDecision:"deny",permissionDecisionReason:$reason}}'
    exit 0
    ;;
esac

# Everything else (incl. `command rm ...`, `/bin/rm ...`, unrelated cmds) passes.
exit 0
