#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib/validation.sh
. "$repo/cloud/aws/lib/validation.sh"
# shellcheck source=lib/host-change.sh
. "$repo/cloud/aws/lib/host-change.sh"

reject() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo "unsafe fixture passed: $label" >&2
    exit 1
  fi
}

change() {
  jq -nc --arg id "$1" --arg type "$2" --arg action "$3" --arg replacement "$4" \
    '{ResourceChange:{LogicalResourceId:$id,ResourceType:$type,
      Action:$action,Replacement:$replacement}}'
}

create_changes="$(jq -sc '.' <<EOF
$(change HostSecurityGroup AWS::EC2::SecurityGroup Add False)
$(change HostRole AWS::IAM::Role Add False)
$(change HostProfile AWS::IAM::InstanceProfile Add False)
$(change DataVolume AWS::EC2::Volume Add False)
$(change HostLaunchTemplate AWS::EC2::LaunchTemplate Add False)
$(change Host AWS::EC2::Instance Add False)
EOF
)"
create_description="$(jq -nc --argjson changes "$create_changes" \
  '{Status:"CREATE_COMPLETE",ChangeSetType:"CREATE",Changes:$changes}')"
bibites_require_safe_host_change_set "$create_description"

update_description="$(jq -nc --argjson change \
  "$(change HostLaunchTemplate AWS::EC2::LaunchTemplate Modify False)" \
  '{Status:"CREATE_COMPLETE",ChangeSetType:"UPDATE",Changes:[$change]}')"
bibites_require_safe_host_change_set "$update_description"

for unsafe in \
  "$(change Host AWS::EC2::Instance Modify True)" \
  "$(change Host AWS::EC2::Instance Modify False)" \
  "$(change DataAttachment AWS::EC2::VolumeAttachment Remove False)" \
  "$(change DataVolume AWS::EC2::Volume Modify False)" \
  "$(change DataVolume AWS::EC2::Volume Remove False)" \
  "$(change HostRole AWS::IAM::Role Modify False)" \
  "$(change HostSecurityGroup AWS::EC2::SecurityGroup Modify False)"; do
  description="$(jq -nc --argjson change "$unsafe" \
    '{Status:"CREATE_COMPLETE",ChangeSetType:"UPDATE",Changes:[$change]}')"
  reject 'unsafe update change' bibites_require_safe_host_change_set "$description"
done

extra_change="$(jq -nc \
  --argjson launch "$(change HostLaunchTemplate AWS::EC2::LaunchTemplate Modify False)" \
  --argjson role "$(change HostRole AWS::IAM::Role Modify False)" \
  '{Status:"CREATE_COMPLETE",ChangeSetType:"UPDATE",Changes:[$launch,$role]}')"
reject 'dormant launch-template update with an unrelated change' \
  bibites_require_safe_host_change_set "$extra_change"

coupled_replacement="$(jq -nc \
  --argjson launch "$(change HostLaunchTemplate AWS::EC2::LaunchTemplate Modify False)" \
  --argjson host "$(change Host AWS::EC2::Instance Modify True)" \
  --argjson attachment "$(change DataAttachment AWS::EC2::VolumeAttachment Remove False)" \
  '{Status:"CREATE_COMPLETE",ChangeSetType:"UPDATE",
    Changes:[$launch,$host,$attachment]}')"
reject 'launch-template update coupled to Host replacement and attachment removal' \
  bibites_require_safe_host_change_set "$coupled_replacement"

parameter_description='{"Parameters":[
  {"ParameterKey":"RuntimeObject","ParameterValue":"runtime/abc.tar.gz"},
  {"ParameterKey":"HostLaunchTemplateVersion","ParameterValue":"7"}]}'
bibites_require_change_set_parameter \
  "$parameter_description" RuntimeObject runtime/abc.tar.gz
reject 'runtime pointer drift' bibites_require_change_set_parameter \
  "$parameter_description" RuntimeObject runtime/other.tar.gz
reject 'launch-template version drift' bibites_require_change_set_parameter \
  "$parameter_description" HostLaunchTemplateVersion 8

[ "$(bibites_change_set_type_for_stack_status '')" = CREATE ]
[ "$(bibites_change_set_type_for_stack_status REVIEW_IN_PROGRESS)" = CREATE ]
[ "$(bibites_change_set_type_for_stack_status UPDATE_COMPLETE)" = UPDATE ]
bibites_require_change_set_name reviewed-host-20260821 fixture
reject 'unsafe change-set name' bibites_require_change_set_name '../host' fixture
bibites_require_launch_template_version 7 fixture
reject 'symbolic launch-template version' \
  bibites_require_launch_template_version '$Latest' fixture

resources='{"StackResourceSummaries":[
  {"LogicalResourceId":"Host","ResourceType":"AWS::EC2::Instance",
   "PhysicalResourceId":"i-0123456789abcdef0"},
  {"LogicalResourceId":"HostLaunchTemplate","ResourceType":"AWS::EC2::LaunchTemplate",
   "PhysicalResourceId":"lt-0123456789abcdef0"},
  {"LogicalResourceId":"DataAttachment","ResourceType":"AWS::EC2::VolumeAttachment",
   "PhysicalResourceId":"vol-0123456789abcdef0-i-0123456789abcdef0"}]}'
[ "$(bibites_legacy_attachment_mode "$resources")" = true ]
[ "$(bibites_legacy_attachment_mode \
  "$(jq 'del(.StackResourceSummaries[2])' <<<"$resources")")" = false ]
