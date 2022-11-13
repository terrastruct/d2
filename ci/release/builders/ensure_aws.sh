#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--dryrun]

$0 creates and ensures the d2 builders in AWS.
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

  create_rhosts
  init_rhosts
}

create_rhosts() {
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
      --instance-type=t2.small \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-linux-amd64}]' \
        'ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-linux-amd64}]' >/dev/null
  fi
  while true; do
    dnsname=$(sh_c aws ec2 describe-instances \
      --filters 'Name=instance-state-name,Values=pending,running,shutting-down,stopping,stopped' 'Name=tag:Name,Values=d2-builder-linux-amd64' \
      | jq -r '.Reservations[].Instances[].PublicDnsName')
    if [ -n "$dnsname" ]; then
      log "TSTRUCT_LINUX_AMD64_BUILDER=ec2-user@$dnsname"
      export TSTRUCT_LINUX_AMD64_BUILDER=ec2-user@$dnsname
      break
    fi
    sleep 5
  done

  header linux-arm64
  if ! aws ec2 describe-instances \
    "--query=Reservations[*].Instances[?State.Name!='terminated']" \
    | grep -q d2-builder-linux-arm64; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0efabcf945ffd8831 \
      --count=1 \
      --instance-type=t4g.small \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-linux-arm64}]' \
        'ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-linux-arm64}]' >/dev/null
  fi
  while true; do
    dnsname=$(sh_c aws ec2 describe-instances \
      --filters 'Name=instance-state-name,Values=pending,running,shutting-down,stopping,stopped' 'Name=tag:Name,Values=d2-builder-linux-arm64' \
      | jq -r '.Reservations[].Instances[].PublicDnsName')
    if [ -n "$dnsname" ]; then
      log "TSTRUCT_LINUX_ARM64_BUILDER=ec2-user@$dnsname"
      export TSTRUCT_LINUX_ARM64_BUILDER=ec2-user@$dnsname
      break
    fi
    sleep 5
  done
}

init_rhosts() {
  header linux-amd64
  RHOST=$TSTRUCT_LINUX_AMD64_BUILDER init_rhost
  header linux-arm64
  RHOST=$TSTRUCT_LINUX_ARM64_BUILDER init_rhost
  log "export TSTRUCT_LINUX_AMD64_BUILDER=$TSTRUCT_LINUX_AMD64_BUILDER"
  log "export TSTRUCT_LINUX_ARM64_BUILDER=$TSTRUCT_LINUX_ARM64_BUILDER"
}

init_rhost() {
  while true; do
    if sh_c ssh "$RHOST" :; then
      break
    fi
    sleep 5
  done
  sh_c ssh "$RHOST" 'sudo yum upgrade -y'
  sh_c ssh "$RHOST" 'sudo yum install -y docker'
  sh_c ssh "$RHOST" 'sudo systemctl start docker'
  sh_c ssh "$RHOST" 'sudo systemctl enable docker'
  sh_c ssh "$RHOST" 'sudo usermod -a -G docker ec2-user'
  sh_c ssh "$RHOST" 'sudo reboot' || true
}

main "$@"
