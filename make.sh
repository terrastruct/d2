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

if ! go version | grep -q '1.2[0-9]'; then
  echoerr "You need go 1.2x to build d2."
  exit 1
fi

# if [ "${CI:-}" ]; then
#   export FORCE_COLOR=1
#   npx playwright@1.31.1 install --with-deps chromium
# fi
_make "$@"
