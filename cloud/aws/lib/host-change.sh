#!/usr/bin/env bash
# Classify named host-stack change sets and wait for bounded terminal states.

bibites_change_set_type_for_stack_status() {
  case "$1" in
    ''|REVIEW_IN_PROGRESS) printf 'CREATE\n' ;;
    *) printf 'UPDATE\n' ;;
  esac
}

bibites_require_change_set_parameter() {
  local description="$1" key="$2" expected="$3" actual
  actual="$(jq -er --arg key "$key" '
    [.Parameters[] | select(.ParameterKey == $key) | .ParameterValue] |
    if length == 1 then .[0]
    else error("missing or duplicate change-set parameter " + $key)
    end
  ' <<<"$description")" || return 1
  [ "$actual" = "$expected" ] || {
    bibites_validation_error \
      "change-set parameter $key does not match the validated deployment input"
    return 1
  }
}

bibites_change_set_summary() {
  jq '{
    changeSetName: .ChangeSetName,
    type: .ChangeSetType,
    status: .Status,
    changes: [
      .Changes[]?.ResourceChange |
      {
        action: .Action,
        logicalResourceId: .LogicalResourceId,
        resourceType: .ResourceType,
        replacement: (.Replacement // "NotApplicable")
      }
    ]
  }' <<<"$1"
}

bibites_legacy_attachment_mode() {
  local resources="$1" count
  count="$(jq '[.StackResourceSummaries[]? |
    select(.LogicalResourceId == "DataAttachment")] | length' <<<"$resources")" || return 1
  case "$count" in
    0) printf 'false\n' ;;
    1)
      jq -e 'any(.StackResourceSummaries[]?;
        .LogicalResourceId == "DataAttachment" and
        .ResourceType == "AWS::EC2::VolumeAttachment")' \
        <<<"$resources" >/dev/null || {
        bibites_validation_error 'DataAttachment has an unexpected resource type'
        return 1
      }
      printf 'true\n'
      ;;
    *)
      bibites_validation_error 'stack contains duplicate DataAttachment resources'
      return 1
      ;;
  esac
}

bibites_live_host_launch_template_binding() {
  local resources="$1" live_host="$2" host_id launch_template_id version
  host_id="$(jq -er '
    [.StackResourceSummaries[] | select(
      .LogicalResourceId == "Host" and .ResourceType == "AWS::EC2::Instance") |
      .PhysicalResourceId] |
    if length == 1 then .[0] else error("missing or duplicate Host resource") end
  ' <<<"$resources")" || return 1
  launch_template_id="$(jq -er '
    [.StackResourceSummaries[] | select(
      .LogicalResourceId == "HostLaunchTemplate" and
      .ResourceType == "AWS::EC2::LaunchTemplate") | .PhysicalResourceId] |
    if length == 1 then .[0]
    else error("missing or duplicate HostLaunchTemplate resource") end
  ' <<<"$resources")" || return 1
  bibites_require_resource_id "$host_id" 'stack Host resource' i || return 1
  bibites_require_resource_id "$launch_template_id" \
    'stack HostLaunchTemplate resource' lt || return 1
  version="$(jq -er --arg host "$host_id" --arg launch_template "$launch_template_id" '
    def numeric_version:
      type == "string" and test("^[1-9][0-9]*$");
    ([.Reservations[]?.Instances[]?] |
      if length == 1 and .[0].InstanceId == $host then .[0]
      else error("missing or different live Host")
      end) as $instance |
    [$instance.Tags[]? |
      select(.Key == "aws:ec2launchtemplate:id") | .Value] as $tag_ids |
    [$instance.Tags[]? |
      select(.Key == "aws:ec2launchtemplate:version") | .Value] as $tag_versions |
    ($instance.LaunchTemplate? // null) as $direct |
    if $direct != null then
      if $direct.LaunchTemplateId == $launch_template and
         ($direct.Version | numeric_version) and
         ((($tag_ids | length) == 0 and ($tag_versions | length) == 0) or
          (($tag_ids | length) == 1 and ($tag_versions | length) == 1 and
           $tag_ids[0] == $direct.LaunchTemplateId and
           $tag_versions[0] == $direct.Version))
      then $direct.Version
      else error("invalid direct launch-template binding")
      end
    else
      if ($tag_ids | length) == 1 and ($tag_versions | length) == 1 and
         $tag_ids[0] == $launch_template and
         ($tag_versions[0] | numeric_version)
      then $tag_versions[0]
      else error("invalid reserved launch-template tags")
      end
    end
  ' <<<"$live_host")" || {
    bibites_validation_error \
      'live Host does not prove one numeric version of the stack launch template'
    return 1
  }
  bibites_require_launch_template_version "$version" \
    'live Host launch-template version' || return 1
  printf '%s\t%s\t%s\n' "$host_id" "$launch_template_id" "$version"
}

