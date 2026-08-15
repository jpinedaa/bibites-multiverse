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
  if grep -Fq 'AWS::EC2::VolumeAttachment' "$template"; then
    echo "$(basename "$template") still uses a post-instance volume attachment" >&2
    exit 1
  fi
done

printf 'broadcast and bootstrap contract fixtures passed\n'