reject 'wrong legacy attachment type' bibites_legacy_attachment_mode \
  "$(jq '.StackResourceSummaries[2].ResourceType = "AWS::EC2::Volume"' <<<"$resources")"

live_host='{"Reservations":[{"Instances":[{
  "InstanceId":"i-0123456789abcdef0",
  "LaunchTemplate":{"LaunchTemplateId":"lt-0123456789abcdef0","Version":"7"},
  "Tags":[
    {"Key":"aws:ec2launchtemplate:id","Value":"lt-0123456789abcdef0"},
    {"Key":"aws:ec2launchtemplate:version","Value":"7"}]}]}]}'
binding="$(bibites_live_host_launch_template_binding "$resources" "$live_host")"
[ "$binding" = $'i-0123456789abcdef0\tlt-0123456789abcdef0\t7' ]
tag_only_host="$(jq 'del(.Reservations[0].Instances[0].LaunchTemplate)' <<<"$live_host")"
[ "$(bibites_live_host_launch_template_binding "$resources" "$tag_only_host")" = \
  $'i-0123456789abcdef0\tlt-0123456789abcdef0\t7' ]
reject 'live Host on a different launch template' \
  bibites_live_host_launch_template_binding "$resources" \
  "$(jq '.Reservations[0].Instances[0].LaunchTemplate.LaunchTemplateId =
    "lt-fedcba98765432100"' <<<"$live_host")"
reject 'live Host using a symbolic launch-template version' \
  bibites_live_host_launch_template_binding "$resources" \
  "$(jq '.Reservations[0].Instances[0].LaunchTemplate.Version = "$Latest"' \
    <<<"$live_host")"
reject 'reserved launch-template id differs from the direct binding' \
  bibites_live_host_launch_template_binding "$resources" \
  "$(jq '.Reservations[0].Instances[0].Tags[0].Value =
    "lt-fedcba98765432100"' <<<"$live_host")"
reject 'tag-only Host using a symbolic launch-template version' \
  bibites_live_host_launch_template_binding "$resources" \
  "$(jq 'del(.Reservations[0].Instances[0].LaunchTemplate) |
    .Reservations[0].Instances[0].Tags[1].Value = "$Latest"' <<<"$live_host")"
reject 'tag-only Host missing the reserved launch-template version' \
  bibites_live_host_launch_template_binding "$resources" \
  "$(jq 'del(.Reservations[0].Instances[0].LaunchTemplate) |
    del(.Reservations[0].Instances[0].Tags[1])' <<<"$live_host")"
reject 'tag-only Host with duplicate reserved launch-template versions' \
  bibites_live_host_launch_template_binding "$resources" \
  "$(jq 'del(.Reservations[0].Instances[0].LaunchTemplate) |
    .Reservations[0].Instances[0].Tags +=
      [{"Key":"aws:ec2launchtemplate:version","Value":"7"}]' <<<"$live_host")"

test_root="$(mktemp -d)"
trap 'find "$test_root" -depth -delete' EXIT
wait_state="$test_root/wait-state"
printf '0\n' >"$wait_state"
wait_responses=(
  '{"Stacks":[{"StackStatus":"REVIEW_IN_PROGRESS","CreationTime":"create"}]}'
  '{"Stacks":[{"StackStatus":"CREATE_IN_PROGRESS","CreationTime":"create"}]}'
  '{"Stacks":[{"StackStatus":"CREATE_COMPLETE","CreationTime":"create"}]}'
)
aws() {
  local index
  index="$(<"$wait_state")"
  printf '%s\n' "$((index + 1))" >"$wait_state"
  printf '%s\n' "${wait_responses[$index]}"
}
result="$(bibites_wait_stack_terminal fixture us-east-1 fixture-stack 5 0 '')"
[ "$(jq -r '.Stacks[0].StackStatus' <<<"$result")" = CREATE_COMPLETE ]
[ "$(<"$wait_state")" -eq 3 ]
unset -f aws

# Run the actual wrapper against isolated AWS fixtures. The CREATE fixture uses
# self-attach mode. The legacy UPDATE fixture binds the live numeric version and
# preserves DataAttachment. Pointer publication must remain safe across terminal
# failure and concurrent publication.
fixture_repo="$test_root/repo"
fixture_cloud="$fixture_repo/cloud/aws"
fixture_dist="$fixture_cloud/dist"
fixture_bin="$test_root/bin"
install -d "$fixture_cloud/lib" "$fixture_cloud/runtime" "$fixture_dist" "$fixture_bin"
cp "$repo/cloud/aws/deploy-host.sh" "$fixture_cloud/deploy-host.sh"
cp "$repo/cloud/aws/template.yaml" "$fixture_cloud/template.yaml"
cp "$repo/cloud/aws/lib/validation.sh" "$fixture_cloud/lib/validation.sh"
cp "$repo/cloud/aws/lib/host-change.sh" "$fixture_cloud/lib/host-change.sh"
cp "$repo/cloud/aws/promote-runtime.sh" "$fixture_cloud/promote-runtime.sh"
cp "$repo/cloud/aws/runtime/validate-world-manifest.jq" \
  "$fixture_cloud/runtime/validate-world-manifest.jq"

