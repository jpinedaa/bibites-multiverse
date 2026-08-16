# The tested-build record, as a sourceable library.
#
# WHAT THE RECORD IS. docs/support-matrix.md carries a top-level "testedBuild"
# object beside "release" and "published": the mod version, the sha256 of the
# BibitesMultiverse.dll that was tested, the bibites-mod/ tree it was built
# from, the commit cmd/sidecar's tested source came from, the digest of that
# source's input manifest, the date, and one sentence of evidence. It is the
# release reference — the thing a release build compares against — and it
# replaced a tracked 6.8 MB zip whose only job was to hold the same two facts
# as bytes.
#
# ONE PARSE, TWO READERS. release/make-release.sh turns it into gates 2, 3 and
# E; release/check-drift.sh answers the same questions from tracked text alone
# on every pull request. A reader that is laxer than the other can make the two
# disagree about what the record says, which is the exact failure class the
# gates exist to prevent. So the parse lives here once.
#
# It needs python3 and awk. It reads; it writes nothing but the extracted JSON
# the caller names.
#
# Source it, do not run it:
#
#   . "$REPO/release/lib/tested-build.sh"
#   matrix_json_block "$REPO/docs/support-matrix.md" "$work/support-matrix.json"
#   sha="$(tested_build_field "$work/support-matrix.json" pluginSha256)"

# matrix_json_block <support-matrix.md> <output path> — the machine-readable
# block, extracted and proved to be JSON. This is the same extraction the
# release build writes into every archive as support-matrix.json.
matrix_json_block() { # $1 docs/support-matrix.md, $2 where to write the JSON
  local doc="$1" out="$2"
  [ -f "$doc" ] || { printf 'missing %s\n' "$doc" >&2; return 1; }
  awk '/SUPPORT-MATRIX-JSON-BEGIN/{f=1;next} /SUPPORT-MATRIX-JSON-END/{f=0} f' "$doc" \
    | sed '/^```/d' > "$out"
  [ -s "$out" ] || { printf 'no JSON block between the markers in %s\n' "$doc" >&2; return 1; }
  python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$out" >/dev/null 2>&1 \
    || { printf 'the matrix block in %s is not valid JSON\n' "$doc" >&2; return 1; }
}

# tested_build_field <matrix json> <field> — the field's value on stdout.
#
# THE WHOLE RECORD IS VALIDATED ON EVERY READ, not only the field asked for, so
# a half-written record cannot pass one gate and fail another with a different
# complaint. Every key must be present, no key may be unknown, and each value
# must have the shape its meaning requires — a 40-hex tree hash that is 39
# characters long is a typo, and a typo in this record is a gate comparing
# against nothing. The cost is one python3 per read; the gain is that "the
# record is well formed" has exactly one definition.
tested_build_field() { # $1 the extracted matrix JSON, $2 field name
  python3 - "$1" "$2" <<'PY'
import json
import re
import sys

REQUIRED = {
    "mod":                 r"[0-9]+(\.[0-9]+)*",
    "pluginSha256":        r"[0-9a-f]{64}",
    "bibitesModTree":      r"[0-9a-f]{40}",
    "sidecarSourceCommit": r"[0-9a-f]{40}",
    "sidecarInputsSha256": r"[0-9a-f]{64}",
    "testedOn":            r"[0-9]{4}-[0-9]{2}-[0-9]{2}",
    "evidence":            r".*\S.*",
}

doc = json.load(open(sys.argv[1]))
field = sys.argv[2]
record = doc.get("testedBuild")
if not isinstance(record, dict):
    raise SystemExit(
        'the matrix carries no top-level "testedBuild" object. It is the release '
        'reference; docs/support-matrix.md, the section "The tested build", says '
        "what it holds and how it is recorded.")
missing = sorted(set(REQUIRED) - set(record))
extra = sorted(set(record) - set(REQUIRED))
if missing or extra:
    raise SystemExit("testedBuild: missing %s, unexpected %s" % (missing, extra))
for key, shape in sorted(REQUIRED.items()):
    value = record[key]
    if not isinstance(value, str) or not re.fullmatch(shape, value):
        raise SystemExit("testedBuild.%s is %r, want /%s/" % (key, value, shape))
if field not in REQUIRED:
    raise SystemExit("no such testedBuild field: %s" % field)
print(record[field])
PY
}
