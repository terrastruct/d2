#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--dry-run] [--skip-create]

$0 creates and ensures the d2 builders in AWS.
EOF
}

main() {
  while flag_parse "$@"; do
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      dry-run)
        flag_noarg && shift "$FLAGSHIFT"
        DRY_RUN=1
        ;;
      skip-create)
        flag_noarg && shift "$FLAGSHIFT"
        SKIP_CREATE=1
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done
  shift "$FLAGSHIFT"
  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  if [ -z "${SKIP_CREATE-}" ]; then
    create_remote_hosts
  fi
  init_remote_hosts
}

create_remote_hosts() {
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
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-linux-amd64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-071e6cafc48327ca2 \
      --count=1 \
      --instance-type=t2.small \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-linux-amd64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-linux-amd64}]"' >/dev/null
  fi
  while true; do
    dnsname=$(sh_c aws ec2 describe-instances \
      --filters 'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-linux-amd64' \
      | jq -r '.Reservations[].Instances[].PublicDnsName')
    if [ -n "$dnsname" ]; then
      log "TSTRUCT_LINUX_AMD64_BUILDER=admin@$dnsname"
      export TSTRUCT_LINUX_AMD64_BUILDER=admin@$dnsname
      break
    fi
    sleep 5
  done

  header linux-arm64
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-linux-arm64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0e67506f183e5ab60 \
      --count=1 \
      --instance-type=t4g.small \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-linux-arm64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-linux-arm64}]"' >/dev/null
  fi
  while true; do
    dnsname=$(sh_c aws ec2 describe-instances \
      --filters 'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-linux-arm64' \
      | jq -r '.Reservations[].Instances[].PublicDnsName')
    if [ -n "$dnsname" ]; then
      log "TSTRUCT_LINUX_ARM64_BUILDER=admin@$dnsname"
      export TSTRUCT_LINUX_ARM64_BUILDER=admin@$dnsname
      break
    fi
    sleep 5
  done

  header "macos-amd64-host"
  MACOS_AMD64_HOST_ID=$(aws ec2 describe-hosts --filter 'Name=state,Values=pending,available' 'Name=tag:Name,Values=d2-builder-macos-amd64' | jq -r '.Hosts[].HostId')
  if [ -z "$MACOS_AMD64_HOST_ID" ]; then
    MACOS_AMD64_HOST_ID=$(sh_c aws ec2 allocate-hosts --instance-type mac1.metal --quantity 1 --availability-zone us-west-2a \
      --tag-specifications '"ResourceType=dedicated-host,Tags=[{Key=Name,Value=d2-builder-macos-amd64}]"' \
      | jq -r .HostIds[0])
  fi

  header "macos-arm64-host"
  MACOS_ARM64_HOST_ID=$(aws ec2 describe-hosts --filter 'Name=state,Values=pending,available' 'Name=tag:Name,Values=d2-builder-macos-arm64' | jq -r '.Hosts[].HostId')
  if [ -z "$MACOS_ARM64_HOST_ID" ]; then
    MACOS_ARM64_HOST_ID=$(sh_c aws ec2 allocate-hosts --instance-type mac2.metal --quantity 1 --availability-zone us-west-2a \
      --tag-specifications '"ResourceType=dedicated-host,Tags=[{Key=Name,Value=d2-builder-macos-amd64}]"' \
      | jq -r .HostIds[0])
  fi

  header macos-amd64
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-macos-amd64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0dd2ded7568750663 \
      --count=1 \
      --instance-type=mac1.metal \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --placement "Tenancy=host,HostId=$MACOS_AMD64_HOST_ID" \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-macos-amd64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-macos-amd64}]"' >/dev/null
  fi
  while true; do
    dnsname=$(sh_c aws ec2 describe-instances \
      --filters 'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-macos-amd64' \
      | jq -r '.Reservations[].Instances[].PublicDnsName')
    if [ -n "$dnsname" ]; then
      log "TSTRUCT_MACOS_AMD64_BUILDER=ec2-user@$dnsname"
      export TSTRUCT_MACOS_AMD64_BUILDER=ec2-user@$dnsname
      break
    fi
    sleep 5
  done

  header macos-arm64
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-macos-arm64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0af0516ff2c43dbbe \
      --count=1 \
      --instance-type=mac2.metal \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --placement "Tenancy=host,HostId=$MACOS_ARM64_HOST_ID" \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=d2-builder-macos-arm64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=d2-builder-macos-arm64}]"' >/dev/null
  fi
  while true; do
    dnsname=$(sh_c aws ec2 describe-instances \
      --filters 'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=d2-builder-macos-arm64' \
      | jq -r '.Reservations[].Instances[].PublicDnsName')
    if [ -n "$dnsname" ]; then
      log "TSTRUCT_MACOS_ARM64_BUILDER=ec2-user@$dnsname"
      export TSTRUCT_MACOS_ARM64_BUILDER=ec2-user@$dnsname
      break
    fi
    sleep 5
  done
}

