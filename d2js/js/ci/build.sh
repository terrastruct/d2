#!/bin/sh
set -eu
. "$(dirname "$0")/../../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/.."

cd ../..
sh_c "GOOS=js GOARCH=wasm go build -ldflags='-s -w' -trimpath -o main.wasm ./d2js"
sh_c "mv main.wasm ./d2js/js/wasm/d2.wasm"

if [ ! -f ./d2js/js/wasm/d2.wasm ]; then
  echoerr "Error: d2.wasm is missing"
  exit 1
else
  echo "d2.wasm exists. Size:"
  ls -lh ./d2js/js/wasm/d2.wasm | awk '{print $5}'
fi

cd d2js/js
sh_c bun build.js

if [ "${PUBLISH:-0}" = "1" ]; then
  echo "Publishing nightly version to NPM..."

  DATE_TAG=$(date +'%Y%m%d')
  COMMIT_SHORT=$(git rev-parse --short HEAD)
  CURRENT_VERSION=$(node -p "require('./package.json').version")
  NIGHTLY_VERSION="${CURRENT_VERSION}-nightly.${DATE_TAG}.${COMMIT_SHORT}"

  cp package.json package.json.bak
  trap 'rm -f .npmrc; mv package.json.bak package.json' EXIT

  echo "Updating package version to ${NIGHTLY_VERSION}"
  npm version "${NIGHTLY_VERSION}" --no-git-tag-version

  echo "Publishing to npm with tag 'nightly'..."
  if [ -n "${NPM_TOKEN-}" ]; then
    echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > .npmrc
    trap 'rm -f .npmrc' EXIT
    if npm publish --tag nightly; then
      echo "Successfully published @terrastruct/d2@${NIGHTLY_VERSION} to npm with tag 'nightly'"
    else
      echoerr "Failed to publish package to npm"
      exit 1
    fi
  else
    echoerr "NPM_TOKEN environment variable is required for publishing to npm"
    exit 1
  fi
fi
