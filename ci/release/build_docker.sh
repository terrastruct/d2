#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

tag="$(sh_c docker build \
  -qf ./ci/release/builders/Dockerfile ./ci/release/builders )"
sh_c docker run -it --rm \
  -v "$HOME:$HOME" \
  -u "$(id -u):$(id -g)" \
  -w "$HOME" \
  -e DRYRUN="${DRYRUN-}" \
  -e HW_BUILD_DIR="$HW_BUILD_DIR" \
  -e VERSION="$VERSION" \
  -e OS="$OS" \
  -e ARCH="$ARCH" \
  -e ARCHIVE="$ARCHIVE" \
  -e TERM="$TERM" \
  "$tag" ./src/d2/ci/release/_build.sh
