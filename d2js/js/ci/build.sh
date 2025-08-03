#!/bin/sh
set -eu
. "$(dirname "$0")/../../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/.."

cd ../..
sh_c "GOOS=js GOARCH=wasm go build -ldflags='-s -w' -gcflags='-l=4' -trimpath -o main.wasm ./d2js"
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

if [ -n "${NPM_VERSION:-}" ]; then
  cp package.json package.json.bak
  trap 'rm -f .npmrc; mv package.json.bak package.json' EXIT

  if [ "$NPM_VERSION" = "nightly" ]; then
    echo "Publishing nightly version to npm..."

    DATE_TAG=$(date +'%Y%m%d')
    COMMIT_SHORT=$(git rev-parse --short HEAD)
    CURRENT_VERSION=$(node -p "require('./package.json').version")
    PUBLISH_VERSION="${CURRENT_VERSION}-nightly.${DATE_TAG}.${COMMIT_SHORT}"
    NPM_TAG="nightly"

    echo "Updating package version to ${PUBLISH_VERSION}"
  else
    echo "Publishing official version ${NPM_VERSION} to npm..."
    PUBLISH_VERSION="$NPM_VERSION"
    NPM_TAG="latest"

    echo "Setting package version to ${PUBLISH_VERSION}"
  fi

  # Update package.json with the new version
  npm version "${PUBLISH_VERSION}" --no-git-tag-version

  echo "Publishing to npm with tag '${NPM_TAG}'..."
  if [ -n "${NPM_TOKEN-}" ]; then
    # Create .npmrc file with auth token
    echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > .npmrc

    if npm publish --tag "$NPM_TAG"; then
      echo "Successfully published @terrastruct/d2@${PUBLISH_VERSION} to npm with tag '${NPM_TAG}'"

      # For official releases, bump the patch version
      if [ "$NPM_VERSION" != "nightly" ]; then
        # Restore original package.json first
        mv package.json.bak package.json

        echo "Bumping version to ${NPM_VERSION}"
        npm version "${NPM_VERSION}" --no-git-tag-version
        git add package.json
        git commit -m "Bump version to ${NPM_VERSION} [skip ci]"

        # Cancel the trap since we manually restored and don't want it to execute on exit
        trap - EXIT
      fi
    else
      echoerr "Failed to publish package to npm"
      exit 1
    fi
  else
    echoerr "NPM_TOKEN environment variable is required for publishing to npm"
    exit 1
  fi
fi