archive="$test_root/runtime.tar.gz"
printf 'fixture runtime\n' >"$archive"
runtime_sha="$(sha256sum "$archive" | awk '{print $1}')"
game_archive="$test_root/game.zip"
printf 'fixture game\n' >"$game_archive"
game_sha="$(sha256sum "$game_archive" | awk '{print $1}')"
bepinex_archive="$test_root/bepinex.zip"
printf 'fixture BepInEx\n' >"$bepinex_archive"
bepinex_sha="$(sha256sum "$bepinex_archive" | awk '{print $1}')"
cat >"$fixture_dist/artifacts.env" <<EOF
RUNTIME_FILE=bibites-cloud-runtime.tar.gz
RUNTIME_OBJECT=runtime/$runtime_sha.tar.gz
RUNTIME_SHA256=$runtime_sha
GAME_FILE=game.zip
GAME_SHA256=$game_sha
BEPINEX_FILE=bepinex.zip
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
MANIFEST_OBJECT=worlds.MANIFEST_SHA_PLACEHOLDER.json
MANIFEST_SHA256=MANIFEST_SHA_PLACEHOLDER
STAGING_SCOPE=complete
EOF
cat >"$fixture_dist/worlds.json" <<'EOF'
{"schema":1,"worlds":[{"id":"slot-1","peerId":"slot-1-fixture",
"worldName":"Fixture-World","sidecarPort":8787,"saveKey":"imports/Fixture-World.zip",
"credentialParameter":"/bibites-multiverse/cloud/slot-1/peer-secret","position":"0,0",
"preferredSlot":1,"targetTimeScale":1,"saveMinutes":10,"saveKeep":6,
"enabled":true}]}
EOF
manifest_sha="$(sha256sum "$fixture_dist/worlds.json" | awk '{print $1}')"
sed -i "s/MANIFEST_SHA_PLACEHOLDER/$manifest_sha/g" "$fixture_dist/staged.env"
chmod 0755 "$fixture_cloud/deploy-host.sh" "$fixture_cloud/promote-runtime.sh"

