#!/usr/bin/env bash
# scripts/bump-version.sh — atomically update every tracked version-bearing
# location to a new semantic version. Backs STORY-VERSIONS-001.
#
# Exit codes:
#   0  every tracked location rewritten to the new version
#   1  partial-rewrite failure detected; backups restored
#   2  validation error (bad semver, downgrade without override, missing files)
#
# Usage:
#   scripts/bump-version.sh <X.Y.Z> [--commit] [--allow-downgrade] [--root <dir>]
#
# Behavior:
#   - Validates <X.Y.Z> as semver 2.0.0 (with optional pre-release and
#     build-metadata suffixes).
#   - Refuses no-op (current == new) and downgrades (new < current) unless
#     --allow-downgrade is passed.
#   - Rewrites internal/version/version.go's `const Value` and inserts a
#     new placeholder bullet at the head of docs/version-history.md's
#     `## Version History` section.
#   - With --commit, stages the two modified files and creates a single
#     commit. NEVER tags, NEVER pushes, NEVER opens a pull request.

set -euo pipefail

version=""
do_commit=0
allow_downgrade=0
repo_root=""

usage() {
    cat >&2 <<'EOF'
Usage: scripts/bump-version.sh <X.Y.Z> [--commit] [--allow-downgrade] [--root <dir>]
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --commit) do_commit=1; shift;;
        --allow-downgrade) allow_downgrade=1; shift;;
        --root)
            if [ $# -lt 2 ]; then
                echo "bump-version: --root requires an argument" >&2
                exit 2
            fi
            repo_root="$2"; shift 2;;
        -h|--help) usage; exit 0;;
        --) shift;;
        -*)
            echo "bump-version: unknown flag $1" >&2
            usage
            exit 2;;
        *)
            if [ -z "$version" ]; then
                version="$1"
            else
                echo "bump-version: unexpected positional argument '$1'" >&2
                usage
                exit 2
            fi
            shift;;
    esac
done

if [ -z "$version" ]; then
    echo "bump-version: missing <X.Y.Z> argument" >&2
    usage
    exit 2
fi

# Semver 2.0.0 with optional pre-release and build-metadata.
semver_re='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
if ! [[ "$version" =~ $semver_re ]]; then
    echo "bump-version: '${version}' is not valid semver (must match MAJOR.MINOR.PATCH with optional pre-release/build suffix)" >&2
    exit 2
fi

if [ -z "$repo_root" ]; then
    repo_root="$(cd "$(dirname "$0")/.." && pwd)"
fi

canonical_file="${repo_root}/internal/version/version.go"
vhistory_file="${repo_root}/docs/version-history.md"

if [ ! -f "$canonical_file" ]; then
    echo "bump-version: missing $canonical_file" >&2
    exit 2
fi
if [ ! -f "$vhistory_file" ]; then
    echo "bump-version: missing $vhistory_file" >&2
    exit 2
fi

current=$(grep -E 'const[[:space:]]+Value[[:space:]]*=[[:space:]]*"[^"]+"' "$canonical_file" \
    | head -n1 \
    | sed -E 's/.*const[[:space:]]+Value[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/')
if [ -z "$current" ]; then
    echo "bump-version: could not parse current canonical from $canonical_file" >&2
    exit 2
fi

if [ "$current" = "$version" ]; then
    echo "bump-version: canonical is already at ${version}" >&2
    exit 2
fi

# Use `sort -V` to detect downgrade. Note: `sort -V` orders pre-releases
# AFTER the release in some implementations (i.e. 1.0.0-rc.1 > 1.0.0),
# which is the OPPOSITE of semver precedence. For mote's actual usage
# (numeric forward bumps), this is fine; if mote starts shipping
# pre-releases regularly, replace this with a real semver comparator.
lower=$(printf '%s\n%s\n' "$current" "$version" | sort -V | head -n1)
if [ "$lower" = "$version" ] && [ "$allow_downgrade" -ne 1 ]; then
    echo "bump-version: ${version} is a downgrade from ${current}; pass --allow-downgrade to override" >&2
    exit 2
