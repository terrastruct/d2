#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

build() {
  OS="$1"
  ARCH="$2"
  BUILD_DIR="$BUILD_DIR/$OS/$ARCH"

  mkdir -p "$BUILD_DIR/bin"
  sh_c cp LICENSE.txt "$BUILD_DIR"
  sh_c "./ci/release/template/README.md.sh > $BUILD_DIR"

  export GOOS=$(goos "$OS")
  export GOARCH="$ARCH"
  sh_c go build -ldflags "-X lib/version.Version=$VERSION" \
    -o "$BUILD_DIR/bin/d2" ./cmd/d2
}

main() {
  VERSION="$(git_describe_ref)"
  BUILD_DIR="ci/release/build/$VERSION"
  

  runjob linux-amd64 'OS=linux ARCH=amd64 build linux amd64' &
  runjob linux-arm64 'OS=linux ARCH=arm64 build linux arm64' &
  runjob macos-amd64 'OS=macos ARCH=amd64 build macos amd64' &
  runjob macos-arm64 'OS=macos ARCH=arm64 build macos arm64' &
  wait_jobs
}

main "$@"