cat >"$fixture_bin/aws" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
args=" $* "
printf '%s\n' "$*" >>"$MOCK_AWS_CALL_LOG"
case "$args" in
  *' sts get-caller-identity '*) printf '123456789012\n' ;;
  *' cloudformation describe-stacks '*)
    case "$MOCK_SCENARIO" in
      legacy_preview|legacy_unaddressed)
        if [ "$MOCK_SCENARIO" = legacy_preview ]; then
          legacy_runtime="runtime/$MOCK_RUNTIME_SHA.tar.gz"
        else
          legacy_runtime=legacy/runtime.tar.gz
        fi
        jq -nc --arg sha "$MOCK_RUNTIME_SHA" --arg runtime "$legacy_runtime" '{Stacks:[{
          StackStatus:"UPDATE_COMPLETE",StackId:"arn:aws:cloudformation:us-east-1:123456789012:stack/fixture/1",
          CreationTime:"old",LastUpdatedTime:"old",Parameters:[
            {ParameterKey:"ArtifactBucket",ParameterValue:"fixture-artifacts"},
            {ParameterKey:"ArtifactPrefix",ParameterValue:"cloud/v1"},
            {ParameterKey:"RuntimeFile",ParameterValue:$runtime},
            {ParameterKey:"RuntimeSha256",ParameterValue:$sha}]}]}'
        ;;
      create_preview)
        echo 'Stack with id fixture does not exist' >&2
        exit 255
        ;;
      create_execute_failure|create_execute_success|create_execute_race_different|create_execute_race_identical|create_execute_pointer_drift|create_execute_pointer_same)
        if [ -e "$MOCK_EXECUTE_STATE" ]; then
          if [ "$MOCK_SCENARIO" = create_execute_failure ]; then
            printf '%s\n' '{"Stacks":[{"StackStatus":"CREATE_FAILED","CreationTime":"new"}]}'
          else
            case "$MOCK_SCENARIO" in
              create_execute_race_different)
                other_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
                jq -nc --arg file "runtime/$other_sha.tar.gz" --arg sha "$other_sha" \
                  '{runtimeSha256:$sha,schema:1,runtimeFile:$file}' \
                  >"$MOCK_POINTER_STATE"
                ;;
              create_execute_race_identical)
                jq -nc --arg file "runtime/$MOCK_RUNTIME_SHA.tar.gz" \
                  --arg sha "$MOCK_RUNTIME_SHA" \
                  '{runtimeSha256:$sha,schema:1,runtimeFile:$file}' \
                  >"$MOCK_POINTER_STATE"
                ;;
              create_execute_pointer_drift)
                other_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
                jq -nc --arg file "runtime/$other_sha.tar.gz" --arg sha "$other_sha" \
                  '{runtimeSha256:$sha,schema:1,runtimeFile:$file}' \
                  >"$MOCK_POINTER_STATE"
                printf '%s\n' '"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"' \
                  >"$MOCK_POINTER_ETAG_STATE"
                ;;
              create_execute_pointer_same)
                printf '%s\n' '"cccccccccccccccccccccccccccccccc"' \
                  >"$MOCK_POINTER_ETAG_STATE"
                ;;
            esac
            printf '%s\n' '{"Stacks":[{"StackStatus":"CREATE_COMPLETE","CreationTime":"new","Outputs":[{"OutputKey":"InstanceId","OutputValue":"i-0123456789abcdef0"}]}]}'
          fi
        else
          printf '%s\n' '{"Stacks":[{"StackStatus":"REVIEW_IN_PROGRESS","CreationTime":"new"}]}'
        fi
        ;;
      *) exit 65 ;;
    esac
    ;;
  *' cloudformation list-stack-resources '*)
    printf '%s\n' '{"StackResourceSummaries":[
      {"LogicalResourceId":"Host","ResourceType":"AWS::EC2::Instance","PhysicalResourceId":"i-0123456789abcdef0"},
      {"LogicalResourceId":"HostLaunchTemplate","ResourceType":"AWS::EC2::LaunchTemplate","PhysicalResourceId":"lt-0123456789abcdef0"},
      {"LogicalResourceId":"DataAttachment","ResourceType":"AWS::EC2::VolumeAttachment","PhysicalResourceId":"fixture"}]}'
    ;;
  *' ec2 describe-instances '*)
    printf '%s\n' '{"Reservations":[{"Instances":[{"InstanceId":"i-0123456789abcdef0",
      "Tags":[
        {"Key":"aws:ec2launchtemplate:id","Value":"lt-0123456789abcdef0"},
        {"Key":"aws:ec2launchtemplate:version","Value":"7"}]}]}]}'
    ;;
  *' ec2 describe-subnets '*)
    printf '%s\n' '{"Subnets":[{"SubnetId":"subnet-0123456789abcdef0",
      "VpcId":"vpc-0123456789abcdef0","AvailabilityZone":"us-east-1a"}]}'
    ;;
  *' ec2 describe-instance-types '*)
    printf '%s\n' '{"InstanceTypes":[{"InstanceType":"m6i.large",
      "ProcessorInfo":{"SupportedArchitectures":["x86_64"]}}]}'
    ;;
  *' ssm describe-parameters '*)
    printf '%s\n' '{"Parameters":[{"Name":"/bibites-multiverse/cloud/slot-1/peer-secret",
      "Type":"SecureString","KeyId":"alias/aws/ssm"}]}'
    ;;
  *' s3api get-object '*)
    destination=''; key=''
    while [ "$#" -gt 0 ]; do
      case "$1" in
        get-object) shift ;;
        --key) key="$2"; shift 2 ;;
        --bucket|--output) shift 2 ;;
        --*) shift ;;
        *) destination="$1"; shift ;;
      esac
    done
    [ -n "$destination" ] || exit 65
    case "$key" in
      */runtime/current.json)
        if [ -e "$MOCK_POINTER_STATE" ]; then
          cp "$MOCK_POINTER_STATE" "$destination"
          jq -nc --arg etag "$(<"$MOCK_POINTER_ETAG_STATE")" '{ETag:$etag}'
        else
          echo 'An error occurred (NoSuchKey): 404 Not Found' >&2
          exit 1
        fi
        ;;
      */worlds.json|*/worlds.*.json)
        cp "$MOCK_MANIFEST_PATH" "$destination"
        jq -nc --arg etag "$MOCK_MANIFEST_ETAG" '{ETag:$etag}'
        ;;
      *) exit 65 ;;
    esac
    ;;
  *' s3 cp '*)
    source=''; destination=''
    while [ "$#" -gt 0 ]; do
      if [ "$1" = cp ]; then source="$2"; destination="$3"; break; fi
      shift
    done
    if [[ "$source" == */runtime/current.json ]]; then
      if [ -e "$MOCK_POINTER_STATE" ]; then
        cat "$MOCK_POINTER_STATE"
      else
        echo '404 Not Found' >&2
        exit 1
      fi
    elif [[ "$source" == */game.zip ]]; then
      cp "$MOCK_GAME_ARCHIVE" "$destination"
    elif [[ "$source" == */bepinex.zip ]]; then
      cp "$MOCK_BEPINEX_ARCHIVE" "$destination"
    else
      cp "$MOCK_ARCHIVE" "$destination"
    fi
    ;;
  *' s3api put-object '*)
    printf '%s\n' "$*" >>"$MOCK_CONDITIONAL_LOG"
    body=''
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --body ]; then body="$2"; break; fi
      shift
    done
    [ -n "$body" ] || exit 65
    if [ -e "$MOCK_POINTER_STATE" ]; then
      echo 'An error occurred (PreconditionFailed) when calling PutObject: 412' >&2
      exit 255
    fi
    cp "$body" "$MOCK_POINTER_STATE"
    printf '%s\n' '"dddddddddddddddddddddddddddddddd"' \
      >"$MOCK_POINTER_ETAG_STATE"
    ;;
  *' cloudformation create-change-set '*)
    printf '%s\n' "$*" >>"$MOCK_AWS_LOG"
    ;;
  *' cloudformation describe-change-set '*)
    if [ "$MOCK_SCENARIO" = legacy_preview ]; then
      type=UPDATE; runtime="runtime/$MOCK_RUNTIME_SHA.tar.gz"; version=7; legacy=true
      changes='[{"ResourceChange":{"LogicalResourceId":"HostLaunchTemplate","ResourceType":"AWS::EC2::LaunchTemplate","Action":"Modify","Replacement":"False"}}]'
    else
      type=CREATE; runtime="runtime/$MOCK_RUNTIME_SHA.tar.gz"; version=1; legacy=false
      changes='[
        {"ResourceChange":{"LogicalResourceId":"HostSecurityGroup","ResourceType":"AWS::EC2::SecurityGroup","Action":"Add","Replacement":"False"}},
        {"ResourceChange":{"LogicalResourceId":"HostRole","ResourceType":"AWS::IAM::Role","Action":"Add","Replacement":"False"}},
        {"ResourceChange":{"LogicalResourceId":"HostProfile","ResourceType":"AWS::IAM::InstanceProfile","Action":"Add","Replacement":"False"}},
        {"ResourceChange":{"LogicalResourceId":"DataVolume","ResourceType":"AWS::EC2::Volume","Action":"Add","Replacement":"False"}},
        {"ResourceChange":{"LogicalResourceId":"HostLaunchTemplate","ResourceType":"AWS::EC2::LaunchTemplate","Action":"Add","Replacement":"False"}},
        {"ResourceChange":{"LogicalResourceId":"Host","ResourceType":"AWS::EC2::Instance","Action":"Add","Replacement":"False"}}]'
    fi
    jq -nc --arg type "$type" --arg runtime "$runtime" --arg sha "$MOCK_RUNTIME_SHA" \
      --arg game_sha "$MOCK_GAME_SHA" --arg bepinex_sha "$MOCK_BEPINEX_SHA" \
      --arg manifest "$MOCK_MANIFEST_OBJECT" --arg manifest_sha "$MOCK_MANIFEST_SHA" \
      --arg version "$version" --arg legacy "$legacy" --argjson changes "$changes" '{
      ChangeSetName:"fixture-change",ChangeSetType:$type,Status:"CREATE_COMPLETE",
      ExecutionStatus:"AVAILABLE",Parameters:[
        {ParameterKey:"InstanceType",ParameterValue:"m6i.large"},
        {ParameterKey:"AvailabilityZone",ParameterValue:"us-east-1a"},
        {ParameterKey:"SubnetId",ParameterValue:"subnet-0123456789abcdef0"},
        {ParameterKey:"VpcId",ParameterValue:"vpc-0123456789abcdef0"},
        {ParameterKey:"ArtifactBucket",ParameterValue:"fixture-artifacts"},
        {ParameterKey:"ArtifactPrefix",ParameterValue:"cloud/v1"},
        {ParameterKey:"RuntimeObject",ParameterValue:$runtime},
        {ParameterKey:"RuntimeSha256",ParameterValue:$sha},
        {ParameterKey:"GameFile",ParameterValue:"game.zip"},
        {ParameterKey:"GameSha256",ParameterValue:$game_sha},
        {ParameterKey:"BepInExFile",ParameterValue:"bepinex.zip"},
        {ParameterKey:"BepInExSha256",ParameterValue:$bepinex_sha},
        {ParameterKey:"ManifestFile",ParameterValue:$manifest},
        {ParameterKey:"ManifestSha256",ParameterValue:$manifest_sha},
        {ParameterKey:"DataVolumeGiB",ParameterValue:"40"},
        {ParameterKey:"RelayPrivateIp",ParameterValue:"10.0.0.5"},
        {ParameterKey:"RelayDomain",ParameterValue:"relay.example.net"},
        {ParameterKey:"CredentialParameterPrefix",ParameterValue:"/bibites-multiverse/cloud"},
        {ParameterKey:"HostLaunchTemplateVersion",ParameterValue:$version},
        {ParameterKey:"UseLegacyDataAttachment",ParameterValue:$legacy}],Changes:$changes}'
    ;;
  *' cloudformation execute-change-set '*)
    printf '%s\n' "$*" >>"$MOCK_AWS_LOG"
    touch "$MOCK_EXECUTE_STATE"
    ;;
  *)
    echo "unexpected aws fixture command: $*" >&2
    exit 65
    ;;
