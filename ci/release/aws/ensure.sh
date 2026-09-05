#!/bin/sh
set -eu
. "$(dirname "$0")/../../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/../../.."

help() {
  cat <<EOF
usage: $0 [--dry-run] [--skip-create] [--skip-init] [--copy-id=id.pub]
          [--run=jobregex]

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
      x)
        flag_noarg && shift "$FLAGSHIFT"
        set -x
        export TRACE=1
        ;;
      dry-run)
        flag_noarg && shift "$FLAGSHIFT"
        export DRY_RUN=1
        ;;
      copy-id)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        ID_PUB_PATH=$FLAGARG
        ;;
      run)
        flag_reqarg && shift "$FLAGSHIFT"
        JOBFILTER="$FLAGARG"
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
  if [ -z "${ID_PUB_PATH-}" ]; then
    flag_errusage "--copy-id is required"
  fi

  JOBNAME=create runjob_filter create_remote_hosts
  JOBNAME=init runjob_filter init_remote_hosts

  FGCOLOR=2 header summary
  echo "export CI_D2_LINUX_AMD64=$CI_D2_LINUX_AMD64"
  echo "export CI_D2_LINUX_ARM64=$CI_D2_LINUX_ARM64"
  echo "export CI_D2_MACOS_AMD64=$CI_D2_MACOS_AMD64"
  echo "export CI_D2_MACOS_ARM64=$CI_D2_MACOS_ARM64"
}

create_remote_hosts() {
  bigheader create_remote_hosts

  KEY_NAME=$(aws ec2 describe-key-pairs | jq -r .KeyPairs[0].KeyName)
  VPC_ID=$(aws ec2 describe-vpcs | jq -r .Vpcs[0].VpcId)

  JOBNAME=$JOBNAME/security-groups runjob_filter create_security_groups
  JOBNAME=$JOBNAME/linux/amd64 runjob_filter create_linux_amd64
  JOBNAME=$JOBNAME/linux/arm64 runjob_filter create_linux_arm64
  JOBNAME=$JOBNAME/macos/amd64 runjob_filter create_macos_amd64
  JOBNAME=$JOBNAME/macos/arm64 runjob_filter create_macos_arm64
}

create_security_groups() {
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

}

create_linux_amd64() {
  header linux-amd64
  REMOTE_NAME=ci-d2-linux-amd64
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=ci-d2-linux-amd64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0ecc74eca1d66d8a6 \
      --count=1 \
      --instance-type=t3.small \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --iam-instance-profile 'Name=AmazonSSMRoleForInstancesQuickSetup' \
      --block-device-mappings '"DeviceName=/dev/sda1,Ebs={VolumeSize=64,VolumeType=gp3}"' \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=ci-d2-linux-amd64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=ci-d2-linux-amd64}]"' >/dev/null
  fi
  wait_remote_host_ip
  log "CI_D2_LINUX_AMD64=ubuntu@$ip"
  export CI_D2_LINUX_AMD64=ubuntu@$ip
}

create_linux_arm64() {
  header linux-arm64
  REMOTE_NAME=ci-d2-linux-arm64
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=ci-d2-linux-arm64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-06e2dea2cdda3acda \
      --count=1 \
      --instance-type=t4g.small \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --iam-instance-profile 'Name=AmazonSSMRoleForInstancesQuickSetup' \
      --block-device-mappings '"DeviceName=/dev/sda1,Ebs={VolumeSize=64,VolumeType=gp3}"' \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=ci-d2-linux-arm64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=ci-d2-linux-arm64}]"' >/dev/null
  fi
  wait_remote_host_ip
  log "CI_D2_LINUX_ARM64=ubuntu@$ip"
  export CI_D2_LINUX_ARM64=ubuntu@$ip
}

create_macos_amd64() {
  header macos-amd64-host
  MACOS_AMD64_ID=$(aws ec2 describe-hosts --filter 'Name=state,Values=pending,available' 'Name=tag:Name,Values=ci-d2-macos-amd64' | jq -r '.Hosts[].HostId')
  if [ -z "$MACOS_AMD64_ID" ]; then
    MACOS_AMD64_ID=$(sh_c aws ec2 allocate-hosts --instance-type mac1.metal --quantity 1 --availability-zone us-west-2a \
      --tag-specifications '"ResourceType=dedicated-host,Tags=[{Key=Name,Value=ci-d2-macos-amd64}]"' \
      | jq -r .HostIds[0])
  fi

  header macos-amd64
  REMOTE_NAME=ci-d2-macos-amd64
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=ci-d2-macos-amd64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0dd2ded7568750663 \
      --count=1 \
      --instance-type=mac1.metal \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --iam-instance-profile 'Name=AmazonSSMRoleForInstancesQuickSetup' \
      --placement "Tenancy=host,HostId=$MACOS_AMD64_ID" \
      --block-device-mappings '"DeviceName=/dev/sda1,Ebs={VolumeSize=100,VolumeType=gp3}"' \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=ci-d2-macos-amd64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=ci-d2-macos-amd64}]"' >/dev/null
  fi
  wait_remote_host_ip
  log "CI_D2_MACOS_AMD64=ec2-user@$ip"
  export CI_D2_MACOS_AMD64=ec2-user@$ip
}

