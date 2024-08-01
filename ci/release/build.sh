#!/bin/sh
set -eu
if [ ! -e "$(dirname "$0")/../../ci/sub/.git" ]; then
  set -x
  git submodule update --init
  set +x
fi
. "$(dirname "$0")/../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/../.."

help() {
  cat <<EOF
usage: $0 [--rebuild] [--dry-run] [--run=regex] [--host-only] [--lockfile-force]
          [--install] [--uninstall]

$0 builds D2 release archives into ./ci/release/build/<version>/d2-<VERSION>-<OS>-<ARCH>.tar.gz

The version is detected via git describe which will use the git tag for the current
commit if available.

Flags:

--rebuild
  By default build.sh will avoid rebuilding finished assets if they already exist but if you
  changed something and need to force rebuild, use this flag.

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

--install
  Ensure a release using --host-only and install it.

--uninstall
  Ensure a release using --host-only and uninstall it.

--push-docker
  Push the built docker image. Unfortunately dockerx requires the multi-arch images be
  pushed if required in the same invocation as build. dockerx cannot load multi-arch
  images into the daemon for push later. It's not slow though to use --push-docker after
  building the image as nearly all artifacts are cached.
  Automatically set if called from release.sh

--latest-docker
  Mark the built image with the latest tag. Automatically set if called from release.sh
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
      dry-run)
        flag_noarg && shift "$FLAGSHIFT"
        DRY_RUN=1
        ;;
      run)
        flag_reqarg && shift "$FLAGSHIFT"
        JOBFILTER=$FLAGARG
        ;;
      host-only)
        flag_noarg && shift "$FLAGSHIFT"
        HOST_ONLY=1
        ;;
      version)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        VERSION=$FLAGARG
        ;;
      lockfile-force)
        flag_noarg && shift "$FLAGSHIFT"
        LOCKFILE_FORCE=1
        ;;
      install)
        flag_noarg && shift "$FLAGSHIFT"
        INSTALL=1
        HOST_ONLY=1
        ;;
      uninstall)
        flag_noarg && shift "$FLAGSHIFT"
        UNINSTALL=1
        HOST_ONLY=1
        ;;
      push-docker)
        flag_noarg && shift "$FLAGSHIFT"
        PUSH_DOCKER=1
        ;;
      latest-docker)
        flag_noarg && shift "$FLAGSHIFT"
        LATEST_DOCKER=1
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
  sh_c mkdir -p "$BUILD_DIR"
  sh_c rm -f ci/release/build/latest
  sh_c ln -s "$VERSION" ci/release/build/latest
  if [ -n "${HOST_ONLY-}" ]; then
    ensure_os
    ensure_arch
    runjob "$OS/$ARCH" "build"

    if [ -n "${INSTALL-}" ]; then
      sh_c make -sC "ci/release/build/$VERSION/$OS-$ARCH/d2-$VERSION" install
    elif [ -n "${UNINSTALL-}" ]; then
      sh_c make -sC "ci/release/build/$VERSION/$OS-$ARCH/d2-$VERSION" uninstall
    fi
    return 0
  fi

  runjob linux/amd64 'OS=linux ARCH=amd64 build' &
  runjob linux/arm64 'OS=linux ARCH=arm64 build' &
  runjob macos/amd64 'OS=macos ARCH=amd64 build' &
  runjob macos/arm64 'OS=macos ARCH=arm64 build' &
  runjob windows/amd64 'OS=windows ARCH=amd64 build' &
  runjob windows/arm64 'OS=windows ARCH=arm64 build' &
  waitjobs

  runjob linux/docker build_docker &
  runjob windows/amd64/msi 'OS=windows ARCH=amd64 build_windows_msi' &
  waitjobs
}

build() {
  HW_BUILD_DIR="$BUILD_DIR/$OS-$ARCH/d2-$VERSION"
  ARCHIVE="$BUILD_DIR/d2-$VERSION-$OS-$ARCH.tar.gz"

  if [ -e "$ARCHIVE" -a -z "${REBUILD-}" ]; then
    log "skipping as already built at $ARCHIVE"
    return 0
  fi

  build_local
  return 0
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

build_docker() {
  sh_c ./ci/release/docker/build.sh \
    --version="$VERSION" \
    ${PUSH_DOCKER:+--push} \
    ${LATEST_DOCKER:+--latest}
  return 0
}

build_windows_msi() {
  REMOTE_HOST=$CI_D2_WINDOWS_AMD64

  ln -sf "../build/$VERSION/windows-amd64/d2-$VERSION/bin/d2.exe" ./ci/release/windows/d2.exe
  sh_c rsync --archive --human-readable --copy-links --delete ./ci/release/windows/ "'$REMOTE_HOST:windows/'"
  if ! echo "$VERSION" | grep '[0-9]\.[0-9]\.[0-9]'; then
    WIX_VERSION=0.0.0
  else
    WIX_VERSION=$VERSION
  fi
  sh_c ssh "$REMOTE_HOST" "'cd ./windows && wix build -arch x64 -d D2Version=$WIX_VERSION ./d2.wxs'"

  # --files-from shouldn't be necessary but for some reason selecting d2.msi directly
  # makes rsync error with:
  # ERROR: rejecting unrequested file-list name: ./windows/d2.msi
  # rsync error: requested action not supported (code 4) at flist.c(1027) [Receiver=3.2.7]
  rsync_files=$(mktempd)/rsync-files
  echo d2.msi >$rsync_files
  sh_c rsync --archive --human-readable --files-from "$rsync_files" "'$REMOTE_HOST:windows/'" "./ci/release/build/$VERSION/d2-$VERSION-$OS-$ARCH.msi"
}

main "$@"
