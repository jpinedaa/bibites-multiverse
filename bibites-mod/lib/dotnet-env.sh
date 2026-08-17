# The .NET toolchain, resolved rather than assumed.
#
# ONE ANSWER, THREE CALLERS. bibites-mod/deploy.sh, bibites-mod/sync-game-refs.sh
# and farend/make-farend-bundle.sh all run `dotnet` on a developer's box. Each
# used to hardcode `DOTNET_ROOT="$HOME/.dotnet"` and put that directory first on
# PATH, which is one of the places an SDK can be and not the only one: this box
# now keeps it in the machine-wide /usr/local/dotnet, and a `~/.dotnet` still
# exists there holding nothing but the SDK's per-user sentinels and caches. A
# hardcoded root is therefore not merely unhelpful — an exported DOTNET_ROOT
# that names a directory with no `dotnet` under it is obeyed, so it can turn a
# working toolchain into "You must install .NET to run this application". The
# resolution lives here once so all three agree.
#
# WHERE IT LOOKS, IN ORDER:
#
#   1. An already-set DOTNET_ROOT — but only when it really holds a `dotnet`.
#      Somebody else's value wins over any local convention; a stale one is
#      dropped rather than obeyed.
#   2. The `dotnet` already on PATH, followed through symlinks. That is the
#      machine's own answer to the same question, and it costs nothing to ask.
#   3. The well-known installs: ~/.dotnet, /usr/local/dotnet, /usr/share/dotnet,
#      /usr/lib/dotnet. Each is tested for an executable `dotnet`, which is what
#      lets an SDK-less ~/.dotnet fall through to the real install.
#
# `dotnet tool install --global` writes to ~/.dotnet/tools whatever DOTNET_ROOT
# says, so that directory goes on PATH as well as $DOTNET_ROOT/tools. That is
# where `ilspycmd` lives for sync-game-refs.sh.
#
# release/make-release.sh resolves the same two compilers inline, because a
# release script that sources a file from the tree it is releasing is a release
# script with one more way to go wrong.
#
# Source it, do not run it:
#
#   . "$REPO/bibites-mod/lib/dotnet-env.sh"
#   dotnet_env || exit 1
#
# On success DOTNET_ROOT names a directory that holds a `dotnet`, and both it
# and the global-tool directories are on PATH. On failure it says in one line
# what it looked for, and returns 1.

DOTNET_PROBE_DIRS="$HOME/.dotnet /usr/local/dotnet /usr/share/dotnet /usr/lib/dotnet"

dotnet_env() {
  local candidate found

  if [ -n "${DOTNET_ROOT:-}" ] && [ ! -x "$DOTNET_ROOT/dotnet" ]; then
    unset DOTNET_ROOT
  fi

  if [ -z "${DOTNET_ROOT:-}" ]; then
    found="$(command -v dotnet 2>/dev/null || true)"
    if [ -n "$found" ]; then
      found="$(readlink -f "$found" 2>/dev/null || printf '%s' "$found")"
      DOTNET_ROOT="$(dirname "$found")"
    fi
  fi

  if [ -z "${DOTNET_ROOT:-}" ]; then
    for candidate in $DOTNET_PROBE_DIRS; do
      if [ -x "$candidate/dotnet" ]; then
        DOTNET_ROOT="$candidate"
        break
      fi
    done
  fi

  if [ -z "${DOTNET_ROOT:-}" ]; then
    printf 'no .NET SDK: set DOTNET_ROOT, or put dotnet on PATH, or install into one of %s\n' \
      "$DOTNET_PROBE_DIRS" >&2
    return 1
  fi

  export DOTNET_ROOT
  if [ "$DOTNET_ROOT" = "$HOME/.dotnet" ]; then
    export PATH="$DOTNET_ROOT:$DOTNET_ROOT/tools:$PATH"
  else
    export PATH="$DOTNET_ROOT:$DOTNET_ROOT/tools:$HOME/.dotnet/tools:$PATH"
  fi
}
