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

bibites_require_change_set_name() {
  [[ "$1" =~ ^[A-Za-z][-A-Za-z0-9]{0,127}$ ]] ||
    bibites_validation_error "$2 is not a valid CloudFormation change-set name"
}

bibites_require_launch_template_version() {
  [[ "$1" =~ ^[1-9][0-9]*$ ]] ||
    bibites_validation_error "$2 is not a positive launch-template version"
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

bibites_runtime_pointer_document() {
  local runtime_file="$1" runtime_sha256="$2"
  bibites_require_sha256 "$runtime_sha256" 'runtime pointer digest' || return 1
  [ "$runtime_file" = "runtime/$runtime_sha256.tar.gz" ] ||
    bibites_validation_error \
      'runtime pointer file must be the content-addressed object for its digest' || return 1
  jq -cn --arg file "$runtime_file" --arg sha256 "$runtime_sha256" \
    '{schema:1,runtimeFile:$file,runtimeSha256:$sha256}'
}

bibites_require_runtime_pointer_document() {
  local document="$1"
  jq -e '
    (keys | sort) == ["runtimeFile", "runtimeSha256", "schema"] and
    .schema == 1 and
    (.runtimeSha256 | type == "string" and test("^[0-9a-f]{64}$")) and
    .runtimeFile == ("runtime/" + .runtimeSha256 + ".tar.gz")
  ' <<<"$document" >/dev/null ||
    bibites_validation_error 'runtime pointer is not a valid schema-1 document'
}

bibites_require_unique_save_basenames() {
  local save_key save_basename
  local -A source_by_basename=()
  for save_key in "$@"; do
    save_basename="$(basename "$save_key")"
    if [ -n "${source_by_basename[$save_basename]:-}" ] &&
       [ "${source_by_basename[$save_basename]}" != "$save_key" ]; then
      bibites_validation_error \
        "$save_key and ${source_by_basename[$save_basename]} use the same local save filename"
      return 1
    fi
    source_by_basename[$save_basename]="$save_key"
  done
}

bibites_require_save_archive() {
  local archive="$1" label="$2" contents
  [ -r "$archive" ] ||
    bibites_validation_error "$label is not readable" || return 1
  unzip -tqq "$archive" ||
    bibites_validation_error "$label is not a valid ZIP archive" || return 1
  contents="$(unzip -Z1 "$archive")" ||
    bibites_validation_error "$label contents cannot be read" || return 1
  grep -Fxq 'scene.bb8scene' <<<"$contents" ||
    bibites_validation_error "$label does not contain scene.bb8scene"
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
  bibites_require_x86_64_instance_description "$description" "$instance_type"
}

bibites_require_x86_64_instance_description() {
  local description="$1" instance_type="$2"
  jq -e --arg instance_type "$instance_type" '
    (.InstanceTypes | length) == 1 and
    .InstanceTypes[0].InstanceType == $instance_type and
    (.InstanceTypes[0].ProcessorInfo.SupportedArchitectures | index("x86_64") != null)
  ' <<<"$description" >/dev/null ||
    bibites_validation_error "$instance_type does not report x86_64 support"
}

bibites_require_nvidia_gpu_instance_description() {
  local description="$1" instance_type="$2"
  jq -e --arg instance_type "$instance_type" '
    (.InstanceTypes | length) == 1 and
    .InstanceTypes[0].InstanceType == $instance_type and
    any((.InstanceTypes[0].GpuInfo.Gpus // [])[];
      .Manufacturer == "NVIDIA" and
      (.Count | type == "number" and . >= 1))
  ' <<<"$description" >/dev/null ||
    bibites_validation_error "$instance_type does not report an NVIDIA GPU"
}

bibites_instance_default_vcpus() {
  local description="$1" instance_type="$2"
  jq -er --arg instance_type "$instance_type" '
    if (.InstanceTypes | length) == 1 and
       .InstanceTypes[0].InstanceType == $instance_type and
       (.InstanceTypes[0].VCpuInfo.DefaultVCpus | type == "number") and
       .InstanceTypes[0].VCpuInfo.DefaultVCpus ==
         (.InstanceTypes[0].VCpuInfo.DefaultVCpus | floor) and
       .InstanceTypes[0].VCpuInfo.DefaultVCpus >= 1
    then .InstanceTypes[0].VCpuInfo.DefaultVCpus
    else error("missing or invalid DefaultVCpus")
    end
  ' <<<"$description" ||
    bibites_validation_error "$instance_type does not report valid default vCPUs"
}

bibites_require_default_secure_parameter_metadata() {
  local description="$1" parameter_name="$2"
  jq -e --arg name "$parameter_name" '
    (.Parameters | length) == 1 and
    .Parameters[0].Name == $name and
    .Parameters[0].Type == "SecureString" and
    ((.Parameters[0].KeyId // "") as $key |
      $key == "alias/aws/ssm" or $key == "aws/ssm" or
      ($key | endswith(":alias/aws/ssm")))
  ' <<<"$description" >/dev/null ||
    bibites_validation_error \
      "$parameter_name must be a SecureString encrypted with the default aws/ssm KMS key"
}

bibites_require_default_secure_parameter() {
  local profile="$1" region="$2" parameter_name="$3" description
  description="$(aws --profile "$profile" --region "$region" ssm describe-parameters \
    --parameter-filters "Key=Name,Option=Equals,Values=$parameter_name" \
    --output json)" || return 1
  bibites_require_default_secure_parameter_metadata "$description" "$parameter_name"
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
  local description="$1" address="$2" cidr state target bits prefix_list_route_count
  local selected_cidr='' selected_state='' selected_target='' selected_bits=-1
  local selected_count=0

  bibites_valid_ipv4 "$address" || return 1
  prefix_list_route_count="$(jq -er '
    [.RouteTables[0].Routes[]? |
      select(((.DestinationPrefixListId // "") | length) > 0)] |
    length
  ' <<<"$description")" || return 1
  (( prefix_list_route_count == 0 )) || return 1
  while IFS=$'\t' read -r cidr state target; do
    [ -n "$cidr" ] || continue
    bibites_valid_ipv4_cidr "$cidr" || continue
    bibites_ipv4_in_cidr "$address" "$cidr" || continue
    bits="${cidr##*/}"
    if (( 10#$bits > selected_bits )); then
      selected_cidr="$cidr"
      selected_state="$state"
      selected_target="$target"
      selected_bits=$((10#$bits))
      selected_count=1
    elif (( 10#$bits == selected_bits )); then
      selected_count=$((selected_count + 1))
    fi
  done < <(jq -r '
    .RouteTables[0].Routes[] |
    [(.DestinationCidrBlock // ""),
     (.State // ""),
     (.GatewayId // .VpcPeeringConnectionId // .TransitGatewayId //
      .NatGatewayId // .NetworkInterfaceId // .InstanceId //
      .EgressOnlyInternetGatewayId // .CarrierGatewayId //
      .CoreNetworkArn // .LocalGatewayId // "")] |
    @tsv
  ' <<<"$description")

  (( selected_bits >= 0 && selected_count == 1 )) || return 1
  bibites_is_rfc1918_cidr "$selected_cidr" || return 1
  [ "$selected_state" = active ] || return 1
  case "$selected_target" in
    local|pcx-*|tgw-*|vgw-*)
      printf '%s via %s\n' "$selected_cidr" "$selected_target"
      ;;
    *) return 1 ;;
  esac
}

bibites_wait_ssm_invocation() {
  local profile="$1" region="$2" command_id="$3" instance_id="$4"
  local timeout_seconds="$5" poll_seconds="$6" deadline response status error_file

  [[ "$command_id" =~ ^[0-9a-f-]{36}$ ]] ||
    bibites_validation_error "Systems Manager returned an invalid command identifier" || return 1
  bibites_require_resource_id "$instance_id" 'Systems Manager instance identifier' i || return 1
  bibites_require_positive_integer "$timeout_seconds" \
    'Systems Manager wait timeout' 1 7200 || return 1
  [[ "$poll_seconds" =~ ^(0|[1-9][0-9]?)$ ]] ||
    bibites_validation_error 'Systems Manager poll interval must be from 0 through 99 seconds' ||
    return 1

  error_file="$(mktemp)"
  deadline=$(( $(date +%s) + 10#$timeout_seconds ))
  while (( $(date +%s) < deadline )); do
    if response="$(aws --profile "$profile" --region "$region" \
      --cli-connect-timeout 5 --cli-read-timeout 10 ssm get-command-invocation \
      --command-id "$command_id" --instance-id "$instance_id" --output json \
      2>"$error_file")"; then
      status="$(jq -er '.Status | select(type == "string")' <<<"$response")" || {
        rm -f "$error_file"
        bibites_validation_error 'Systems Manager returned an invalid invocation document'
        return 2
      }
      case "$status" in
        Success)
          rm -f "$error_file"
          printf '%s\n' "$response"
          return 0
          ;;
        Failed|Cancelled|TimedOut)
          rm -f "$error_file"
          printf '%s\n' "$response"
          return 1
          ;;
        Pending|InProgress|Delayed|Cancelling) ;;
        *)
          rm -f "$error_file"
          bibites_validation_error "Systems Manager returned unknown status $status"
          return 2
          ;;
      esac
    elif grep -Fq InvocationDoesNotExist "$error_file"; then
      : # Systems Manager can hide a new invocation for a short time.
    else
      sed 's/^/Systems Manager status error: /' "$error_file" >&2
      rm -f "$error_file"
      return 2
    fi
    sleep "$poll_seconds"
  done

  rm -f "$error_file"
  bibites_validation_error \
    "Systems Manager command $command_id did not reach a terminal state in $timeout_seconds seconds"
  return 124
}
