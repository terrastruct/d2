#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. "./ci/sub/lib.sh"


NPM_VERSION=""

for arg in "$@"; do
  case "$arg" in
    --npm-version=*)
      NPM_VERSION="${arg#*=}"
      ;;
  esac
done

if [ -z "$NPM_VERSION" ]; then
  flag_errusage "--npm-version is required"
fi

./ci/sub/release/release.sh "$@"

if [ -n "$NPM_VERSION" ]; then
  ./ci/release/release-js.sh --version="$NPM_VERSION"
fi

