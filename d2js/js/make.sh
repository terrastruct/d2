#!/bin/sh
set -eu
if [ ! -e "$(dirname "$0")/../../ci/sub/.git" ]; then
  set -x
  git submodule update --init
  set +x
fi
. "$(dirname "$0")/../../ci/sub/lib.sh"
PATH="$(cd -- "$(dirname "$0")" && pwd)/../../ci/sub/bin:$PATH"
cd -- "$(dirname "$0")"

echo "DEBUG: d2js/js/make.sh received NPM_VERSION=${NPM_VERSION:-not set}"
if ! command -v bun >/dev/null 2>&1; then
  if [ -n "${CI-}" ]; then
    echo "Bun is not installed. Installing Bun..."
    curl -fsSL https://bun.sh/install | bash
    export PATH="$HOME/.bun/bin:$PATH"
  else
    echoerr "You need bun to build d2.js: curl -fsSL https://bun.sh/install | bash"
    exit 1
  fi
fi

_make "$@"