create_macos_arm64() {
  header macos-arm64-host
  MACOS_ARM64_ID=$(aws ec2 describe-hosts --filter 'Name=state,Values=pending,available' 'Name=tag:Name,Values=ci-d2-macos-arm64' | jq -r '.Hosts[].HostId')
  if [ -z "$MACOS_ARM64_ID" ]; then
    MACOS_ARM64_ID=$(sh_c aws ec2 allocate-hosts --instance-type mac2.metal --quantity 1 --availability-zone us-west-2a \
      --tag-specifications '"ResourceType=dedicated-host,Tags=[{Key=Name,Value=ci-d2-macos-arm64}]"' \
      | jq -r .HostIds[0])
  fi

  header macos-arm64
  REMOTE_NAME=ci-d2-macos-arm64
  state=$(aws ec2 describe-instances --filters \
    'Name=instance-state-name,Values=pending,running,stopping,stopped' 'Name=tag:Name,Values=ci-d2-macos-arm64' \
    | jq -r '.Reservations[].Instances[].State.Name')
  if [ -z "$state" ]; then
    sh_c aws ec2 run-instances \
      --image-id=ami-0af0516ff2c43dbbe \
      --count=1 \
      --instance-type=mac2.metal \
      --security-groups=ssh \
      "--key-name=$KEY_NAME" \
      --iam-instance-profile 'Name=AmazonSSMRoleForInstancesQuickSetup' \
      --placement "Tenancy=host,HostId=$MACOS_ARM64_ID" \
      --block-device-mappings '"DeviceName=/dev/sda1,Ebs={VolumeSize=100,VolumeType=gp3}"' \
      --tag-specifications '"ResourceType=instance,Tags=[{Key=Name,Value=ci-d2-macos-arm64}]"' \
        '"ResourceType=volume,Tags=[{Key=Name,Value=ci-d2-macos-arm64}]"' >/dev/null
  fi
  wait_remote_host_ip
  log "CI_D2_MACOS_ARM64=ec2-user@$ip"
  export CI_D2_MACOS_ARM64=ec2-user@$ip
}

wait_remote_host_ip() {
  while true; do
    ip=$(sh_c aws ec2 describe-instances \
      --filters 'Name=instance-state-name,Values=pending,running,stopping,stopped' "Name=tag:Name,Values=$REMOTE_NAME" \
      | jq -r '.Reservations[].Instances[].PublicIpAddress')
    if [ -n "$ip" ]; then
      alloc_static_ip
      ip=$(sh_c aws ec2 describe-instances \
        --filters 'Name=instance-state-name,Values=pending,running,stopping,stopped' "Name=tag:Name,Values=$REMOTE_NAME" \
        | jq -r '.Reservations[].Instances[].PublicIpAddress')
      ssh-keygen -R "$ip"
      break
    fi
    sleep 5
  done
}

alloc_static_ip() {
  allocation_id=$(aws ec2 describe-addresses --filters "Name=tag:Name,Values=$REMOTE_NAME" | jq -r '.Addresses[].AllocationId')
  if [ -z "$allocation_id" ]; then
    sh_c aws ec2 allocate-address --tag-specifications "'ResourceType=elastic-ip,Tags=[{Key=Name,Value=$REMOTE_NAME}]'"
    allocation_id=$(aws ec2 describe-addresses --filters "Name=tag:Name,Values=$REMOTE_NAME" | jq -r '.Addresses[].AllocationId')
  fi

  instance_id=$(aws ec2 describe-instances \
    --filters 'Name=instance-state-name,Values=pending,running,stopping,stopped' "Name=tag:Name,Values=$REMOTE_NAME" \
    | jq -r '.Reservations[].Instances[].InstanceId')
  aws ec2 associate-address --instance-id "$instance_id" --allocation-id "$allocation_id"
}

