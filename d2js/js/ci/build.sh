#!/bin/sh
set -eu
. "$(dirname "$0")/../../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/.."

cd ../..
if [ -n "${NPM_VERSION:-}" ]; then
  JS_VERSION="$NPM_VERSION"
else
  JS_VERSION=$(awk -F'"' '/"version"/ {print $4}' ./d2js/js/package.json)
fi
sh_c "GOOS=js GOARCH=wasm go build -ldflags='-s -w -X oss.terrastruct.com/d2/d2js/d2wasm.jsVersion=${JS_VERSION}' -trimpath -o main.wasm ./d2js"

if [ -n "${NPM_VERSION:-}" ]; then
  # Optimize with wasm-opt if available
  if command -v wasm-opt >/dev/null 2>&1; then
    echo "Optimizing WASM with wasm-opt..."
    sh_c "wasm-opt -Oz --enable-bulk-memory-opt main.wasm -o main.wasm"
  else
    echo "wasm-opt not found, skipping optimization (install with: brew install binaryen)"
  fi
fi

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
  PUBLISH_MODE="${NPM_PUBLISH_MODE:-publish}"

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

  case "$PUBLISH_MODE" in
    stage)
      echo "Staging package on npm with tag '${NPM_TAG}'..."
      if npm stage publish --tag "$NPM_TAG"; then
        echo "Successfully staged @terrastruct/d2@${PUBLISH_VERSION} on npm with tag '${NPM_TAG}'"
      else
        echoerr "Failed to stage package on npm"
        exit 1
      fi
      ;;
    publish)
      if [ -z "${NPM_TOKEN-}" ]; then
        echoerr "NPM_TOKEN environment variable is required for direct publishing"
        exit 1
      fi

      # Create .npmrc file with auth token
      echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > .npmrc

      echo "Publishing to npm with tag '${NPM_TAG}'..."
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
      ;;
    *)
      echoerr "Unknown NPM_PUBLISH_MODE: ${PUBLISH_MODE}"
      exit 1
      ;;
  esac
fi
