#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cloud="$repo/cloud/aws"

grep -Fq 'AllowedValues: [slot-1]' "$cloud/broadcast-template.yaml" || {
  echo 'broadcast template does not constrain WorldId to slot-1' >&2
  exit 1
}
grep -Fq 'bibites-sidecar@slot-1.service' \
  "$cloud/runtime/bibites-broadcast-game.service" || {
  echo 'broadcast game unit does not use slot-1' >&2
  exit 1
}
grep -Fq 'bibites-set-timescale slot-1' \
  "$cloud/runtime/bibites-broadcast-timescale.service" || {
  echo 'broadcast timescale unit does not use slot-1' >&2
  exit 1
}

set +e
install_output="$(
  AWS_REGION=us-east-1 \
  DATA_VOLUME_ID=vol-0123456789abcdef0 \
  RELAY_PRIVATE_IP=10.0.0.5 \
  RELAY_DOMAIN=relay.example.net \
  STREAM_RTMP_ADDRESS=10.0.0.6:1935 \
  PUBLISH_PASSWORD_PARAMETER=/bibites/broadcast/password \
  WORLD_ID=slot-1 \
  "$cloud/runtime/install-broadcast-host" 2>&1
)"
install_status=$?
set -e
[ "$install_status" -ne 0 ] && grep -Fq 'process arguments' <<<"$install_output" || {
  echo 'broadcast installer did not fail at the password-transport gate' >&2
  exit 1
}

set +e
wrong_world_output="$(
  AWS_REGION=us-east-1 \
  DATA_VOLUME_ID=vol-0123456789abcdef0 \
  RELAY_PRIVATE_IP=10.0.0.5 \
  RELAY_DOMAIN=relay.example.net \
  STREAM_RTMP_ADDRESS=10.0.0.6:1935 \
  PUBLISH_PASSWORD_PARAMETER=/bibites/broadcast/password \
  WORLD_ID=world-a \
  "$cloud/runtime/install-broadcast-host" 2>&1
)"
wrong_world_status=$?
set -e
[ "$wrong_world_status" -ne 0 ] && grep -Fq 'WORLD_ID must be slot-1' \
  <<<"$wrong_world_output" || {
  echo 'broadcast installer accepted a world other than slot-1' >&2
  exit 1
}
grep -Fq 'BIBITES_BROADCAST_WORLD_ID must be slot-1' \
  "$cloud/deploy-broadcast.sh" || {
  echo 'broadcast deployment wrapper does not constrain the world to slot-1' >&2
  exit 1
}
grep -Fq '[ "${STAGING_SCOPE:-}" = complete ]' \
  "$cloud/deploy-broadcast.sh" || {
  echo 'broadcast deployment does not require complete staging' >&2
  exit 1
}
scope_line="$(grep -n 'STAGING_SCOPE:-.*complete' \
  "$cloud/deploy-broadcast.sh" | cut -d: -f1)"
account_line="$(grep -n 'sts get-caller-identity' \
  "$cloud/deploy-broadcast.sh" | cut -d: -f1)"
[ "$scope_line" -lt "$account_line" ] || {
  echo 'broadcast staging-scope gate occurs after an AWS call' >&2
  exit 1
}

# Receipt-owned fields must come from staged.env. Ambient values cannot make a
# missing or stale complete-stage receipt pass before the first AWS command.
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
fixture_repo="$test_root/repo"
fixture_cloud="$fixture_repo/cloud/aws"
fixture_dist="$fixture_cloud/dist"
fixture_bin="$test_root/bin"
install -d "$fixture_cloud/lib" "$fixture_cloud/runtime" "$fixture_dist" "$fixture_bin"
cp "$cloud/deploy-broadcast.sh" "$fixture_cloud/deploy-broadcast.sh"
cp "$cloud/lib/validation.sh" "$fixture_cloud/lib/validation.sh"
cp "$cloud/runtime/validate-world-manifest.jq" \
  "$fixture_cloud/runtime/validate-world-manifest.jq"
printf '#!/usr/bin/env bash\nexit 0\n' >"$fixture_cloud/source-world-stopped.sh"
chmod 0755 "$fixture_cloud/deploy-broadcast.sh" \
  "$fixture_cloud/source-world-stopped.sh"

