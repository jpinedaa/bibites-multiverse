#!/usr/bin/env bash
# Import the stopped laptop world into the existing cloud host and verify all six worlds.
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: $0 /path/to/M4-Slot6.zip" >&2; exit 2; }

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
profile="${AWS_PROFILE:-bibites-multiverse}"
region="${AWS_REGION:-us-east-1}"
stack="${BIBITES_STACK_NAME:-bibites-cloud-worlds}"
prefix="${BIBITES_ARTIFACT_PREFIX:-cloud/v1}"
expected_account="663615031964"
src="$1"

for command in aws curl jq sha256sum unzip; do
  command -v "$command" >/dev/null || { echo "missing required command: $command" >&2; exit 1; }
done

if [ ! -f "$src" ] && command -v wslpath >/dev/null 2>&1; then
  src="$(wslpath -u "$src")"
fi
[ -f "$src" ] || { echo "not a file: $src" >&2; exit 1; }
[ "$(basename "$src")" = M4-Slot6.zip ] || {
  echo 'the source file must be named M4-Slot6.zip' >&2
  exit 1
}

if command -v powershell.exe >/dev/null 2>&1 &&
  powershell.exe -NoProfile -NonInteractive -Command \
    'if (Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -like "*Bibites*" }) { exit 0 } else { exit 1 }' \
    >/dev/null 2>&1; then
  echo 'The Bibites is still running. Close it cleanly, wait for the save to finish, and retry.' >&2
  exit 1
fi
if command -v pgrep >/dev/null 2>&1 &&
  { pgrep -x 'The Bibites' >/dev/null 2>&1 || pgrep -x 'The Bibites.x86_64' >/dev/null 2>&1; }; then
  echo 'The Bibites is still running. Close it cleanly, wait for the save to finish, and retry.' >&2
  exit 1
fi

account="$(aws --profile "$profile" --region "$region" sts get-caller-identity \
  --query Account --output text)"
[ "$account" = "$expected_account" ] || {
  echo "refusing AWS account $account; expected $expected_account" >&2
  exit 1
}
bucket="${BIBITES_ARTIFACT_BUCKET:-bibites-multiverse-cloud-$account-$region}"
manifest_uri="s3://$bucket/$prefix/worlds.json"
save_key="$prefix/imports/M4-Slot6.zip"

aws --profile "$profile" --region "$region" s3api head-bucket --bucket "$bucket" >/dev/null
aws --profile "$profile" --region "$region" ssm get-parameter \
  --name /bibites-multiverse/cloud/slot-6/peer-secret \
  --query Parameter.Name --output text >/dev/null

tmpdir="$(mktemp -d)"
trap 'rm -rf -- "$tmpdir"' EXIT
current="$tmpdir/worlds.current.json"
next="$tmpdir/worlds.next.json"
latest="$tmpdir/worlds.latest.json"
remote_save="$tmpdir/M4-Slot6.remote.zip"

aws --profile "$profile" --region "$region" s3 cp "$manifest_uri" "$current" --only-show-errors
jq -e '
  .schema == 1 and
  (.worlds | type == "array") and
  ([.worlds[].id] | length == (unique | length)) and
  (all(.worlds[];
    (.preferredSlot | type == "number") and
    (.sidecarPort | type == "number") and
    (.targetTimeScale == 100) and
    (.enabled == true)))
' "$current" >/dev/null