init_remote_hosts() {
  header linux-amd64
  REMOTE_HOST=$TSTRUCT_LINUX_AMD64_BUILDER init_remote_linux
  header linux-arm64
  REMOTE_HOST=$TSTRUCT_LINUX_ARM64_BUILDER init_remote_linux
  header macos-amd64
  REMOTE_HOST=$TSTRUCT_MACOS_AMD64_BUILDER init_remote_macos
  header macos-arm64
  REMOTE_HOST=$TSTRUCT_MACOS_ARM64_BUILDER init_remote_macos

  FGCOLOR=2 header summary
  log "export TSTRUCT_LINUX_AMD64_BUILDER=$TSTRUCT_LINUX_AMD64_BUILDER"
  log "export TSTRUCT_LINUX_ARM64_BUILDER=$TSTRUCT_LINUX_ARM64_BUILDER"
  log "export TSTRUCT_MACOS_AMD64_BUILDER=$TSTRUCT_MACOS_AMD64_BUILDER"
  log "export TSTRUCT_MACOS_ARM64_BUILDER=$TSTRUCT_MACOS_ARM64_BUILDER"
}

init_remote_linux() {
  while true; do
    if sh_c ssh "$REMOTE_HOST" :; then
      break
    fi
    sleep 5
  done

  sh_c ssh "$REMOTE_HOST" sh -s -- <<EOF
set -eux
export DEBIAN_FRONTEND=noninteractive

sudo -E apt-get update -y
sudo -E apt-get dist-upgrade -y
sudo -E apt-get install -y build-essential rsync

# Docker from https://docs.docker.com/engine/install/debian/
sudo -E apt-get -y install \
    ca-certificates \
    curl \
    gnupg \
    lsb-release
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --batch --yes --dearmor -o /etc/apt/keyrings/docker.gpg
echo \
  "deb [arch=\$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
  \$(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo -E apt-get update -y
sudo -E apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo groupadd docker || true
sudo usermod -aG docker \$USER
EOF

  sh_c ssh "$REMOTE_HOST" 'sudo reboot' || true
}

init_remote_macos() {
  while true; do
    if sh_c ssh "$REMOTE_HOST" :; then
      break
    fi
    sleep 5
  done
  sh_c ssh "$REMOTE_HOST" '"/bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""'
  sh_c ssh "$REMOTE_HOST" 'PATH="/usr/local/bin:/opt/homebrew/bin:\$PATH" brew update'
  sh_c ssh "$REMOTE_HOST" 'PATH="/usr/local/bin:/opt/homebrew/bin:\$PATH" brew upgrade'
  sh_c ssh "$REMOTE_HOST" 'PATH="/usr/local/bin:/opt/homebrew/bin:\$PATH" brew install go rsync'
}

main "$@"