init_remote_hosts() {
  bigheader init_remote_hosts

  JOBNAME=$JOBNAME/linux/amd64 runjob_filter REMOTE_HOST=$CI_D2_LINUX_AMD64 REMOTE_NAME=ci-d2-linux-amd64 init_remote_linux
  JOBNAME=$JOBNAME/linux/arm64 runjob_filter REMOTE_HOST=$CI_D2_LINUX_ARM64 REMOTE_NAME=ci-d2-linux-arm64 init_remote_linux
  JOBNAME=$JOBNAME/macos/amd64 runjob_filter REMOTE_HOST=$CI_D2_MACOS_AMD64 REMOTE_NAME=ci-d2-macos-amd64 init_remote_macos
  JOBNAME=$JOBNAME/macos/arm64 runjob_filter REMOTE_HOST=$CI_D2_MACOS_ARM64 REMOTE_NAME=ci-d2-macos-arm64 init_remote_macos
}

init_remote_linux() {
  header "$REMOTE_NAME"
  wait_remote_host

  sh_c ssh_copy_id -i="$ID_PUB_PATH" "$REMOTE_HOST"

  sh_c ssh "$REMOTE_HOST" sh -s -- <<EOF
set -eux
export DEBIAN_FRONTEND=noninteractive

sudo -E apt-get update -y
sudo -E apt-get dist-upgrade -y
sudo -E apt-get update -y
sudo -E apt-get install -y build-essential ca-certificates curl rsync

mkdir -p \$HOME/.local/bin
mkdir -p \$HOME/.local/share/man
EOF
  init_remote_env

  sh_c ssh "$REMOTE_HOST" sh -s -- <<EOF
set -eux
export DEBIAN_FRONTEND=noninteractive
sudo -E apt-get autoremove -y
EOF
  sh_c ssh "$REMOTE_HOST" 'sudo reboot' || true
}

init_remote_macos() {
  header "$REMOTE_NAME"
  wait_remote_host

  sh_c ssh_copy_id -i="$ID_PUB_PATH" "$REMOTE_HOST"

  sh_c ssh "$REMOTE_HOST" '"/bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""'

  if sh_c ssh "$REMOTE_HOST" uname -m | grep -qF arm64; then
    shellenv=$(sh_c ssh "$REMOTE_HOST" /opt/homebrew/bin/brew shellenv)
  else
    shellenv=$(sh_c ssh "$REMOTE_HOST" /usr/local/bin/brew shellenv)
  fi
  if ! echo "$shellenv" | sh_c ssh "$REMOTE_HOST" "IFS= read -r regex\; \"grep -qF \\\"\\\$regex\\\" ~/.zshrc\""; then
    echo "$shellenv" | sh_c ssh "$REMOTE_HOST" "\"(echo && cat) >> ~/.zshrc\""
  fi
  if ! sh_c ssh "$REMOTE_HOST" "'grep -qF \\\$HOME/.local ~/.zshrc'"; then
    sh_c ssh "$REMOTE_HOST" "\"(echo && cat) >> ~/.zshrc\"" <<EOF
PATH=\$HOME/.local/bin:\$PATH
MANPATH=\$HOME/.local/share/man:\$MANPATH
EOF
  fi
  init_remote_env
  sh_c ssh "$REMOTE_HOST" brew update
  sh_c ssh "$REMOTE_HOST" brew upgrade
  sh_c ssh "$REMOTE_HOST" brew install go rsync

  sh_c ssh "$REMOTE_HOST" 'sudo reboot' || true
}

init_remote_env() {
  sh_c ssh "$REMOTE_HOST" '"rm -f ~/.ssh/environment"'
  sh_c ssh "$REMOTE_HOST" '"echo PATH=\$(echo \"echo \\\$PATH\" | \"\$SHELL\" -ils) >\$HOME/.ssh/environment"'
  sh_c ssh "$REMOTE_HOST" '"echo MANPATH=\$(echo \"echo \\\$MANPATH\" | \"\$SHELL\" -ils) >>\$HOME/.ssh/environment"'

  sh_c ssh "$REMOTE_HOST" "sudo sed -i.bak '\"s/#PermitUserEnvironment no/PermitUserEnvironment yes/\"' /etc/ssh/sshd_config"

  if sh_c ssh "$REMOTE_HOST" uname | grep -qF Darwin; then
    sh_c ssh "$REMOTE_HOST" "sudo launchctl stop com.openssh.sshd"
  else
    sh_c ssh "$REMOTE_HOST" "sudo systemctl restart sshd"
    # ubuntu has $PATH hard coded in /etc/environment for some reason. It takes precedence
    # over ~/.ssh/environment.
    sh_c ssh "$REMOTE_HOST" "sudo rm -f /etc/environment"
  fi
}

wait_remote_host() {
  while true; do
    if sh_c ssh "$REMOTE_HOST" true; then
      break
    fi
    sleep 5
  done
}

main "$@"