fi

declare -a backed_up=()
rollback() {
    local rc=$?
    for f in "${backed_up[@]}"; do
        if [ -f "${f}.bumpbak" ]; then
            mv -f "${f}.bumpbak" "$f"
        fi
    done
    exit "$rc"
}
trap rollback ERR

backup() {
    cp "$1" "${1}.bumpbak"
    backed_up+=("$1")
}

# Rewrite canonical.
backup "$canonical_file"
tmp=$(mktemp)
sed -E "s|(const[[:space:]]+Value[[:space:]]*=[[:space:]]*\")${current}(\")|\1${version}\2|" \
    "$canonical_file" > "$tmp"
mv "$tmp" "$canonical_file"
if ! grep -qE "const[[:space:]]+Value[[:space:]]*=[[:space:]]*\"${version}\"" "$canonical_file"; then
    echo "bump-version: failed to rewrite ${canonical_file}" >&2
    false
fi

# Insert new bullet at head of `## Version History` section.
backup "$vhistory_file"
tmp=$(mktemp)
awk -v ver="$version" '
BEGIN { inserted = 0; in_section = 0 }
/^##[[:space:]]+Version History[[:space:]]*$/ {
    print
    in_section = 1
    next
}
in_section == 1 && /^[[:space:]]*$/ && inserted == 0 {
    print
    print "- **v" ver "** — (placeholder; edit before release)"
    inserted = 1
    in_section = 0
    next
}
in_section == 1 && /^- / && inserted == 0 {
    # No blank line between heading and first bullet — insert here.
    print "- **v" ver "** — (placeholder; edit before release)"
    print ""
    print $0
    inserted = 1
    in_section = 0
    next
}
{ print }
END {
    if (!inserted) {
        print "- **v" ver "** — (placeholder; edit before release)"
    }
}
' "$vhistory_file" > "$tmp"
mv "$tmp" "$vhistory_file"

escaped_version="${version//./\\.}"
escaped_version="${escaped_version//+/\\+}"
if ! grep -qE "^- \*\*v${escaped_version}\*\*" "$vhistory_file"; then
    echo "bump-version: failed to insert v${version} bullet into ${vhistory_file}" >&2
    false
fi

# Rewrite version field in vendored plugin manifests (STORY-PLUGINS-001).
# Files are tolerated as optional — test setups that don't create them are
# unaffected; production repos always have them present. Each manifest carries
# exactly one `"version": "X.Y.Z"` field; replace in place.
escaped_current="${current//./\\.}"
escaped_current="${escaped_current//+/\\+}"
plugin_manifests=(
    "plugins/mote/.claude-plugin/plugin.json"
    "plugins/mote/.codex-plugin/plugin.json"
    "plugins/mote/.claude-plugin/marketplace.json"
)
rewritten_manifests=()
for rel in "${plugin_manifests[@]}"; do
    f="${repo_root}/${rel}"
    if [ ! -f "$f" ]; then
        continue
    fi
    backup "$f"
    tmp=$(mktemp)
    sed -E "s|(\"version\"[[:space:]]*:[[:space:]]*\")${escaped_current}(\")|\1${version}\2|g" \
        "$f" > "$tmp"
    mv "$tmp" "$f"
    if ! grep -qE "\"version\"[[:space:]]*:[[:space:]]*\"${escaped_version}\"" "$f"; then
        echo "bump-version: failed to rewrite version field in ${rel}" >&2
        false
    fi
    rewritten_manifests+=("${rel}")
done

for f in "${backed_up[@]}"; do
    rm -f "${f}.bumpbak"
done
trap - ERR

if [ "$do_commit" -eq 1 ]; then
    git_files=("internal/version/version.go" "docs/version-history.md")
    for rel in "${rewritten_manifests[@]}"; do
        git_files+=("${rel}")
    done
    git -C "$repo_root" add "${git_files[@]}"
    git -C "$repo_root" commit -q -m "chore(version): bump to ${version}"
fi

echo "bump-version: ${current} -> ${version}"
exit 0