esac
MOCK
chmod 0755 "$fixture_bin/aws"

run_deploy_fixture() {
  local scenario="$1"
  shift
  env PATH="$fixture_bin:$PATH" AWS_PROFILE=fixture AWS_REGION=us-east-1 \
    BIBITES_AWS_ACCOUNT_ID=123456789012 \
    BIBITES_SUBNET_ID=subnet-0123456789abcdef0 \
    BIBITES_VPC_ID=vpc-0123456789abcdef0 BIBITES_AVAILABILITY_ZONE=us-east-1a \
    BIBITES_RELAY_PRIVATE_IP=10.0.0.5 BIBITES_RELAY_DOMAIN=relay.example.net \
    BIBITES_CREDENTIAL_PARAMETER_PREFIX=/bibites-multiverse/cloud BIBITES_INSTANCE_TYPE=m6i.large \
    MOCK_SCENARIO="$scenario" MOCK_RUNTIME_SHA="$runtime_sha" MOCK_ARCHIVE="$archive" \
    MOCK_GAME_SHA="$game_sha" MOCK_GAME_ARCHIVE="$game_archive" \
    MOCK_BEPINEX_SHA="$bepinex_sha" MOCK_BEPINEX_ARCHIVE="$bepinex_archive" \
    MOCK_MANIFEST_OBJECT="worlds.$manifest_sha.json" \
    MOCK_MANIFEST_SHA="$manifest_sha" \
    MOCK_MANIFEST_PATH="$fixture_dist/worlds.json" \
    MOCK_MANIFEST_ETAG='"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"' \
    MOCK_AWS_LOG="$test_root/aws.log" MOCK_EXECUTE_STATE="$test_root/executed" \
    MOCK_AWS_CALL_LOG="$test_root/aws-calls.log" \
    MOCK_POINTER_STATE="$test_root/pointer.json" \
    MOCK_POINTER_ETAG_STATE="$test_root/pointer.etag" \
    MOCK_CONDITIONAL_LOG="$test_root/conditional.log" \
    "$fixture_cloud/deploy-host.sh" --change-set-name fixture-change "$@"
}

seed_pointer() {
  local etag="$1"
  jq -nc --arg file "runtime/$runtime_sha.tar.gz" --arg sha "$runtime_sha" \
    '{schema:1,runtimeFile:$file,runtimeSha256:$sha}' >"$test_root/pointer.json"
  printf '%s\n' "$etag" >"$test_root/pointer.etag"
}

