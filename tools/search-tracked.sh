#!/usr/bin/env bash
# Search repository text without opening ignored or untracked custody data.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: tools/search-tracked.sh PATTERN [PATHSPEC ...]

Search tracked, non-binary repository files. PATTERN is a git-grep regular
expression. Optional PATHSPEC arguments limit the search from the repository
root.

Examples:
  tools/search-tracked.sh 'RuntimeFile|RuntimeSha256'
  tools/search-tracked.sh 'peerId' -- cloud/aws deploy
EOF
}

if [ "$#" -eq 0 ]; then
  usage >&2
  exit 2
fi

case "$1" in
  -h|--help)
    usage
    exit 0
    ;;
esac

pattern=$1
shift

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "!! search-tracked.sh must run inside a Git worktree" >&2
  exit 2
}

if [ "$#" -eq 0 ]; then
  set -- .
elif [ "$1" = "--" ]; then
  shift
  [ "$#" -gt 0 ] || {
    echo "!! -- must be followed by at least one pathspec" >&2
    exit 2
  }
fi

# git grep reads the index and tracked working-tree files. It does not enumerate
# ignored or ordinary untracked paths, and it does not follow tracked symlinks.
exec git -C "$repo_root" grep --no-color -n -I -e "$pattern" -- "$@"