world_ids="$(jq -c '[.worlds[].id] | sort' "$current")"
case "$world_ids" in
  '["slot-1","slot-2","slot-3","slot-4","slot-5"]')
    AWS_PROFILE="$profile" AWS_REGION="$region" BIBITES_STACK_NAME="$stack" \
      "$repo/cloud/aws/verify-host.sh"
    jq '
      .worlds += [{
        id:"slot-6", peerId:"slot-6", worldName:"M4-Slot6", sidecarPort:8792,
        saveKey:"imports/M4-Slot6.zip",
        credentialParameter:"/bibites-multiverse/cloud/slot-6/peer-secret",
        position:"2,1", preferredSlot:6, targetTimeScale:100,
        saveMinutes:10, saveKeep:6, enabled:true
      }] |
      .worlds |= sort_by(.preferredSlot)
    ' "$current" > "$next"
    ;;
  '["slot-1","slot-2","slot-3","slot-4","slot-5","slot-6"]')
    jq -e '
      any(.worlds[];
        .id == "slot-6" and .peerId == "slot-6" and .worldName == "M4-Slot6" and
        .sidecarPort == 8792 and .saveKey == "imports/M4-Slot6.zip" and
        .credentialParameter == "/bibites-multiverse/cloud/slot-6/peer-secret" and
        .position == "2,1" and .preferredSlot == 6 and .targetTimeScale == 100 and
        .saveMinutes == 10 and .saveKeep == 6 and .enabled == true)
    ' "$current" >/dev/null
    cp "$current" "$next"
    ;;
  *)
    echo "unexpected cloud manifest world set: $world_ids" >&2
    exit 1
    ;;
esac

jq -e '
  (.worlds | length == 6) and
  ([.worlds[].id] | sort == ["slot-1","slot-2","slot-3","slot-4","slot-5","slot-6"]) and
  ([.worlds[].preferredSlot] | sort == [1,2,3,4,5,6]) and
  ([.worlds[].sidecarPort] | length == (unique | length)) and
  ([.worlds[].position] | length == (unique | length))
' "$next" >/dev/null

"$repo/cloud/aws/import-slot6-save.sh" "$src"
local_save="$repo/cloud/aws/imports/M4-Slot6.zip"
local_sha="$(sha256sum "$local_save" | awk '{print $1}')"

if aws --profile "$profile" --region "$region" s3api head-object \
  --bucket "$bucket" --key "$save_key" >/dev/null 2>&1; then
  aws --profile "$profile" --region "$region" s3 cp \
    "s3://$bucket/$save_key" "$remote_save" --only-show-errors
  remote_sha="$(sha256sum "$remote_save" | awk '{print $1}')"
  [ "$local_sha" = "$remote_sha" ] || {
    echo "cloud slot-6 save differs from the laptop save; refusing to replace it" >&2
    echo "laptop: $local_sha" >&2
    echo "cloud:  $remote_sha" >&2
    exit 1
  }
else
  aws --profile "$profile" --region "$region" s3 cp "$local_save" \
    "s3://$bucket/$save_key" --only-show-errors --metadata "sha256=$local_sha"
fi

if ! cmp -s "$current" "$next"; then
  aws --profile "$profile" --region "$region" s3 cp "$manifest_uri" "$latest" --only-show-errors
  cmp -s "$current" "$latest" || {
    echo 'the cloud manifest changed during migration; no manifest was replaced' >&2
    echo 'The save is staged safely. Rerun this command to reconcile the new manifest.' >&2
    exit 1
  }
  aws --profile "$profile" --region "$region" s3 cp "$next" "$manifest_uri" --only-show-errors
fi

AWS_PROFILE="$profile" AWS_REGION="$region" BIBITES_STACK_NAME="$stack" \
  "$repo/cloud/aws/sync-host.sh"
AWS_PROFILE="$profile" AWS_REGION="$region" BIBITES_STACK_NAME="$stack" \
  "$repo/cloud/aws/verify-host.sh"

ready=0
for _ in $(seq 1 60); do
  if curl -fsS https://bibitesmultiverse.com/api/status -o "$tmpdir/public-status.json" &&
    jq -e '
      .relayConnected == true and .map.width == 3 and .map.height == 2 and
      .slotCount == 6 and .totals.liveSlots == 6 and .totals.darkSlots == 0 and
      ([.slots[].slot] | sort == [1,2,3,4,5,6]) and
      (all(.slots[]; .live == true and .modConnected == true and .timeScale == 100))
    ' "$tmpdir/public-status.json" >/dev/null; then
    ready=1
    break
  fi
  sleep 5
done
[ "$ready" -eq 1 ] || {
  echo 'the host passed its local checks, but the public map did not show six ready worlds' >&2
  exit 1
}

printf 'slot 6 migration complete: sha256=%s\n' "$local_sha"
echo 'All six cloud worlds are live. Do not restart the retired laptop copy.'