sed -i '/^STAGING_SCOPE=/d' "$fixture_dist/staged.env"
export STAGING_SCOPE=complete
: >"$test_root/aws.log"
: >"$test_root/aws-calls.log"
set +e
scope_output="$(run_deploy_fixture create_preview 2>&1)"
scope_status=$?
set -e
[ "$scope_status" -ne 0 ] || {
  echo 'missing staging scope reached deployment' >&2
  exit 1
}
[ ! -s "$test_root/aws-calls.log" ] || {
  echo 'missing staging scope reached AWS' >&2
  exit 1
}
grep -Fq 'requires a complete or runtime-only receipt' <<<"$scope_output"
unset STAGING_SCOPE
sed -i '/^STAGING_SCOPE=/d' "$fixture_dist/staged.env"
printf 'STAGING_SCOPE=complete\n' >>"$fixture_dist/staged.env"

for stale_name in STAGED_RUNTIME_SHA256 STAGED_GAME_SHA256 STAGED_BEPINEX_SHA256; do
  prior_value="$(sed -n "s/^$stale_name=//p" "$fixture_dist/staged.env")"
  sed -i "s/^$stale_name=.*/$stale_name=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/" \
    "$fixture_dist/staged.env"
  : >"$test_root/aws-calls.log"
  set +e
  stale_output="$(run_deploy_fixture create_preview 2>&1)"
  stale_status=$?
  set -e
  [ "$stale_status" -ne 0 ]
  [ ! -s "$test_root/aws-calls.log" ] || {
    echo "$stale_name mismatch reached AWS" >&2
    exit 1
  }
  grep -Fq 'complete staging receipt does not match artifacts.env' <<<"$stale_output"
  sed -i "s/^$stale_name=.*/$stale_name=$prior_value/" "$fixture_dist/staged.env"
done

sed -i '/^STAGED_RUNTIME_SHA256=/d' "$fixture_dist/staged.env"
export STAGED_RUNTIME_SHA256="$runtime_sha"
: >"$test_root/aws-calls.log"
set +e
missing_digest_output="$(run_deploy_fixture create_preview 2>&1)"
missing_digest_status=$?
set -e
unset STAGED_RUNTIME_SHA256
[ "$missing_digest_status" -ne 0 ]
[ ! -s "$test_root/aws-calls.log" ]
grep -Fq 'complete staging receipt is missing STAGED_RUNTIME_SHA256' \
  <<<"$missing_digest_output"
printf 'STAGED_RUNTIME_SHA256=%s\n' "$runtime_sha" >>"$fixture_dist/staged.env"

cp "$fixture_dist/artifacts.env" "$test_root/complete-artifacts.env"
sed -i '/^RUNTIME_SHA256=/d' "$fixture_dist/artifacts.env"
export RUNTIME_SHA256="$runtime_sha"
: >"$test_root/aws-calls.log"
set +e
missing_artifact_output="$(run_deploy_fixture create_preview 2>&1)"
missing_artifact_status=$?
set -e
unset RUNTIME_SHA256
[ "$missing_artifact_status" -ne 0 ]
[ ! -s "$test_root/aws-calls.log" ] || {
  echo 'ambient runtime digest filled an incomplete artifacts.env' >&2
  exit 1
}
grep -Fq 'complete staging receipt does not match artifacts.env' \
  <<<"$missing_artifact_output"
cp "$test_root/complete-artifacts.env" "$fixture_dist/artifacts.env"

cp "$fixture_dist/staged.env" "$test_root/complete-staged.env"
for missing_manifest_field in MANIFEST_OBJECT MANIFEST_SHA256; do
  cp "$test_root/complete-staged.env" "$fixture_dist/staged.env"
  sed -i "/^$missing_manifest_field=/d" "$fixture_dist/staged.env"
  if [ "$missing_manifest_field" = MANIFEST_OBJECT ]; then
    export MANIFEST_OBJECT="worlds.$manifest_sha.json"
  else
    export MANIFEST_SHA256="$manifest_sha"
  fi
  : >"$test_root/aws-calls.log"
  set +e
  missing_manifest_output="$(run_deploy_fixture create_preview 2>&1)"
  missing_manifest_status=$?
  set -e
  unset MANIFEST_OBJECT MANIFEST_SHA256
  [ "$missing_manifest_status" -ne 0 ]
  [ ! -s "$test_root/aws-calls.log" ] || {
    echo "ambient $missing_manifest_field filled an incomplete staging receipt" >&2
    exit 1
  }
  grep -Fq "complete staging receipt is missing $missing_manifest_field" \
    <<<"$missing_manifest_output"
done

cp "$test_root/complete-staged.env" "$fixture_dist/staged.env"
sed -i \
  's/^MANIFEST_SHA256=.*/MANIFEST_SHA256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd/' \
  "$fixture_dist/staged.env"
: >"$test_root/aws-calls.log"
set +e
stale_manifest_output="$(run_deploy_fixture create_preview 2>&1)"
stale_manifest_status=$?
set -e
[ "$stale_manifest_status" -ne 0 ]
[ ! -s "$test_root/aws-calls.log" ]
grep -Fq 'manifest object does not match its digest' <<<"$stale_manifest_output"

cp "$test_root/complete-staged.env" "$fixture_dist/staged.env"
cp "$fixture_dist/worlds.json" "$test_root/worlds-before-mutation.json"
printf '\n' >>"$fixture_dist/worlds.json"
: >"$test_root/aws-calls.log"
set +e
changed_manifest_output="$(run_deploy_fixture create_preview 2>&1)"
changed_manifest_status=$?
set -e
[ "$changed_manifest_status" -ne 0 ]
[ ! -s "$test_root/aws-calls.log" ]
grep -Fq 'staged manifest does not match the complete staging receipt' \
  <<<"$changed_manifest_output"
