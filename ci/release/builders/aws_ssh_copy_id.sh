#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--dry-run] [...args]

$0 runs ssh-copy-id on each builder.
args are passed to ssh-copy-id directly.
EOF
}

main() {
  while :; do
    flag_parse "$@"
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      dry-run)
        flag_noarg && shift "$FLAGSHIFT"
        DRY_RUN=1
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

  sh_c ssh-copy-id "$@" "\$TSTRUCT_LINUX_AMD64_BUILDER"
  sh_c ssh-copy-id "$@" "\$TSTRUCT_LINUX_ARM64_BUILDER"
  sh_c ssh-copy-id "$@" "\$TSTRUCT_MACOS_AMD64_BUILDER"
  sh_c ssh-copy-id "$@" "\$TSTRUCT_MACOS_ARM64_BUILDER"
}

main "$@"
