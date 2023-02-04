#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./ci/sub/lib.sh

main() {
  if [ "$*" = "" ]; then
    set ./...
  fi

  mkdir -p out
  capcode ./ci/test.sh -covermode=atomic -coverprofile=out/cov.prof "$@"
  go tool cover -html=out/cov.prof -o=out/cov.html
  go tool cover -func=out/cov.prof | grep '^total:' \
    | sed 's#^total:.*(statements)[[:space:]]*\([0-9.%]*\)#TOTAL:\t\1#'
  return "$code"
}

main "$@"
