#!/usr/bin/env bash
# scripts/test_test.sh — contract tests for scripts/test.sh. Covers all
# 11 BDD scenarios in STORY-TWRAP-001.
#
# Strategy: stub the `go` binary on $PATH so we can assert the exact
# argv the wrapper passes through without paying real Go compile time.
# The stub appends every invocation's argv to $SANDBOX/last_go_args
# (one line per call) and, when invoked as `go test -list ...`, emits
# the fixture test names from $FIXTURE_TESTS so the wrapper's stale-
# entry detection has meaningful input.
#
# Run: bash scripts/test_test.sh
# Exit 0 on success, non-zero on first failing assertion.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT_UNDER_TEST="${SCRIPT_DIR}/test.sh"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [ ! -x "$SCRIPT_UNDER_TEST" ]; then
    echo "test_test.sh: ${SCRIPT_UNDER_TEST} is not executable" >&2
    exit 2
fi

# Each test gets a fresh sandbox so state cannot leak.
new_sandbox() {
    SANDBOX="$(mktemp -d)"
    export SANDBOX
    # Reset shared state.
    unset FIXTURE_TESTS TEST_VERBOSE TEST_RUN TEST_TIMEOUT
}

cleanup_sandbox() {
    if [ -n "${SANDBOX:-}" ] && [ -d "$SANDBOX" ]; then
        rm -rf "$SANDBOX"
    fi
}

trap cleanup_sandbox EXIT

# install_go_test_stub puts a fake `go` binary at the front of PATH.
# It records each invocation's argv (one line per call) to
# $SANDBOX/last_go_args. On `go test -list ...` it echoes the
# space-separated $FIXTURE_TESTS one name per line.
install_go_test_stub() {
    mkdir -p "$SANDBOX/bin"
    cat > "$SANDBOX/bin/go" <<'EOF'
#!/usr/bin/env bash
echo "go $*" >> "$SANDBOX/last_go_args"
# Emit a fixture listing when invoked as `go test -list <regex> <pkgs>`
if [ "${1:-}" = "test" ] && [ "${2:-}" = "-list" ]; then
    if [ -n "${FIXTURE_TESTS:-}" ]; then
        for n in $FIXTURE_TESTS; do
            echo "$n"
        done
    fi
fi
exit 0
EOF
    chmod +x "$SANDBOX/bin/go"
    export PATH="$SANDBOX/bin:$PATH"
}

# fail prints a message and exits non-zero.
fail() {
    echo "FAIL: $*" >&2
    if [ -f "$SANDBOX/last_go_args" ]; then
        echo "--- last_go_args ---" >&2
        cat "$SANDBOX/last_go_args" >&2
        echo "--------------------" >&2
    fi
    exit 1
}

# --- HAPPY PATH (Scenario 1) -----------------------------------------------

test_no_skip_file_no_env_vars_pass_through() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne"
    cd "$SANDBOX"
    bash "$SCRIPT_UNDER_TEST" >/dev/null 2>&1
    grep -qF "go test" "$SANDBOX/last_go_args" \
        || fail "did not invoke go test"
    grep -qF "./..." "$SANDBOX/last_go_args" \
        || fail "did not include ./..."
    if grep -qF -- "-skip" "$SANDBOX/last_go_args"; then
        fail "should not have -skip when .test-skip is absent"
    fi
    grep -qF -- "-timeout=5m" "$SANDBOX/last_go_args" \
        || fail "did not pass default timeout"
}

# --- ALTERNATIVE (Scenario 2) ----------------------------------------------

test_one_skip_entry_produces_anchored_regex() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne TestFlaky"
    cd "$SANDBOX"
    echo "TestFlaky  # mote-T1abc — flaky on macOS" > .test-skip
    bash "$SCRIPT_UNDER_TEST" >/dev/null 2>&1
    grep -qF -- "-skip ^TestFlaky\$" "$SANDBOX/last_go_args" \
        || fail "anchored regex wrong for one entry"
}

# --- ALTERNATIVE (Scenario 3) ----------------------------------------------

