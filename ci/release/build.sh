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
  Use to run only the OS/ARCH jobs that match the given regex. e.g. --run=linux only runs
  the linux jobs. --run=linux/amd64 only runs the linux-amd64 job.

--version vX.X.X
  Use to overwrite the version detected from git.

--lockfile-force
  Forcefully take ownership of remote builder lockfiles.

--install
  Ensure a release using --host-only and install it.

--uninstall
  Ensure a release using --host-only and uninstall it.

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
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done
  shift "$FLAGSHIFT"
  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  if [ -n "${RELEASE-}" ] && [ -z "${RELEASE_BUILD_IN_CI-}" ]; then
    echo >&2 "production archives are built and uploaded by the Release archives workflow"
    echo >&2 "use ./ci/release/release.sh to prepare a release, then follow its Actions run"
    return 1
  fi

  VERSION=${VERSION:-$(git_describe_ref)}
  BUILD_DIR=ci/release/build/$VERSION
  ensure_release_tool
  sh_c mkdir -p "$BUILD_DIR"
  sh_c rm -f ci/release/build/latest
  sh_c ln -s "$VERSION" ci/release/build/latest
  if [ -n "${HOST_ONLY-}" ]; then
    ensure_os
    ensure_arch
    runjob "$OS/$ARCH" "build"
    write_checksums

    if [ -n "${INSTALL-}" ]; then
      sh_c make -sC "ci/release/build/$VERSION/$OS-$ARCH/d2-$VERSION" install
    elif [ -n "${UNINSTALL-}" ]; then
      sh_c make -sC "ci/release/build/$VERSION/$OS-$ARCH/d2-$VERSION" uninstall
    fi
    return 0
  fi

  if [ -n "${RELEASE_BUILD_SERIAL-}" ]; then
    runjob linux/amd64 'OS=linux ARCH=amd64 build'
    runjob linux/arm64 'OS=linux ARCH=arm64 build'
    runjob macos/amd64 'OS=macos ARCH=amd64 build'
    runjob macos/arm64 'OS=macos ARCH=arm64 build'
    runjob windows/amd64 'OS=windows ARCH=amd64 build'
    runjob windows/arm64 'OS=windows ARCH=arm64 build'
  else
    runjob_bg linux/amd64 'OS=linux ARCH=amd64 build'
    runjob_bg linux/arm64 'OS=linux ARCH=arm64 build'
    runjob_bg macos/amd64 'OS=macos ARCH=amd64 build'
    runjob_bg macos/arm64 'OS=macos ARCH=arm64 build'
    runjob_bg windows/amd64 'OS=windows ARCH=amd64 build'
    runjob_bg windows/arm64 'OS=windows ARCH=arm64 build'
    waitjobs
  fi
  write_checksums
}

ensure_release_tool() {
  PINNED_GO_VERSION=go$(awk '$1 == "go" { print $2; exit }' go.mod)
  ACTUAL_GO_VERSION=$(GOTOOLCHAIN=local go env GOVERSION)
  if { [ -n "${RELEASE-}" ] || [ -n "${RELEASE_BUILD_IN_CI-}" ]; } &&
    [ "$ACTUAL_GO_VERSION" != "$PINNED_GO_VERSION" ]; then
    echo >&2 "release builds require $PINNED_GO_VERSION, got $ACTUAL_GO_VERSION"
    return 1
  fi
  RELEASE_TOOL=$(mktempd)/release-tool
  export RELEASE_TOOL
  sh_c GOTOOLCHAIN=local GOWORK=off GOFLAGS= GOEXPERIMENT= \
    go build -mod=readonly -pgo=off -trimpath \
    -o "'$RELEASE_TOOL'" ./ci/release/releasetool
}

write_checksums() {
  if [ -n "${DRY_RUN-}" ]; then
    sh_c "$RELEASE_TOOL" checksums \
      --output "'$BUILD_DIR/SHA256SUMS'" \
      "'$BUILD_DIR/d2-$VERSION-<os>-<arch>.tar.gz'"
    return
  fi
  set -- "$BUILD_DIR"/d2-"$VERSION"-*.tar.gz
  if [ ! -e "$1" ]; then
    echo >&2 "no release archives were built for $VERSION"
    return 1
  fi
  EXPECTED_COUNT=$#
  if [ -n "${RELEASE_BUILD_IN_CI-}" ] && [ "$EXPECTED_COUNT" -ne 6 ]; then
    echo >&2 "CI release builds require six archives, got $EXPECTED_COUNT"
    return 1
  fi
  sh_c "$RELEASE_TOOL" checksums \
    --output "'$BUILD_DIR/SHA256SUMS'" \
    "$@"
  sh_c "$RELEASE_TOOL" verify-checksums \
    --manifest "'$BUILD_DIR/SHA256SUMS'" \
    --directory "'$BUILD_DIR'" \
    --expected-count "$EXPECTED_COUNT"
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

main "$@"