runtime_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
game_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
bepinex_sha=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
cat >"$fixture_dist/worlds.json" <<'EOF'
{"schema":1,"worlds":[{"id":"slot-1","peerId":"slot-1-fixture",
"worldName":"Fixture World","sidecarPort":8787,"saveKey":"imports/Fixture.zip",
"credentialParameter":"/bibites/cloud/slot-1/peer-secret","position":"0,0",
"preferredSlot":1,"targetTimeScale":1,"saveMinutes":10,"saveKeep":6,
"enabled":false}]}
EOF
cp "$fixture_dist/worlds.json" "$test_root/complete-worlds.json"
manifest_sha="$(sha256sum "$fixture_dist/worlds.json" | awk '{print $1}')"
manifest_object="worlds.$manifest_sha.json"
cat >"$fixture_bin/aws" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$MOCK_AWS_LOG"
exit 65
EOF
chmod 0755 "$fixture_bin/aws"

run_invalid_receipt() {
  env PATH="$fixture_bin:$PATH" MOCK_AWS_LOG="$test_root/aws.log" \
    RUNTIME_SHA256="$runtime_sha" GAME_SHA256="$game_sha" \
    BEPINEX_SHA256="$bepinex_sha" \
    STAGING_SCOPE=complete STAGED_RUNTIME_SHA256="$runtime_sha" \
    STAGED_GAME_SHA256="$game_sha" STAGED_BEPINEX_SHA256="$bepinex_sha" \
    MANIFEST_OBJECT="$manifest_object" MANIFEST_SHA256="$manifest_sha" \
    "$fixture_cloud/deploy-broadcast.sh"
}

for receipt_case in missing-scope missing-digest stale-runtime stale-game \
  stale-bepinex missing-artifact missing-manifest-object missing-manifest-sha \
  stale-manifest changed-manifest; do
  cp "$test_root/complete-worlds.json" "$fixture_dist/worlds.json"
  cat >"$fixture_dist/artifacts.env" <<EOF
RUNTIME_SHA256=$runtime_sha
GAME_SHA256=$game_sha
BEPINEX_SHA256=$bepinex_sha
EOF
  cat >"$fixture_dist/staged.env" <<EOF
AWS_PROFILE=fixture
AWS_REGION=us-east-1
ARTIFACT_BUCKET=fixture-artifacts
ARTIFACT_PREFIX=cloud/v1
RUNTIME_OBJECT=runtime/$runtime_sha.tar.gz
STAGED_RUNTIME_SHA256=$runtime_sha
STAGED_GAME_SHA256=$game_sha
STAGED_BEPINEX_SHA256=$bepinex_sha
MANIFEST_OBJECT=$manifest_object
MANIFEST_SHA256=$manifest_sha
STAGING_SCOPE=complete
EOF
  case "$receipt_case" in
    missing-scope) sed -i '/^STAGING_SCOPE=/d' "$fixture_dist/staged.env" ;;
    missing-digest) sed -i '/^STAGED_RUNTIME_SHA256=/d' "$fixture_dist/staged.env" ;;
    stale-runtime)
      sed -i \
        's/^STAGED_RUNTIME_SHA256=.*/STAGED_RUNTIME_SHA256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd/' \
        "$fixture_dist/staged.env"
      ;;
    stale-game)
      sed -i \
        's/^STAGED_GAME_SHA256=.*/STAGED_GAME_SHA256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd/' \
        "$fixture_dist/staged.env"
      ;;
    stale-bepinex)
      sed -i \
        's/^STAGED_BEPINEX_SHA256=.*/STAGED_BEPINEX_SHA256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd/' \
        "$fixture_dist/staged.env"
      ;;
    missing-artifact) sed -i '/^RUNTIME_SHA256=/d' "$fixture_dist/artifacts.env" ;;
    missing-manifest-object) sed -i '/^MANIFEST_OBJECT=/d' "$fixture_dist/staged.env" ;;
    missing-manifest-sha) sed -i '/^MANIFEST_SHA256=/d' "$fixture_dist/staged.env" ;;
    stale-manifest)
      sed -i \
        's/^MANIFEST_SHA256=.*/MANIFEST_SHA256=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee/' \
        "$fixture_dist/staged.env"
      ;;
    changed-manifest) printf '\n' >>"$fixture_dist/worlds.json" ;;
  esac
  : >"$test_root/aws.log"
  set +e
  receipt_output="$(run_invalid_receipt 2>&1)"
  receipt_status=$?
  set -e
  [ "$receipt_status" -ne 0 ] || {
    echo "$receipt_case broadcast receipt reached deployment" >&2
    exit 1
  }
  [ ! -s "$test_root/aws.log" ] || {
    echo "$receipt_case broadcast receipt reached AWS" >&2
    exit 1
  }
  case "$receipt_case" in
    missing-scope)
      grep -Fq 'requires STAGING_SCOPE=complete' <<<"$receipt_output"
      ;;
    missing-digest)
      grep -Fq 'complete staging receipt is missing STAGED_RUNTIME_SHA256' \
        <<<"$receipt_output"
      ;;
    stale-runtime|stale-game|stale-bepinex|missing-artifact)
      grep -Fq 'complete staging receipt does not match artifacts.env' \
        <<<"$receipt_output"
      ;;
    missing-manifest-object)
      grep -Fq 'complete staging receipt is missing MANIFEST_OBJECT' \
        <<<"$receipt_output"
      ;;
    missing-manifest-sha)
      grep -Fq 'complete staging receipt is missing MANIFEST_SHA256' \
        <<<"$receipt_output"
      ;;
    stale-manifest)
      grep -Fq 'manifest object does not match its digest' \
        <<<"$receipt_output"
      ;;
    changed-manifest)
      grep -Fq 'staged manifest does not match the complete staging receipt' \
        <<<"$receipt_output"
      ;;
  esac
