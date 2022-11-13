#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--dryrun]

$0 inits the D2 builders by installing docker.
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

  init_aws
}

init_aws() {
  header linux-amd64
  # RHOST=$TSTRUCT_LINUX_AMD64_BUILDER init_rhost
  header linux-arm64
  RHOST=$TSTRUCT_LINUX_ARM64_BUILDER init_rhost
}

init_rhost() {
  sh_c ssh "$RHOST" 'sudo yum upgrade -y'
  sh_c ssh "$RHOST" 'sudo yum install -y docker'
  sh_c ssh "$RHOST" 'sudo systemctl start docker'
  sh_c ssh "$RHOST" 'sudo systemctl enable docker'
  sh_c ssh "$RHOST" 'sudo usermod -a -G docker ec2-user'
  sh_c ssh "$RHOST" 'sudo reboot' || true
}

main "$@"
