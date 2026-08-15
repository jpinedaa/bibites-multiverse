#!/usr/bin/env bash
# Shared validation for the local AWS deployment wrappers.

bibites_validation_error() {
  printf 'ERROR: %s\n' "$*" >&2
  return 1
}

bibites_require_account_id() {
  [[ "$1" =~ ^[0-9]{12}$ ]] ||
    bibites_validation_error "$2 must be a 12-digit AWS account identifier"
}

bibites_require_stack_name() {
  [[ "$1" =~ ^[A-Za-z][A-Za-z0-9-]{0,127}$ ]] ||
    bibites_validation_error "$2 is not a valid CloudFormation stack name"
}

bibites_require_region() {
  [[ "$1" =~ ^[a-z]{2}(-gov)?-[a-z]+-[0-9]+$ ]] ||
    bibites_validation_error "$2 is not a valid AWS region name"
}

bibites_require_instance_type() {
  [[ "$1" =~ ^[a-z0-9][a-z0-9.-]{0,63}$ ]] ||
    bibites_validation_error "$2 is not a valid EC2 instance type"
}

bibites_require_resource_id() {
  local value="$1" label="$2" prefix="$3"
  [[ "$value" =~ ^${prefix}-[0-9a-f]{8,17}$ ]] ||
    bibites_validation_error "$label is not a valid $prefix identifier"
}

bibites_require_availability_zone() {
  [[ "$1" =~ ^[a-z]{2}(-gov)?-[a-z]+-[0-9]+[a-z]$ ]] ||
    bibites_validation_error "$2 is not a valid availability zone"
}

bibites_require_sha256() {
  [[ "$1" =~ ^[0-9a-f]{64}$ ]] ||
    bibites_validation_error "$2 must be a lowercase SHA-256 value"
}

