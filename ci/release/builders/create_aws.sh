#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--dryrun]

$0 creates the d2 builders in a AWS account.
EOF
}

main() {
  unset DRYRUN 
  while :; do
    flag_parse "$@"
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      dryrun)
        flag_noarg
        DRYRUN=1
        shift "$FLAGSHIFT"
        ;;
      '')
        shift "$FLAGSHIFT"
        break
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done
  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  create_aws
}

create_aws() {
  KEY_NAME=$(aws ec2 describe-key-pairs | jq -r .KeyPairs[0].KeyName)
  VPC_ID=$(aws ec2 describe-vpcs | jq -r .Vpcs[0].VpcId)

  header security-group
  SG_ID=$(aws ec2 describe-security-groups --group-names ssh 2>/dev/null \
    | jq -r .SecurityGroups[0].GroupId)
  if [ -z "$SG_ID" ]; then
    SG_ID=$(sh_c aws ec2 create-security-group \
      --group-name ssh \
      --description ssh \
      --vpc-id "$VPC_ID" | jq -r .GroupId)
  fi

  header security-group-ingress
  SG_RULES_COUNT=$(aws ec2 describe-security-groups --group-names ssh \
    | jq -r '.SecurityGroups[0].IpPermissions | length')
  if [ "$SG_RULES_COUNT" -eq 0 ]; then
    sh_c aws ec2 authorize-security-group-ingress \
      --group-id "$SG_ID" \
      --protocol tcp \
      --port 22 \
      --cidr 0.0.0.0/0 >/dev/null
  fi

  header linux-amd64
  if ! aws ec2 describe-instances \
    "--query=Reservations[*].Instances[?State.Name!='terminated']" \
    | grep -q d2-builder-linux-amd64; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0d593311db5abb72b \
      --count=1 \
      --instance-type=t2.micro \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-linux-amd64}]' \
        'ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-linux-amd64}]' >/dev/null
  fi

  header linux-arm64
  if ! aws ec2 describe-instances \
    "--query=Reservations[*].Instances[?State.Name!='terminated']" \
    | grep -q d2-builder-linux-arm64; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0efabcf945ffd8831 \
      --count=1 \
      --instance-type=t4g.nano \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-linux-arm64}]' \
        'ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-linux-arm64}]' >/dev/null
  fi
}

main "$@"