test_two_skip_entries_combine_into_alternation() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne TestA TestB"
    cd "$SANDBOX"
    cat > .test-skip <<'EOF'
TestA  # mote-T1aaa
TestB  # mote-T1bbb
EOF
    bash "$SCRIPT_UNDER_TEST" >/dev/null 2> "$SANDBOX/stderr.txt"
    grep -qF -- "-skip ^(TestA|TestB)\$" "$SANDBOX/last_go_args" \
        || fail "combined regex wrong"
    grep -qF "skipping 2 tests per .test-skip: TestA, TestB" "$SANDBOX/stderr.txt" \
        || fail "summary missing or wrong: $(cat "$SANDBOX/stderr.txt")"
}

# --- BOUNDARY (Scenario 4) -------------------------------------------------

test_skip_file_tolerates_comments_and_blanks() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestA TestB TestC"
    cd "$SANDBOX"
    cat > .test-skip <<'EOF'
# header comment
TestA  # mote-T1aaa

    TestB# mote-T1bbb (no space before comment)
# in the middle
TestC#mote-T1ccc
EOF
    bash "$SCRIPT_UNDER_TEST" >/dev/null 2>&1
    local n
    for n in TestA TestB TestC; do
        grep -qF "$n" "$SANDBOX/last_go_args" \
            || fail "$n missing from -skip regex"
    done
}

# --- ALTERNATIVE (Scenarios 5-7) -------------------------------------------

test_TEST_VERBOSE_maps_to_v() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne"
    cd "$SANDBOX"
    TEST_VERBOSE=1 bash "$SCRIPT_UNDER_TEST" >/dev/null 2>&1
    grep -qF -- " -v " "$SANDBOX/last_go_args" \
        || grep -qE -- " -v$" "$SANDBOX/last_go_args" \
        || fail "TEST_VERBOSE=1 did not add -v"
}

test_TEST_RUN_maps_to_run() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne TestTwo"
    cd "$SANDBOX"
    TEST_RUN=TestTwo bash "$SCRIPT_UNDER_TEST" >/dev/null 2>&1
    grep -qF -- "-run TestTwo" "$SANDBOX/last_go_args" \
        || fail "TEST_RUN not mapped"
}

test_TEST_TIMEOUT_overrides_default() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne"
    cd "$SANDBOX"
    TEST_TIMEOUT=30s bash "$SCRIPT_UNDER_TEST" >/dev/null 2>&1
    grep -qF -- "-timeout=30s" "$SANDBOX/last_go_args" \
        || fail "TEST_TIMEOUT override not applied"
    if grep -qF -- "-timeout=5m" "$SANDBOX/last_go_args"; then
        fail "default timeout leaked alongside override"
    fi
}

# --- ERROR PATHS (Scenarios 8, 9) -----------------------------------------

test_stale_skip_entry_warns_but_does_not_fail() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne TestTwo"   # Ghost is NOT in this list
    cd "$SANDBOX"
    echo "TestGhostThatNeverExisted  # mote-T0xyz — DELETE ME" > .test-skip
    set +e
    bash "$SCRIPT_UNDER_TEST" 2> "$SANDBOX/stderr.txt" >/dev/null
    rc=$?
    set -e
    [ "$rc" -eq 0 ] || fail "stale entry should not change exit code, got $rc"
    grep -qF "matched 0 tests" "$SANDBOX/stderr.txt" \
        || fail "WARNING missing for stale entry: $(cat "$SANDBOX/stderr.txt")"
    grep -qF "TestGhostThatNeverExisted" "$SANDBOX/stderr.txt" \
        || fail "WARNING did not name the stale entry"
}

test_skip_entry_without_rationale_exits_2() {
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne"
    cd "$SANDBOX"
    echo "TestOne" > .test-skip       # NO `# mote-<id>` comment
    set +e
    bash "$SCRIPT_UNDER_TEST" 2> "$SANDBOX/stderr.txt" >/dev/null
    rc=$?
    set -e
    [ "$rc" -eq 2 ] || fail "expected exit 2 for missing rationale, got $rc"
    grep -qF "missing rationale" "$SANDBOX/stderr.txt" \
        || fail "error message missing 'missing rationale': $(cat "$SANDBOX/stderr.txt")"
    if [ -f "$SANDBOX/last_go_args" ]; then
        fail "go test was invoked despite parse error"
    fi
}

