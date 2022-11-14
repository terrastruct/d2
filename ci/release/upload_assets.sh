#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 <version>

Uploads the assets for release <version> to GitHub.

For example, if <version> is v0.0.99 then it uploads files matching
./ci/release/build/v0.0.99/*.tar.gz to the GitHub release v0.0.99.

Example:
  $0 v0.0.99
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
      '')
        shift "$FLAGSHIFT"
        break
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done

  if [ $# -ne 1 ]; then
    flag_errusage "first argument must be release version like v0.0.99"
  fi
  VERSION="$1"
  shift

  sh_c gh release upload --clobber "$VERSION" "./ci/release/build/$VERSION"/*.tar.gz
}

main "$@"