bibites_require_positive_integer() {
  local value="$1" label="$2" minimum="$3" maximum="$4"
  [[ "$value" =~ ^(0|[1-9][0-9]*)$ ]] ||
    bibites_validation_error "$label must be an integer from $minimum through $maximum" || return 1
  (( ${#value} <= ${#maximum} )) ||
    bibites_validation_error "$label must be an integer from $minimum through $maximum" || return 1
  (( 10#$value >= minimum && 10#$value <= maximum )) ||
    bibites_validation_error "$label must be an integer from $minimum through $maximum"
}

bibites_valid_s3_bucket() {
  local value="$1"
  (( ${#value} >= 3 && ${#value} <= 63 )) || return 1
  [[ "$value" =~ ^[a-z0-9][a-z0-9.-]*[a-z0-9]$ ]] || return 1
  [[ "$value" != *..* && "$value" != *.-* && "$value" != *-.* ]] || return 1
  [[ ! "$value" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

bibites_require_s3_bucket() {
  bibites_valid_s3_bucket "$1" ||
    bibites_validation_error "$2 is not a safe S3 bucket name"
}

bibites_valid_s3_path() {
  local value="$1" maximum="$2" segment
  (( ${#value} >= 1 && ${#value} <= maximum )) || return 1
  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]] || return 1
  [[ "$value" != */ && "$value" != *//* ]] || return 1
  local IFS=/
  read -r -a segments <<<"$value"
  for segment in "${segments[@]}"; do
    [[ "$segment" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1
  done
}

bibites_require_s3_prefix() {
  bibites_valid_s3_path "$1" 256 ||
    bibites_validation_error "$2 is not a safe S3 prefix"
}

bibites_require_s3_key() {
  bibites_valid_s3_path "$1" 1024 ||
    bibites_validation_error "$2 is not a safe S3 object key"
}

bibites_require_s3_filename() {
  local value="$1" label="$2"
  (( ${#value} >= 1 && ${#value} <= 256 )) &&
    [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] ||
    bibites_validation_error "$label is not a safe S3 object filename"
}

bibites_valid_ipv4() {
  local value="$1" octet
  local IFS=.
  read -r -a octets <<<"$value"
  [ "${#octets[@]}" -eq 4 ] || return 1
  for octet in "${octets[@]}"; do
    [[ "$octet" =~ ^(0|[1-9][0-9]{0,2})$ ]] || return 1
    (( 10#$octet <= 255 )) || return 1
  done
}

bibites_ipv4_to_integer() {
  local value="$1" a b c d
  bibites_valid_ipv4 "$value" || return 1
  IFS=. read -r a b c d <<<"$value"
  printf '%u\n' "$(( (10#$a << 24) | (10#$b << 16) | (10#$c << 8) | 10#$d ))"
}

bibites_is_rfc1918_ipv4() {
  local value="$1" number
  number="$(bibites_ipv4_to_integer "$value")" || return 1
  (( (number >= 167772160 && number <= 184549375) ||
     (number >= 2886729728 && number <= 2887778303) ||
     (number >= 3232235520 && number <= 3232301055) ))
}

bibites_require_rfc1918_ipv4() {
  bibites_is_rfc1918_ipv4 "$1" ||
    bibites_validation_error "$2 must be an RFC1918 IPv4 address"
}

bibites_valid_hostname() {
  local value="$1" label
  (( ${#value} >= 1 && ${#value} <= 253 )) || return 1
  [[ "$value" != .* && "$value" != *. && "$value" == *.* ]] || return 1
  local IFS=.
  read -r -a labels <<<"$value"
  for label in "${labels[@]}"; do
    (( ${#label} >= 1 && ${#label} <= 63 )) || return 1
    [[ "$label" =~ ^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$ ]] || return 1
  done
}

bibites_require_hostname() {
  bibites_valid_hostname "$1" ||
    bibites_validation_error "$2 must be a fully qualified DNS name"
}

bibites_valid_ssm_parameter() {
  local value="$1" segment
  (( ${#value} >= 2 && ${#value} <= 1011 )) || return 1
  [[ "$value" == /* && "$value" != */ && "$value" != *//* ]] || return 1
  local path="${value#/}" IFS=/
  read -r -a segments <<<"$path"
  for segment in "${segments[@]}"; do
    [[ "$segment" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || return 1
  done
}

bibites_require_ssm_parameter() {
  bibites_valid_ssm_parameter "$1" ||
    bibites_validation_error "$2 is not a safe SSM parameter path"
}

bibites_require_x86_64_instance_type() {
  local profile="$1" region="$2" instance_type="$3" description
  description="$(aws --profile "$profile" --region "$region" ec2 describe-instance-types \
    --instance-types "$instance_type" --output json)" || return 1
  jq -e --arg instance_type "$instance_type" '
    (.InstanceTypes | length) == 1 and
    .InstanceTypes[0].InstanceType == $instance_type and
    (.InstanceTypes[0].ProcessorInfo.SupportedArchitectures | index("x86_64") != null)
  ' <<<"$description" >/dev/null ||
    bibites_validation_error "$instance_type does not report x86_64 support"
}

bibites_valid_ipv4_cidr() {
  local value="$1" address bits
  [[ "$value" == */* ]] || return 1
  address="${value%/*}"
  bits="${value##*/}"
  bibites_valid_ipv4 "$address" || return 1
  [[ "$bits" =~ ^(0|[1-2]?[0-9]|3[0-2])$ ]]
}

bibites_ipv4_in_cidr() {
  local address="$1" cidr="$2" bits mask address_number network_number
  bibites_valid_ipv4_cidr "$cidr" || return 1
  address_number="$(bibites_ipv4_to_integer "$address")" || return 1
  network_number="$(bibites_ipv4_to_integer "${cidr%/*}")" || return 1
  bits="${cidr##*/}"
  if [ "$bits" -eq 0 ]; then
    mask=0
  else
    mask=$(( (0xffffffff << (32 - bits)) & 0xffffffff ))
  fi
  (( (address_number & mask) == (network_number & mask) ))
}

bibites_is_rfc1918_cidr() {
  local cidr="$1" bits mask first last
  bibites_valid_ipv4_cidr "$cidr" || return 1
  bits="${cidr##*/}"
  first="$(bibites_ipv4_to_integer "${cidr%/*}")" || return 1
  if [ "$bits" -eq 0 ]; then
    mask=0
  else
    mask=$(( (0xffffffff << (32 - bits)) & 0xffffffff ))
  fi
  first=$(( first & mask ))
  last=$(( first | (0xffffffff ^ mask) ))
  (( (first >= 167772160 && last <= 184549375) ||
     (first >= 2886729728 && last <= 2887778303) ||
     (first >= 3232235520 && last <= 3232301055) ))
}

bibites_effective_route_table() {
  local profile="$1" region="$2" vpc_id="$3" subnet_id="$4" description count
  description="$(aws --profile "$profile" --region "$region" ec2 describe-route-tables \
    --filters "Name=association.subnet-id,Values=$subnet_id" --output json)" || return 1
  count="$(jq -er '.RouteTables | length' <<<"$description")" || return 1
  if [ "$count" -eq 0 ]; then
    description="$(aws --profile "$profile" --region "$region" ec2 describe-route-tables \
      --filters "Name=vpc-id,Values=$vpc_id" 'Name=association.main,Values=true' \
      --output json)" || return 1
    count="$(jq -er '.RouteTables | length' <<<"$description")" || return 1
  fi
  [ "$count" -eq 1 ] ||
    bibites_validation_error "the selected subnet does not resolve to one effective route table" || return 1
  printf '%s\n' "$description"
}

bibites_private_route_for_ip() {
  local description="$1" address="$2" cidr target
  while IFS=$'\t' read -r cidr target; do
    [ -n "$cidr" ] || continue
    bibites_is_rfc1918_cidr "$cidr" || continue
    bibites_ipv4_in_cidr "$address" "$cidr" || continue
    case "$target" in
      local|pcx-*|tgw-*|vgw-*) printf '%s via %s\n' "$cidr" "$target"; return 0 ;;
    esac
  done < <(jq -r '
    .RouteTables[0].Routes[] |
    select(.State == "active") |
    [(.DestinationCidrBlock // ""),
     (.GatewayId // .VpcPeeringConnectionId // .TransitGatewayId // "")] |
    @tsv
  ' <<<"$description")
  return 1
}
