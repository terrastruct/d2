#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--rebuild] [--local] [--dryrun]

$0 builds D2 release archives into ./ci/release/build/<version>/d2-<version>.tar.gz

The version is detected via git describe which will use the git tag for the current
commit if available.

Flags:

--rebuild: By default build.sh will avoid rebuilding finished assets if they
           already exist but if you changed something and need to force rebuild, use
           this flag.
--local:   By default build.sh uses \$TSTRUCT_MACOS_AMD64_BUILDER,
           \$TSTRUCT_MACOS_ARM64_BUILDER, \$TSTRUCT_LINUX_AMD64_BUILDER and
           \$TSTRUCT_LINUX_ARM64_BUILDER to build the release archives. It's required for
           now due to the following issue: https://github.com/terrastruct/d2/issues/31
           With --local, build.sh will cross compile locally.
           warning: This is only for testing purposes, do not use in production!
EOF
}

main() {
  VERSION="$(git_describe_ref)"
  BUILD_DIR="ci/release/build/$VERSION"
  while :; do
    flag_parse "$@"
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      rebuild)
        flag_noarg
        REBUILD=1
        shift "$FLAGSHIFT"
        ;;
      local)
        flag_noarg
        LOCAL=1
        shift "$FLAGSHIFT"
        ;;
      dryrun)
        flag_noarg
        DRYRUN=1
        shift "$FLAGSHIFT"
        ;;
      run)
        flag_reqarg
        JOB_FILTER="$FLAGARG"
        shift "$FLAGSHIFT"
      '')
        shift "$FLAGSHIFT"
        break
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done

  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  runjob linux-amd64 'OS=linux ARCH=amd64 build' &
  runjob linux-arm64 'OS=linux ARCH=arm64 build' &
  runjob macos-amd64 'OS=macos ARCH=amd64 build' &
  runjob macos-arm64 'OS=macos ARCH=arm64 build' &
  waitjobs
}

build() {
  HW_BUILD_DIR="$BUILD_DIR/$OS/$ARCH/d2-$VERSION"
  ARCHIVE="$BUILD_DIR/d2-$OS-$ARCH-$VERSION.tar.gz"

  if [ -e "$ARCHIVE" -a -z "${REBUILD-}" ]; then
    log "skipping as already built at $ARCHIVE"
    return 0
  fi

  if [ -n "${LOCAL-}" ]; then
    build_local
    return 0
  fi

  case $OS in
    # macos)
    #   ;;
    linux)
      case $ARCH in
        amd64)
          RHOST=$TSTRUCT_LINUX_AMD64_BUILDER build_rhost
          ;;
        arm64)
          RHOST=$TSTRUCT_LINUX_ARM64_BUILDER build_rhost
          ;;
        *)
          COLOR=3 logp warn "no builder for OS=$OS, building locally..."
          build_local
          ;;
      esac
      ;;
    *)
      COLOR=3 logp warn "no builder for OS=$OS, building locally..."
      build_local
      ;;
  esac
}

build_local() {
  export DRYRUN \
    HW_BUILD_DIR \
    VERSION \
    OS \
    ARCH \
    ARCHIVE
  sh_c ./ci/release/_build.sh
}

build_rhost() {
  sh_c ssh "$RHOST" mkdir -p src
  sh_c rsync --archive --human-readable --delete ./ "$RHOST:src/d2/"
  sh_c ssh -tttt "$RHOST" "DRYRUN=${DRYRUN-} \
HW_BUILD_DIR=$HW_BUILD_DIR \
VERSION=$VERSION \
OS=$OS \
ARCH=$ARCH \
ARCHIVE=$ARCHIVE \
./src/d2/ci/release/build_docker.sh"
  sh_c rsync --archive --human-readable "$RHOST:src/d2/$ARCHIVE" "$ARCHIVE"
}

main "$@"
