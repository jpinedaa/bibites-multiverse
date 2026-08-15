#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib/validation.sh
. "$repo/cloud/aws/lib/validation.sh"

accept() {
  local label="$1"
  shift
  "$@" || { printf 'valid fixture failed: %s\n' "$label" >&2; exit 1; }
}

reject() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    printf 'invalid fixture passed: %s\n' "$label" >&2
    exit 1
  fi
}

accept 'artifact prefix' bibites_valid_s3_path 'cloud/v1' 256
accept 'content-addressed key' bibites_valid_s3_path \
  'cloud/v1/runtime/0123456789abcdef.tar.gz' 1024
reject 'S3 traversal' bibites_valid_s3_path 'cloud/../private' 1024
reject 'empty S3 segment' bibites_valid_s3_path 'cloud//private' 1024
reject 'shell text in S3 key' bibites_valid_s3_path 'cloud/$(id)' 1024

accept 'private class A address' bibites_is_rfc1918_ipv4 '10.2.3.4'
accept 'private class B address' bibites_is_rfc1918_ipv4 '172.31.255.254'
accept 'private class C address' bibites_is_rfc1918_ipv4 '192.168.4.5'
reject 'public address' bibites_is_rfc1918_ipv4 '198.51.100.10'
reject 'invalid address' bibites_is_rfc1918_ipv4 '10.2.3.999'

accept 'relay DNS name' bibites_valid_hostname 'relay.example.net'
reject 'single DNS label' bibites_valid_hostname 'relay'
reject 'shell text in DNS name' bibites_valid_hostname 'relay.$(id).net'

accept 'parameter path' bibites_valid_ssm_parameter \
  '/bibites-multiverse/cloud/world-a/peer-secret'
reject 'parameter traversal' bibites_valid_ssm_parameter '/bibites/../peer-secret'

route_table='{
  "RouteTables": [{
    "Routes": [
      {"DestinationCidrBlock":"10.0.0.0/16","GatewayId":"local","State":"active"},
      {"DestinationCidrBlock":"172.20.0.0/16","VpcPeeringConnectionId":"pcx-123","State":"active"},
      {"DestinationCidrBlock":"0.0.0.0/0","GatewayId":"igw-123","State":"active"}
    ]
  }]
}'
accept 'local private route' bibites_private_route_for_ip "$route_table" '10.0.2.4'
accept 'peered private route' bibites_private_route_for_ip "$route_table" '172.20.4.5'
reject 'internet route' bibites_private_route_for_ip "$route_table" '192.168.4.5'

printf 'AWS validation fixtures passed\n'
