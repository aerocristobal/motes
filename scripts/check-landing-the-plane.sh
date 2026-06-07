#!/usr/bin/env bash
# scripts/check-landing-the-plane.sh — verify that CLAUDE.md's
# `## Landing the Plane (Session Completion)` section matches the eight-step
# shape codified by STORY-LAND-001 (anti-patterns callout, follow-up prompt
# blockquote, copy-paste bash block, force-push prohibition).
#
# Exit codes:
#   0  every assertion holds
#   1  one or more assertions failed (message on stderr)
#
# Usage:
#   scripts/check-landing-the-plane.sh [repo_root]
#
# repo_root defaults to the directory above the script. Wired into CI alongside
# scripts/check-versions.sh (.github/workflows/ci.yml `landing-the-plane` job).

set -euo pipefail

repo_root="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
CLAUDE_MD="${repo_root}/CLAUDE.md"

if [ ! -f "$CLAUDE_MD" ]; then
    echo "check-landing-the-plane: missing $CLAUDE_MD" >&2
    exit 1
fi

# Extract the Landing the Plane section into a tmp file for awk-friendly tests.
# Range: from `## Landing the Plane` to the next H2 not starting with `L`.
SECTION="$(mktemp)"
trap 'rm -f "$SECTION"' EXIT
awk '/^## Landing the Plane/,/^## [^L]/' "$CLAUDE_MD" > "$SECTION"

if [ ! -s "$SECTION" ]; then
    echo "check-landing-the-plane: '## Landing the Plane' section not found in $CLAUDE_MD" >&2
    exit 1
fi

# --- SCENARIO 1: eight numbered steps in order ---

test_section_has_eight_numbered_steps() {
    local n
    n="$(grep -cE "^[0-9]+\. " "$SECTION")"
    [ "$n" -eq 8 ] || { echo "FAIL: expected 8 steps, found $n" >&2; exit 1; }
}

test_steps_6_and_7_are_new_commands() {
    grep -qF "git stash clear" "$SECTION" \
        || { echo "FAIL: step 6 missing 'git stash clear'" >&2; exit 1; }
    grep -qF "git remote prune origin" "$SECTION" \
        || { echo "FAIL: step 7 missing 'git remote prune origin'" >&2; exit 1; }
}

# --- SCENARIO 2: anti-patterns callout ---

test_anti_patterns_callout_exists() {
    grep -qF "### ⚠ Anti-patterns" "$SECTION" \
        || { echo "FAIL: anti-patterns callout heading missing" >&2; exit 1; }
    # Body between the ⚠ Anti-patterns heading and the next H3 (exclusive).
    # A naked awk range would terminate on the start line itself (it matches
    # both bounds), so use a flag-driven extractor.
    local n
    n="$(awk '/^### ⚠ Anti-patterns/{flag=1; next} flag && /^### /{flag=0} flag' "$SECTION" | grep -cE "^- ")"
    [ "$n" -ge 5 ] || { echo "FAIL: anti-patterns expected >= 5 bullets, found $n" >&2; exit 1; }
}

test_anti_patterns_uses_required_tagline() {
    # The literal lowercase phrase must appear inside the callout body (per
    # STORY-LAND-001 Scenario 2 "as its tagline"), not only as a capitalized
    # sentence-start in the section lead-in.
    local callout_body
    callout_body="$(awk '/^### ⚠ Anti-patterns/{flag=1; next} flag && /^### /{flag=0} flag' "$SECTION")"
    printf '%s\n' "$callout_body" | grep -qF "the plane has not landed until \`git push\` succeeds" \
        || { echo "FAIL: literal tagline missing from the anti-patterns callout body" >&2; exit 1; }
}

# --- SCENARIO 3: example session bash block ---

test_example_session_block_is_bash_fenced() {
    grep -qE '^```bash' "$SECTION" \
        || { echo "FAIL: no fenced bash block" >&2; exit 1; }
}

test_example_session_lists_all_eight_commands_in_order() {
    # Scope the ordering check to the "### Example session (copy verbatim)"
    # subsection — otherwise `git push` in the lead-in tagline shadows the
    # ordered occurrence inside the bash block.
    local block
    block="$(awk '/^### Example session/{flag=1; next} flag && /^### /{flag=0} flag' "$SECTION")"
    [ -n "$block" ] || { echo "FAIL: '### Example session' subsection missing" >&2; exit 1; }
    local commands=("git pull --rebase" "git push" "git status" \
                    "git stash clear" "git remote prune origin" "echo")
    local prev_line=0
    local cmd line
    for cmd in "${commands[@]}"; do
        line="$(printf '%s\n' "$block" | grep -nF "$cmd" | head -1 | cut -d: -f1)"
        [ -n "$line" ] || { echo "FAIL: command '$cmd' missing from example session block" >&2; exit 1; }
        [ "$line" -ge "$prev_line" ] \
            || { echo "FAIL: command '$cmd' out of order (line $line < $prev_line) inside example session" >&2; exit 1; }
        prev_line="$line"
    done
}

# --- SCENARIO 4: follow-up prompt template ---

test_followup_template_is_blockquote() {
    grep -qE "^> Continue work on mote-<id>" "$SECTION" \
        || { echo "FAIL: follow-up template missing or not a > blockquote" >&2; exit 1; }
}

test_followup_template_has_filled_example() {
    grep -qE "^> Continue work on mote-T[0-9a-zA-Z]+" "$SECTION" \
        || { echo "FAIL: no filled-in example below template" >&2; exit 1; }
}

# --- SCENARIO 5: verify step has 4 sub-bullets ---

test_verify_step_has_four_subcommands() {
    local n
    n="$(awk '/^5\. /,/^6\. /' "$SECTION" | grep -cE "^   - ")"
    [ "$n" -eq 4 ] || { echo "FAIL: verify step expected 4 sub-bullets, found $n" >&2; exit 1; }
    local c
    for c in "git status" "git log @{u}..HEAD" "git stash list" "git diff --stat HEAD~1"; do
        awk '/^5\. /,/^6\. /' "$SECTION" | grep -qF "$c" \
            || { echo "FAIL: verify sub-bullet '$c' missing" >&2; exit 1; }
    done
}

# --- SCENARIO 6: force-push explicitly forbidden ---

test_force_push_is_explicitly_forbidden() {
    grep -qF "git push --force" "$SECTION" \
        || { echo "FAIL: 'git push --force' not mentioned" >&2; exit 1; }
    grep -qF "is not landing, it is crashing" "$SECTION" \
        || { echo "FAIL: force-push anti-pattern phrasing missing" >&2; exit 1; }
}

test_section_has_eight_numbered_steps
test_steps_6_and_7_are_new_commands
test_anti_patterns_callout_exists
test_anti_patterns_uses_required_tagline
test_example_session_block_is_bash_fenced
test_example_session_lists_all_eight_commands_in_order
test_followup_template_is_blockquote
test_followup_template_has_filled_example
test_verify_step_has_four_subcommands
test_force_push_is_explicitly_forbidden

echo "check-landing-the-plane: ok"
exit 0