mv "$test_root/worlds-before-mutation.json" "$fixture_dist/worlds.json"

: >"$test_root/aws.log"
run_deploy_fixture create_preview >/dev/null
grep -Fq 'ParameterKey=HostLaunchTemplateVersion,ParameterValue=1' \
  "$test_root/aws.log"
grep -Fq 'ParameterKey=UseLegacyDataAttachment,ParameterValue=false' \
  "$test_root/aws.log"
grep -Fq "ParameterKey=ManifestFile,ParameterValue=worlds.$manifest_sha.json" \
  "$test_root/aws.log"
grep -Fq "ParameterKey=ManifestSha256,ParameterValue=$manifest_sha" \
  "$test_root/aws.log"

: >"$test_root/aws.log"
run_deploy_fixture legacy_preview >/dev/null
grep -Fq 'ParameterKey=HostLaunchTemplateVersion,ParameterValue=7' "$test_root/aws.log"
grep -Fq 'ParameterKey=UseLegacyDataAttachment,ParameterValue=true' "$test_root/aws.log"

cat >"$fixture_dist/staged.env" <<EOF
AWS_PROFILE=fixture
AWS_REGION=us-east-1
ARTIFACT_BUCKET=fixture-artifacts
ARTIFACT_PREFIX=cloud/v1
RUNTIME_OBJECT=runtime/$runtime_sha.tar.gz
RUNTIME_SHA256=$runtime_sha
GAME_OBJECT=runtime-inputs/game/$game_sha.zip
GAME_SHA256=$game_sha
BEPINEX_OBJECT=runtime-inputs/bepinex/$bepinex_sha.zip
BEPINEX_SHA256=$bepinex_sha
MANIFEST_OBJECT=worlds.$manifest_sha.json
MANIFEST_SHA256=$manifest_sha
MANIFEST_PREIMAGE_ETAG='"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"'
STAGING_SCOPE=runtime-only
EOF
seed_pointer '"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
: >"$test_root/aws.log"
: >"$test_root/aws-calls.log"
run_deploy_fixture legacy_preview >/dev/null
grep -Fq 'ParameterKey=HostLaunchTemplateVersion,ParameterValue=7' "$test_root/aws.log"
grep -Fq "s3api get-object --bucket fixture-artifacts --key cloud/v1/worlds.$manifest_sha.json" \
  "$test_root/aws-calls.log"
grep -Fq 's3api get-object --bucket fixture-artifacts --key cloud/v1/worlds.json' \
  "$test_root/aws-calls.log"

sed -i \
  's/^MANIFEST_PREIMAGE_ETAG=.*/MANIFEST_PREIMAGE_ETAG='\''"dddddddddddddddddddddddddddddddd"'\''/' \
  "$fixture_dist/staged.env"
: >"$test_root/aws.log"
set +e
manifest_drift_output="$(run_deploy_fixture legacy_preview 2>&1)"
manifest_drift_status=$?
set -e
[ "$manifest_drift_status" -ne 0 ]
[ ! -s "$test_root/aws.log" ]
grep -Fq 'mutable manifest ETag changed after runtime-only staging' \
  <<<"$manifest_drift_output"
sed -i \
  's/^MANIFEST_PREIMAGE_ETAG=.*/MANIFEST_PREIMAGE_ETAG='\''"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"'\''/' \
  "$fixture_dist/staged.env"

: >"$test_root/aws.log"
set +e
runtime_create_output="$(run_deploy_fixture create_preview 2>&1)"
runtime_create_status=$?
set -e
[ "$runtime_create_status" -ne 0 ]
[ ! -s "$test_root/aws.log" ]
grep -Fq 'runtime-only receipt can reconcile only an existing stack' \
  <<<"$runtime_create_output"

cp "$test_root/complete-staged.env" "$fixture_dist/staged.env"
rm -f "$test_root/pointer.json" "$test_root/pointer.etag"

for legacy_mode in preview execute; do
  : >"$test_root/aws.log"
  rm -f "$test_root/executed" "$test_root/pointer.json" "$test_root/pointer.etag"
  set +e
  if [ "$legacy_mode" = execute ]; then
    legacy_output="$(run_deploy_fixture legacy_unaddressed --execute 2>&1)"
  else
    legacy_output="$(run_deploy_fixture legacy_unaddressed 2>&1)"
  fi
  legacy_status=$?
  set -e
  [ "$legacy_status" -ne 0 ]
  grep -Fq 'legacy stack runtime is not a matching content-addressed object' \
    <<<"$legacy_output"
  [ ! -s "$test_root/aws.log" ] || {
    echo "legacy missing-pointer $legacy_mode reached a stack mutation" >&2
    exit 1
  }
  [ ! -e "$test_root/executed" ]
done

rm -f "$test_root/executed" "$test_root/pointer.json" "$test_root/pointer.etag" \
  "$test_root/conditional.log"
reject 'failed CREATE execution' run_deploy_fixture create_execute_failure --execute
[ ! -e "$test_root/pointer.json" ]
[ ! -e "$test_root/conditional.log" ]

rm -f "$test_root/executed" "$test_root/pointer.json" "$test_root/pointer.etag" \
  "$test_root/conditional.log"
run_deploy_fixture create_execute_success --execute >/dev/null
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
[ "$(wc -l <"$test_root/conditional.log")" -eq 1 ]
grep -Fq -- '--if-none-match *' "$test_root/conditional.log"

