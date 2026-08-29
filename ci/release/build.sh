#!/bin/sh
set -eu
if [ ! -e "$(dirname "$0")/../../ci/sub/.git" ]; then
  set -x
  git submodule update --init
  set +x
fi
. "$(dirname "$0")/../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/../.."
. ./ci/release/assets.sh

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

  VERSION=${VERSION:-$(git_describe_ref)}
  BUILD_DIR=ci/release/build/$VERSION
  ensure_release_tool
  sh_c mkdir -p "$BUILD_DIR"
  sh_c rm -f ci/release/build/latest
  sh_c ln -s "$VERSION" ci/release/build/latest
  if [ -n "${RELEASE-}" ] && [ -z "${RELEASE_BUILD_IN_CI-}" ]; then
    download_ci_build
    return
  fi
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
    runjob linux/amd64 'OS=linux ARCH=amd64 build' &
    runjob linux/arm64 'OS=linux ARCH=arm64 build' &
    runjob macos/amd64 'OS=macos ARCH=amd64 build' &
    runjob macos/arm64 'OS=macos ARCH=arm64 build' &
    runjob windows/amd64 'OS=windows ARCH=amd64 build' &
    runjob windows/arm64 'OS=windows ARCH=arm64 build' &
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

download_ci_build() {
  if [ -n "${REBUILD-}" ]; then
    echo >&2 "release workflow artifacts are immutable and cannot be rebuilt locally"
    echo >&2 "rerun failed jobs in the existing Release archives workflow instead"
    return 1
  fi
  if [ -n "${DRY_RUN-}" ]; then
    sh_c gh run download RELEASE_RUN_ID --name release-archives --dir "'$BUILD_DIR'"
    return
  fi

  REPOSITORY=$(gh_repo)
  TAG_COMMIT=$(git rev-parse "$VERSION^{commit}")
  RUN_ID=
  RUN_URL=
  ATTEMPT=1
  while [ "$ATTEMPT" -le 360 ]; do
    RUNS=$(gh run list \
      --repo "$REPOSITORY" \
      --workflow release-archives.yml \
      --commit "$TAG_COMMIT" \
      --event push \
      --limit 20 \
      --json databaseId,status,conclusion,headBranch,headSha,createdAt,url)
    RUN=$(printf '%s' "$RUNS" | jq -c \
      --arg version "$VERSION" \
      --arg commit "$TAG_COMMIT" \
      '[.[] | select(.headBranch == $version and .headSha == $commit)] |
       sort_by(.createdAt) | last // empty')
    if [ -n "$RUN" ]; then
      RUN_ID=$(printf '%s' "$RUN" | jq -r .databaseId)
      RUN_URL=$(printf '%s' "$RUN" | jq -r .url)
      RUN_STATUS=$(printf '%s' "$RUN" | jq -r .status)
      RUN_CONCLUSION=$(printf '%s' "$RUN" | jq -r '.conclusion // ""')
      if [ "$RUN_STATUS" = completed ]; then
        if [ "$RUN_CONCLUSION" != success ]; then
          echo >&2 "Release archives workflow failed: $RUN_URL"
          echo >&2 "rerun its failed jobs, then rerun the release command"
          return 1
        fi
        break
      fi
    fi
    if [ "$ATTEMPT" -eq 360 ]; then
      echo >&2 "timed out waiting for the Release archives workflow for $VERSION"
      echo >&2 "last matching workflow: ${RUN_URL:-none}"
      return 1
    fi
    sleep 10
    ATTEMPT=$((ATTEMPT + 1))
  done

  DOWNLOAD_DIR=$(mktempd)/release-archives
  sh_c mkdir -p "$DOWNLOAD_DIR"
  sh_c gh run download "$RUN_ID" \
    --repo "$REPOSITORY" \
    --name release-archives \
    --dir "'$DOWNLOAD_DIR'"
  "$RELEASE_TOOL" verify-checksums \
    --manifest "$DOWNLOAD_DIR/SHA256SUMS" \
    --directory "$DOWNLOAD_DIR" \
    --expected-count 6
  verify_ci_release_asset_directory "$DOWNLOAD_DIR" "$VERSION"
  verify_ci_release_asset_directory "$BUILD_DIR" "$VERSION" 0
  for TARGET in \
    linux-amd64 linux-arm64 \
    macos-amd64 macos-arm64 \
    windows-amd64 windows-arm64; do
    sh_c rm -f "$BUILD_DIR/d2-$VERSION-$TARGET.tar.gz"
    sh_c cp "$DOWNLOAD_DIR/d2-$VERSION-$TARGET.tar.gz" "$BUILD_DIR/"
  done
  sh_c rm -f "$BUILD_DIR/SHA256SUMS"
  sh_c cp "$DOWNLOAD_DIR/SHA256SUMS" "$BUILD_DIR/"
  verify_ci_release_asset_directory "$BUILD_DIR" "$VERSION"
  log "downloaded verified release archives from $RUN_URL"
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
