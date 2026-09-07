#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. "./ci/sub/lib.sh"

VERSION=""
D2_REF=""

help() {
  cat <<EOF
usage: $0 --version=<version> [--d2-ref=<tag>]

Dispatches the protected npm staging workflow for both d2.js package identities.

Flags:
  --version     npm version to stage (e.g., "0.1.2" or "nightly"), independent of the D2 version. Stable packages use npm's latest tag after approval.
  --d2-ref      Exact D2 release tag to build (e.g., "v0.9.0"). It must be an ancestor of master. Omit to build the dispatched master commit.
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
    --d2-ref=*)
      D2_REF="${arg#*=}"
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
  --field "version=$VERSION" \
  --field "d2_ref=$D2_REF"

FGCOLOR=2 header 'npm staging workflow dispatched; approve both package stages after every job succeeds'
