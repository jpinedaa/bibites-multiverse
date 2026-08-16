# The cmd/sidecar input manifest, as a sourceable library.
#
# A Go binary carries a VCS stamp, so two builds from identical inputs at two
# commits differ as files. This function writes the thing that CAN be compared:
# every main-module source file that `go list -deps` reaches from cmd/sidecar,
# with its sha256, plus the identity of every external module that graph
# selects. Two checkouts whose manifests are equal build the same sidecar.
#
# It is the body of release/make-release.sh's gate 2, lifted out unchanged so
# that release/check-drift.sh runs the identical gate rather than a lookalike.
# Both sources this file; neither owns a second copy.
#
# It needs go and python3 on PATH, and a checkout that holds go/ as a directory
# tree (a `git archive` extraction is enough — no .git is required, because the
# listing passes -buildvcs=false).
#
# Source it, do not run it:
#
#   . "$REPO/release/lib/sidecar-manifest.sh"
#   sidecar_manifest "$checkout" "$manifest_path" "$golist_json_path"
#
# The manifest is a sorted text file; compare two of them with cmp or diff.

sidecar_manifest() { # $1 checkout root, $2 manifest path, $3 go-list JSON path
  local checkout="$1" manifest="$2" packages_json="$3"
  (
    cd "$checkout/go"
    env GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
      go list -buildvcs=false -deps -json ./cmd/sidecar
  ) > "$packages_json"
  python3 - "$checkout/go" "$packages_json" "$manifest" <<'PY'
import hashlib
import json
from pathlib import Path
import sys

module_root = Path(sys.argv[1]).resolve()
packages_text = Path(sys.argv[2]).read_text(encoding="utf-8")
manifest_path = Path(sys.argv[3])
decoder = json.JSONDecoder()
packages = []
offset = 0
while offset < len(packages_text):
    while offset < len(packages_text) and packages_text[offset].isspace():
        offset += 1
    if offset == len(packages_text):
        break
    package, offset = decoder.raw_decode(packages_text, offset)
    packages.append(package)

source_fields = (
    "GoFiles", "CgoFiles", "CFiles", "CXXFiles", "MFiles", "HFiles",
    "FFiles", "SFiles", "SwigFiles", "SwigCXXFiles", "SysoFiles", "EmbedFiles",
)
files = {}
modules = set()
main_module = None

def module_identity(module):
    replacement = module.get("Replace") or {}
    return (
        module.get("Path", ""), module.get("Version", ""), module.get("Sum", ""),
        module.get("GoVersion", ""), replacement.get("Path", ""),
        replacement.get("Version", ""), replacement.get("Sum", ""),
        replacement.get("GoVersion", ""),
    )

for package in packages:
    module = package.get("Module")
    if not module:
        continue
    if not module.get("Main"):
        modules.add(module_identity(module))
        continue
    identity = (module.get("Path", ""), module.get("GoVersion", ""))
    if main_module is None:
        main_module = identity
    elif main_module != identity:
        raise SystemExit("cmd/sidecar resolved more than one main-module identity")
    package_dir = Path(package["Dir"]).resolve()
    for field in source_fields:
        for name in package.get(field) or ():
            path = (package_dir / name).resolve()
            try:
                relative = path.relative_to(module_root)
            except ValueError:
                raise SystemExit(f"main-module input is outside {module_root}: {path}")
            files[relative.as_posix()] = hashlib.sha256(path.read_bytes()).hexdigest()

if main_module is None:
    raise SystemExit("cmd/sidecar did not resolve to the main module")

lines = ["main-module\t" + "\t".join(main_module)]
lines.extend("module\t" + "\t".join(identity) for identity in sorted(modules))
lines.extend(f"file\tgo/{path}\t{digest}" for path, digest in sorted(files.items()))
manifest_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
}
