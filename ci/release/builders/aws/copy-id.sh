#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--dry-run] -i keys.pub

$0 copies keys.pub to each builder and then deduplicates its .authorized_keys.
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
      i)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        KEY_FILE=$FLAGARG
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done
  shift "$FLAGSHIFT"
  if [ -z "${KEY_FILE-}" ]; then
    echoerr "-i is required"
    exit 1
  fi

  header linux-amd64
  REMOTE_HOST=$TSTRUCT_LINUX_AMD64_BUILDER copy_keys
  header linux-arm64
  REMOTE_HOST=$TSTRUCT_LINUX_ARM64_BUILDER copy_keys
  header macos-amd64
  REMOTE_HOST=$TSTRUCT_MACOS_AMD64_BUILDER copy_keys
  header macos-arm64
  REMOTE_HOST=$TSTRUCT_MACOS_ARM64_BUILDER copy_keys
}

copy_keys() {
  sh_c ssh-copy-id -fi "$KEY_FILE" "$REMOTE_HOST"
  sh_c ssh "$REMOTE_HOST" 'cat .ssh/authorized_keys \| sort -u \> .ssh/authorized_keys.dedup'
  sh_c ssh "$REMOTE_HOST" 'cp .ssh/authorized_keys.dedup .ssh/authorized_keys'
  sh_c ssh "$REMOTE_HOST" 'rm .ssh/authorized_keys.dedup'
}

main "$@"
