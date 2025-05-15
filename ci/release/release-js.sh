#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. "./ci/sub/lib.sh"

VERSION=""

help() {
  cat <<EOF
usage: $0 --version=<version>

Publishes the d2.js to NPM.

Flags:
  --version     Version to publish (e.g., "0.1.2" or "nightly"). Note this is the js version, not related to the d2 version. A non-nightly version will publish to latest.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --help|-h)
      help
      exit 0
      ;;
    --version=*)
      VERSION="${arg#*=}"
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  flag_errusage "--version is required"
fi

FGCOLOR=6 header "Publishing JavaScript package to NPM (version: $VERSION)"

sh_c "NPM_VERSION=$VERSION ./make.sh js"

FGCOLOR=2 header 'NPM publish completed'
