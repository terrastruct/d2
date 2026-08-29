#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. "./ci/sub/lib.sh"

VERSION=""

help() {
  cat <<EOF
usage: $0 --version=<version>

Dispatches the protected npm staging workflow for both d2.js package identities.

Flags:
  --version     Version to stage (e.g., "0.1.2" or "nightly"). Note this is the js version, not related to the d2 version. A non-nightly version will use the latest tag after approval.
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

if ! command -v gh >/dev/null 2>&1; then
  echoerr 'gh is required to dispatch the npm staging workflow'
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echoerr 'gh must be authenticated before dispatching the npm staging workflow'
  exit 1
fi

FGCOLOR=6 header "Staging JavaScript packages for npm review (version: $VERSION)"

gh workflow run npm-stage.yml \
  --repo d2lang/d2 \
  --ref master \
  --field "version=$VERSION"

FGCOLOR=2 header 'npm staging workflow dispatched; approve both package stages after every job succeeds'
