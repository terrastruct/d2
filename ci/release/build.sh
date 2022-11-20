#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--rebuild] [--local] [--dry-run] [--run=regex] [--host-only] [--lockfile-force]

$0 builds D2 release archives into ./ci/release/build/<version>/d2-<VERSION>-<OS>-<ARCH>.tar.gz

The version is detected via git describe which will use the git tag for the current
commit if available.

Flags:

--rebuild
  By default build.sh will avoid rebuilding finished assets if they already exist but if you
  changed something and need to force rebuild, use this flag.

--local
  By default build.sh uses \$TSTRUCT_MACOS_AMD64_BUILDER, \$TSTRUCT_MACOS_ARM64_BUILDER,
  \$TSTRUCT_LINUX_AMD64_BUILDER and \$TSTRUCT_LINUX_ARM64_BUILDER to build the release
  archives. It's required for now due to the following issue:
  https://github.com/terrastruct/d2/issues/31 With --local, build.sh will cross compile
  locally. warning: This is only for testing purposes, do not use in production!

--host-only
  Use to build the release archive for the host OS-ARCH only. All logging is done to stderr
  so in a script you can read from stdout to get the path to the release archive.

--run=regex
  Use to run only the OS-ARCH jobs that match the given regex. e.g. --run=linux only runs
  the linux jobs. --run=linux-amd64 only runs the linux-amd64 job.

--version vX.X.X
  Use to overwrite the version detected from git.

--lockfile-force
  Forcefully take ownership of remote builder lockfiles.
EOF
}

main() {
  while flag_parse "$@"; do
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      rebuild)
        flag_noarg && shift "$FLAGSHIFT"
        REBUILD=1
        ;;
      local)
        flag_noarg && shift "$FLAGSHIFT"
        LOCAL=1
        ;;
      dry-run)
        flag_noarg && shift "$FLAGSHIFT"
        DRY_RUN=1
        ;;
      run)
        flag_reqarg && shift "$FLAGSHIFT"
        JOBFILTER="$FLAGARG"
        ;;
      host-only)
        flag_noarg && shift "$FLAGSHIFT"
        HOST_ONLY=1
        LOCAL=1
        ;;
      version)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        VERSION=$FLAGARG
        ;;
      lockfile-force)
        flag_noarg && shift "$FLAGSHIFT"
        LOCKFILE_FORCE=1
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done
  shift "$FLAGSHIFT"
  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  VERSION=${VERSION:-$(git_describe_ref)}
  BUILD_DIR=ci/release/build/$VERSION
  if [ -n "${HOST_ONLY-}" ]; then
    runjob $(os)-$(arch) "OS=$(os) ARCH=$(arch) build" &
    waitjobs
    return 0
  fi

  runjob linux-amd64 'OS=linux ARCH=amd64 build' &
  runjob linux-arm64 'OS=linux ARCH=arm64 build' &
  runjob macos-amd64 'OS=macos ARCH=amd64 build' &
  runjob macos-arm64 'OS=macos ARCH=arm64 build' &
  waitjobs
}

build() {
  HW_BUILD_DIR="$BUILD_DIR/$OS-$ARCH/d2-$VERSION"
  ARCHIVE="$BUILD_DIR/d2-$VERSION-$OS-$ARCH.tar.gz"

  if [ -e "$ARCHIVE" -a -z "${REBUILD-}" ]; then
    log "skipping as already built at $ARCHIVE"
    return 0
  fi

  if [ -n "${LOCAL-}" ]; then
    build_local
    return 0
  fi

  case $OS in
    macos)
      case $ARCH in
        amd64)
          REMOTE_HOST=$TSTRUCT_MACOS_AMD64_BUILDER build_remote_macos
          ;;
        arm64)
          REMOTE_HOST=$TSTRUCT_MACOS_ARM64_BUILDER build_remote_macos
          ;;
        *)
          warn "no builder for OS=$OS ARCH=$ARCH, building locally..."
          build_local
          ;;
      esac
      ;;
    linux)
      case $ARCH in
        amd64)
          REMOTE_HOST=$TSTRUCT_LINUX_AMD64_BUILDER build_remote_linux
          ;;
        arm64)
          REMOTE_HOST=$TSTRUCT_LINUX_ARM64_BUILDER build_remote_linux
          ;;
        *)
          warn "no builder for OS=$OS ARCH=$ARCH, building locally..."
          build_local
          ;;
      esac
      ;;
    *)
      warn "no builder for OS=$OS, building locally..."
      build_local
      ;;
  esac
}

build_local() {
  export DRY_RUN \
    HW_BUILD_DIR \
    VERSION \
    OS \
    ARCH \
    ARCHIVE
  sh_c ./ci/release/_build.sh
}

build_remote_macos() {
  sh_c lockfile_ssh "$REMOTE_HOST" .d2-build-lock
  trap unlockfile_ssh EXIT
  sh_c ssh "$REMOTE_HOST" mkdir -p src
  sh_c rsync --archive --human-readable --delete ./ "$REMOTE_HOST:src/d2/"
  sh_c ssh "$REMOTE_HOST" "COLOR=${COLOR-} \
TERM=${TERM-} \
DRY_RUN=${DRY_RUN-} \
HW_BUILD_DIR=$HW_BUILD_DIR \
VERSION=$VERSION \
OS=$OS \
ARCH=$ARCH \
ARCHIVE=$ARCHIVE \
PATH=\\\"/usr/local/bin:/usr/local/sbin:/opt/homebrew/bin:/opt/homebrew/sbin\\\${PATH+:\\\$PATH}\\\" \
./src/d2/ci/release/_build.sh"
  sh_c mkdir -p "$HW_BUILD_DIR"
  sh_c rsync --archive --human-readable "$REMOTE_HOST:src/d2/$ARCHIVE" "$ARCHIVE"
}

build_remote_linux() {
  sh_c lockfile_ssh "$REMOTE_HOST" .d2-build-lock
  trap unlockfile_ssh EXIT
  sh_c ssh "$REMOTE_HOST" mkdir -p src
  sh_c rsync --archive --human-readable --delete ./ "$REMOTE_HOST:src/d2/"
  sh_c ssh "$REMOTE_HOST" "COLOR=${COLOR-} \
TERM=${TERM-} \
DRY_RUN=${DRY_RUN-} \
HW_BUILD_DIR=$HW_BUILD_DIR \
VERSION=$VERSION \
OS=$OS \
ARCH=$ARCH \
ARCHIVE=$ARCHIVE \
./src/d2/ci/release/build_docker.sh"
  sh_c mkdir -p "$HW_BUILD_DIR"
  sh_c rsync --archive --human-readable "$REMOTE_HOST:src/d2/$ARCHIVE" "$ARCHIVE"
}

ssh() {
  command ssh -o='StrictHostKeyChecking=accept-new' "$@"
}

main "$@"
