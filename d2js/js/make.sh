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

BUN_VERSION=$(awk -F'"' '$2 == "bun" { print $4 }' package.json)
BUN_INSTALLER_COMMIT=0d9b296af33f2b851fcbf4df3e9ec89751734ba4
if [ -z "$BUN_VERSION" ]; then
  echoerr "Unable to read the Bun version from package.json"
  exit 1
fi

if ! command -v bun >/dev/null 2>&1 || [ "$(bun --version)" != "$BUN_VERSION" ]; then
  if [ -n "${CI-}" ]; then
    echo "Installing Bun ${BUN_VERSION}..."
    curl -fsSL \
      "https://raw.githubusercontent.com/oven-sh/bun/${BUN_INSTALLER_COMMIT}/src/cli/install.sh" |
      bash -s "bun-v${BUN_VERSION}"
    export PATH="${BUN_INSTALL:-$HOME/.bun}/bin:$PATH"
  else
    echoerr "You need Bun ${BUN_VERSION} to build d2.js"
    exit 1
  fi
fi

if ! command -v bun >/dev/null 2>&1 || [ "$(bun --version)" != "$BUN_VERSION" ]; then
  echoerr "Expected Bun ${BUN_VERSION} after installation"
  exit 1
fi

# Go sources outside this directory also affect the WASM package. Invoke make
# directly so _make's directory-based change filter cannot skip these checks.
export CI_MAKE_ROOT=0
make -sj8 "$@"
ci_waitjobs
