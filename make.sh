#!/bin/sh
set -eu
if [ ! -d "$(dirname "$0")/ci/sub/.git" ]; then
  git submodule update --init
fi
. "$(dirname "$0")/ci/sub/lib.sh"
PATH="$(cd -- "$(dirname "$0")" && pwd)/ci/sub/bin:$PATH"
cd "$(dirname "$0")"

_make "$@"