done

grep -Fq '  ManifestFile:' "$cloud/broadcast-template.yaml"
grep -Fq '  ManifestSha256:' "$cloud/broadcast-template.yaml"
grep -Fq 'ManifestFile="$MANIFEST_OBJECT"' "$cloud/deploy-broadcast.sh"
grep -Fq 'ManifestSha256="$MANIFEST_SHA256"' "$cloud/deploy-broadcast.sh"
grep -Fq '${ArtifactPrefix}/${ManifestFile}' "$cloud/broadcast-template.yaml"
grep -Fq "'\${ManifestSha256}' /tmp/bibites-worlds.json" \
  "$cloud/broadcast-template.yaml"
grep -Fq "any(.worlds[]; .id == \$world and .enabled == false)" \
  "$cloud/broadcast-template.yaml"

set +e
stream_output="$("$cloud/runtime/bibites-broadcast-stream" 2>&1)"
stream_status=$?
set -e
[ "$stream_status" -ne 0 ] && grep -Fq 'process arguments' <<<"$stream_output" || {
  echo 'broadcast stream command did not fail at the password-transport gate' >&2
  exit 1
}

if grep -Eq 'pass=|STREAM_PUBLISH_PASSWORD|rtmp://[^ ]*[?]' \
  "$cloud/runtime/bibites-broadcast-stream" \
  "$cloud/runtime/install-broadcast-host"; then
  echo 'broadcast runtime still contains an argument-based password transport' >&2
  exit 1
fi

gate_line="$(grep -n 'broadcast deployment is disabled' \
  "$cloud/deploy-broadcast.sh" | cut -d: -f1)"
send_line="$(grep -n 'ssm send-command' "$cloud/deploy-broadcast.sh" | cut -d: -f1)"
deploy_line="$(grep -n 'cloudformation deploy' "$cloud/deploy-broadcast.sh" | cut -d: -f1)"
[ "$gate_line" -lt "$send_line" ] && [ "$gate_line" -lt "$deploy_line" ] || {
  echo 'broadcast deployment gate occurs after a cloud mutation' >&2
  exit 1
}

for template in "$cloud/template.yaml" "$cloud/broadcast-template.yaml"; do
  grep -Fq 'CreationPolicy:' "$template"
  grep -Fq 'cloudformation signal-resource' "$template"
done
if grep -Fq 'AWS::EC2::VolumeAttachment' "$cloud/broadcast-template.yaml"; then
  echo 'broadcast-template.yaml still uses a post-instance volume attachment' >&2
  exit 1
fi
grep -Fq '    Condition: KeepLegacyDataAttachment' "$cloud/template.yaml"
grep -Fq "if [ '\${UseLegacyDataAttachment}' = true ]" "$cloud/template.yaml"

printf 'broadcast and bootstrap contract fixtures passed\n'
