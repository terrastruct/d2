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

if ! go version | grep -qF '1.18'; then
  echoerr "You need go 1.18 to build d2."
  exit 1
fi

_make "$@"
