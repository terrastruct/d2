#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--rebuild] [--local]

$0 builds D2 release archives into ./ci/release/build/<ref>/d2-<ref>.tar.gz

Flags:

--rebuild: By default build.sh will avoid rebuilding finished assets if they
           already exist but if you changed something and need to force rebuild, use
           this flag.
--local:   By default build.sh uses \$TSTRUCT_MACOS_BUILDER and \$TSTRUCT_LINUX_BUILDER
           to build the release archives. It's required for now due to the following
           issue: https://github.com/terrastruct/d2/issues/31
           With --local, build.sh will cross compile locally.
           warning: This is only for testing purposes, do not use in production!
EOF
}

build() {
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
  unset FLAG \
    FLAGRAW \
    FLAGARG \
    FLAGSHIFT \
    VERSION \
    REBUILD \
    DEST  \
    LOCAL \
  VERSION="$(git_describe_ref)"
  BUILD_DIR="ci/release/build/$VERSION"

  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  runjob linux-amd64 'OS=linux ARCH=amd64 build' &
  runjob linux-arm64 'OS=linux ARCH=arm64 build' &
  runjob macos-amd64 'OS=macos ARCH=amd64 build' &
  runjob macos-arm64 'OS=macos ARCH=arm64 build' &
  wait_jobs
}

main "$@"
