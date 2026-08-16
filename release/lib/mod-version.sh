# The mod's declared version, read from bibites-mod/src/MultiversePlugin.cs.
#
# ONE PARSE, THREE READERS. `MultiversePlugin.cs` declares the version that
# BepInEx registers at runtime; docs/support-matrix.md's `mod` field and
# dev_environment.md's Plugin row are claims about it, and
# farend/dist/BUNDLE-SOURCE.txt records it. release/make-release.sh's gate E,
# release/check-drift.sh's check B and check D, and
# farend/make-farend-bundle.sh's record all have to agree on what the
# declaration says — a reader that is laxer than the others can make the gates
# and the record disagree about the version, which is the exact failure class
# those gates exist to prevent. So the pattern lives here once.
#
# It matches the declaration as written, `public const string Version = "x"`,
# and takes the first one in the file.
#
# Source it, do not run it:
#
#   . "$REPO/release/lib/mod-version.sh"
#   version="$(mod_version "$REPO/bibites-mod/src/MultiversePlugin.cs")"
#
# Prints the version and returns 0, or explains on stderr and returns 1.

mod_version() { # $1 path to MultiversePlugin.cs
  local source="$1" version
  [ -f "$source" ] || { printf 'missing %s\n' "$source" >&2; return 1; }
  version="$(sed -n \
    's/.*public const string Version[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' \
    "$source" | head -n 1)"
  [ -n "$version" ] \
    || { printf "no 'public const string Version' declaration in %s\n" "$source" >&2; return 1; }
  printf '%s\n' "$version"
}