bibites_require_safe_host_change_set() {
  local description="$1"
  jq -e '
    .Status == "CREATE_COMPLETE" and
    if .ChangeSetType == "CREATE" then
      ([.Changes[]?.ResourceChange | .LogicalResourceId] | sort) ==
        (["DataVolume", "Host", "HostLaunchTemplate", "HostProfile",
          "HostRole", "HostSecurityGroup"] | sort) and
      all(.Changes[]?.ResourceChange;
        .Action == "Add" and
        (.Replacement // "False") == "False" and
        ((.LogicalResourceId == "DataVolume" and .ResourceType == "AWS::EC2::Volume") or
         (.LogicalResourceId == "Host" and .ResourceType == "AWS::EC2::Instance") or
         (.LogicalResourceId == "HostLaunchTemplate" and
          .ResourceType == "AWS::EC2::LaunchTemplate") or
         (.LogicalResourceId == "HostProfile" and
          .ResourceType == "AWS::IAM::InstanceProfile") or
         (.LogicalResourceId == "HostRole" and .ResourceType == "AWS::IAM::Role") or
         (.LogicalResourceId == "HostSecurityGroup" and
          .ResourceType == "AWS::EC2::SecurityGroup")))
    elif .ChangeSetType == "UPDATE" then
      (.Changes | length) == 1 and
      .Changes[0].ResourceChange.LogicalResourceId == "HostLaunchTemplate" and
      .Changes[0].ResourceChange.ResourceType == "AWS::EC2::LaunchTemplate" and
      .Changes[0].ResourceChange.Action == "Modify" and
      (.Changes[0].ResourceChange.Replacement // "False") == "False"
    else false
    end
  ' <<<"$description" >/dev/null || {
    bibites_validation_error \
      'host deployment allows only an initial stack create or one dormant launch-template update; Host, DataAttachment, DataVolume, live IAM or network, and unrelated resource changes are blocked'
    return 1
  }
}

bibites_wait_change_set() {
  local profile="$1" region="$2" stack="$3" change_set="$4"
  local timeout_seconds="${5:-600}" poll_seconds="${6:-3}"
  local deadline description status
  deadline=$(( $(date +%s) + timeout_seconds ))
  while (( $(date +%s) < deadline )); do
    description="$(aws --profile "$profile" --region "$region" cloudformation \
      describe-change-set --stack-name "$stack" --change-set-name "$change_set" \
      --include-property-values --output json)" || return 2
    status="$(jq -er '.Status | select(type == "string")' <<<"$description")" || return 2
    case "$status" in
      CREATE_COMPLETE)
        printf '%s\n' "$description"
        return 0
        ;;
      FAILED)
        printf '%s\n' "$description"
        return 1
        ;;
      CREATE_PENDING|CREATE_IN_PROGRESS) ;;
      *)
        bibites_validation_error "change set entered unexpected status $status"
        return 2
        ;;
    esac
    sleep "$poll_seconds"
  done
  bibites_validation_error \
    "change set $change_set did not finish in $timeout_seconds seconds"
  return 124
}

bibites_wait_stack_terminal() {
  local profile="$1" region="$2" stack="$3" timeout_seconds="${4:-3600}"
  local poll_seconds="${5:-15}" previous_marker="${6:-}"
  local deadline description status current_marker seen_progress=0
  deadline=$(( $(date +%s) + timeout_seconds ))
  while (( $(date +%s) < deadline )); do
    description="$(aws --profile "$profile" --region "$region" cloudformation \
      describe-stacks --stack-name "$stack" --output json)" || return 2
    status="$(jq -er '.Stacks[0].StackStatus | select(type == "string")' \
      <<<"$description")" || return 2
    current_marker="$(jq -er '
      .Stacks[0].LastUpdatedTime // .Stacks[0].CreationTime // ""
    ' <<<"$description")" || return 2
    case "$status" in
      CREATE_COMPLETE|UPDATE_COMPLETE)
        if [ "$seen_progress" -eq 1 ] || [ -z "$previous_marker" ] ||
           [ "$current_marker" != "$previous_marker" ]; then
          printf '%s\n' "$description"
          return 0
        fi
        ;;
      CREATE_FAILED|ROLLBACK_COMPLETE|ROLLBACK_FAILED|UPDATE_FAILED|UPDATE_ROLLBACK_COMPLETE|UPDATE_ROLLBACK_FAILED)
        if [ "$seen_progress" -eq 1 ] || [ -z "$previous_marker" ] ||
           [ "$current_marker" != "$previous_marker" ]; then
          printf '%s\n' "$description"
          return 1
        fi
        ;;
      REVIEW_IN_PROGRESS|*_IN_PROGRESS|*_CLEANUP_IN_PROGRESS) seen_progress=1 ;;
      *)
        bibites_validation_error "stack entered unexpected status $status"
        return 2
        ;;
    esac
    sleep "$poll_seconds"
  done
  bibites_validation_error "stack $stack did not finish in $timeout_seconds seconds"
  return 124
}
