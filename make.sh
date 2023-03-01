#!/bin/sh
set -eu

if [ ! -e "$(dirname "$0")/ci/sub/.git" ]; then
  set -x
  git submodule update --init
  set +x
fi
. "$(dirname "$0")/ci/sub/lib.sh"
PATH="$(cd -- "$(dirname "$0")" && pwd)/ci/sub/bin:$PATH"
cd -- "$(dirname "$0")"

GO_VERSION=$(sed -En 's/^go[[:space:]]+([[:digit:].]+)$/\1/p' go.mod)

if ! $(go version | grep -qF "${GO_VERSION}"); then
  printferr "You need go %s to build d2.\n" "$GO_VERSION"
  exit 1
fi

_make "$@"
