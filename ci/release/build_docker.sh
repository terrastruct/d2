#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

tag="$(sh_c docker build \
  --build-arg GOVERSION="1.19.3.linux-$ARCH" \
  -qf ./ci/release/builders/Dockerfile ./ci/release/builders)"
docker_run \
  -e DRY_RUN \
  -e HW_BUILD_DIR \
  -e VERSION \
  -e OS \
  -e ARCH \
  -e ARCHIVE \
  "$tag" ./src/d2/ci/release/_build.sh