rm -f "$test_root/executed" "$test_root/pointer.json" "$test_root/pointer.etag" \
  "$test_root/conditional.log"
reject 'different concurrent runtime pointer' \
  run_deploy_fixture create_execute_race_different --execute
other_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$other_sha" ]
[ "$(wc -l <"$test_root/conditional.log")" -eq 1 ]

rm -f "$test_root/executed" "$test_root/pointer.json" "$test_root/pointer.etag" \
  "$test_root/conditional.log"
run_deploy_fixture create_execute_race_identical --execute >/dev/null
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
[ "$(wc -l <"$test_root/conditional.log")" -eq 1 ]

rm -f "$test_root/executed" "$test_root/conditional.log"
seed_pointer '"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
set +e
pointer_drift_output="$(run_deploy_fixture create_execute_pointer_drift --execute 2>&1)"
pointer_drift_status=$?
set -e
[ "$pointer_drift_status" -ne 0 ]
[ -e "$test_root/executed" ]
[ ! -e "$test_root/conditional.log" ]
other_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$other_sha" ]
grep -Fq 'Deployment status: partial' <<<"$pointer_drift_output"
grep -Fq 'Reconcile runtime/current.json' <<<"$pointer_drift_output"

rm -f "$test_root/executed" "$test_root/conditional.log"
seed_pointer '"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
run_deploy_fixture create_execute_pointer_same --execute >/dev/null
[ -e "$test_root/executed" ]
[ ! -e "$test_root/conditional.log" ]
[ "$(jq -r .runtimeSha256 "$test_root/pointer.json")" = "$runtime_sha" ]
[ "$(<"$test_root/pointer.etag")" = '"cccccccccccccccccccccccccccccccc"' ]

template="$repo/cloud/aws/template.yaml"
deploy="$repo/cloud/aws/deploy-host.sh"
install_host="$repo/cloud/aws/runtime/install-host"
sync_worlds="$repo/cloud/aws/runtime/bibites-sync-worlds"
grep -Fq '  HostLaunchTemplateVersion:' "$template"
grep -Fq '  UseLegacyDataAttachment:' "$template"
grep -Fq '  KeepLegacyDataAttachment: !Equals' "$template"
grep -Fq '    Condition: KeepLegacyDataAttachment' "$template"
grep -Fq '        Version: !Ref HostLaunchTemplateVersion' "$template"
grep -Fq '                  - KeepLegacyDataAttachment' "$template"
grep -Fq 'parameter/bibites-multiverse/cloud/*' "$template"
grep -B3 -F '{Key: BibitesBackup, Value: daily}' "$template" | \
  grep -Fq -- '- !If'
if grep -Fq 'LatestVersionNumber' "$template"; then
  echo 'Host still follows the mutable latest launch-template version' >&2
  exit 1
fi
grep -Fq "if [ '\${UseLegacyDataAttachment}' = true ]" "$template"
grep -Fq "'s3://\${ArtifactBucket}/\${ArtifactPrefix}/\${RuntimeObject}'" "$template"
grep -Fq "'\${RuntimeSha256}' /tmp/bibites-runtime.tar.gz" "$template"
grep -Fq "MANIFEST_KEY=\${ArtifactPrefix}/worlds.json" "$template"
grep -Fq "export MANIFEST_SHA256='\${ManifestSha256}'" "$template"
grep -Fq ': "${MANIFEST_SHA256:?}"' "$install_host"
grep -Fq "printf '%s  %s\\n' \"\$MANIFEST_SHA256\" \"\$stage/worlds.json\"" \
  "$sync_worlds"
grep -Fq "AllowedPattern: '^worlds\\.[0-9a-f]{64}\\.json$'" "$template"
if grep -A2 '^  ManifestFile:' "$template" | grep -Fq 'Default:'; then
  echo 'ManifestFile still defaults to mutable worlds.json' >&2
  exit 1
fi
if grep -Fq 'runtime/current.json' "$template"; then
  echo 'bootstrap still dereferences the mutable runtime pointer' >&2
  exit 1
fi

set +e
missing_name_output="$(env -u BIBITES_CHANGE_SET_NAME "$deploy" 2>&1)"
missing_name_status=$?
set -e
[ "$missing_name_status" -eq 2 ]
grep -Fq 'for every preview and execution' <<<"$missing_name_output"
grep -Fq 'bibites_require_safe_host_change_set "$change_set_description"' "$deploy"
grep -Fq 'use_legacy_attachment="$(bibites_legacy_attachment_mode' "$deploy"
grep -Fq 'bibites_live_host_launch_template_binding' "$deploy"
grep -Fq 'runtime-only receipt can reconcile only an existing stack' "$deploy"
grep -Fq 'use that exact prefix so the reviewed change leaves live IAM unchanged' "$deploy"
success_guard_line="$(grep -n '\[ "$terminal_status" -eq 0 \]' "$deploy" | cut -d: -f1)"
publish_line="$(grep -n '"$repo/cloud/aws/promote-runtime.sh"' "$deploy" | cut -d: -f1)"
[ "$success_guard_line" -lt "$publish_line" ] || {
  echo 'runtime pointer can publish before terminal stack success' >&2
  exit 1
}
grep -Fq -- '"$repo/cloud/aws/promote-runtime.sh" --if-absent' "$deploy"
grep -Fq 's3api get-object' "$deploy"
grep -Fq 'Deployment status: partial. Reconcile runtime/current.json' "$deploy"

printf 'host update safety fixtures passed\n'