test_skip_entry_with_wrong_rationale_exits_2() {
    # Defense in depth: a `# foo` comment that does NOT name a mote
    # should still be rejected.
    new_sandbox
    install_go_test_stub
    export FIXTURE_TESTS="TestOne"
    cd "$SANDBOX"
    echo "TestOne  # see github issue 42" > .test-skip
    set +e
    bash "$SCRIPT_UNDER_TEST" 2> "$SANDBOX/stderr.txt" >/dev/null
    rc=$?
    set -e
    [ "$rc" -eq 2 ] || fail "expected exit 2 for non-mote rationale, got $rc"
    grep -qF "missing rationale" "$SANDBOX/stderr.txt" \
        || fail "error message missing 'missing rationale'"
}

# --- ALTERNATIVE (Scenario 10) --------------------------------------------

test_CLAUDE_md_references_the_wrapper_not_bare_go_test() {
    local doc="${REPO_ROOT}/CLAUDE.md"
    [ -f "$doc" ] || fail "CLAUDE.md not found at $doc"
    grep -qF "bash scripts/test.sh" "$doc" \
        || fail "CLAUDE.md does not reference wrapper"
    if awk '/^## Build & Development Commands/,/^## [^B]/' "$doc" \
        | grep -qE "^go test \./\.\.\."; then
        fail "CLAUDE.md still has bare 'go test ./...' in Build & Development Commands"
    fi
}

test_AGENTS_md_references_the_wrapper_not_bare_go_test() {
    local doc="${REPO_ROOT}/AGENTS.md"
    [ -f "$doc" ] || fail "AGENTS.md not found at $doc"
    grep -qF "## Build & Development Commands" "$doc" \
        || fail "AGENTS.md is missing the ## Build & Development Commands section"
    grep -qF "bash scripts/test.sh" "$doc" \
        || fail "AGENTS.md does not reference wrapper"
    if awk '/^## Build & Development Commands/,/^## [^B]/' "$doc" \
        | grep -qE "^go test \./\.\.\."; then
        fail "AGENTS.md still has bare 'go test ./...' in Build & Development Commands"
    fi
}

# --- BOUNDARY (Scenario 11) -----------------------------------------------

test_Makefile_delegates_to_wrapper() {
    local mk="${REPO_ROOT}/Makefile"
    [ -f "$mk" ] || fail "Makefile not found at $mk"
    # Extract the test: target body (lines after `test:` until the next
    # non-indented line). A simple range pattern doesn't work because
    # `test:` matches both /^test:/ AND /^[a-zA-Z]/ on the same line.
    local body
    body=$(awk '/^test:/{f=1;next} /^[a-zA-Z]/{f=0} f' "$mk")
    grep -qF "bash scripts/test.sh" <<<"$body" \
        || fail "Makefile test target does not delegate to wrapper (body: $body)"
    if grep -qE "^[[:space:]]+go test" <<<"$body"; then
        fail "Makefile still has bare 'go test' in test target"
    fi
}

# --- Driver ----------------------------------------------------------------

run() {
    local name="$1"
    "$name"
    echo "  ok ${name}"
}

run test_no_skip_file_no_env_vars_pass_through
run test_one_skip_entry_produces_anchored_regex
run test_two_skip_entries_combine_into_alternation
run test_skip_file_tolerates_comments_and_blanks
run test_TEST_VERBOSE_maps_to_v
run test_TEST_RUN_maps_to_run
run test_TEST_TIMEOUT_overrides_default
run test_stale_skip_entry_warns_but_does_not_fail
run test_skip_entry_without_rationale_exits_2
run test_skip_entry_with_wrong_rationale_exits_2
run test_CLAUDE_md_references_the_wrapper_not_bare_go_test
run test_AGENTS_md_references_the_wrapper_not_bare_go_test
run test_Makefile_delegates_to_wrapper

echo "OK"
