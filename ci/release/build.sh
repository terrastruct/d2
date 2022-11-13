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

build() {
  HW_BUILD_DIR="$BUILD_DIR/$OS/$ARCH/d2-$VERSION"
  ARCHIVE="$BUILD_DIR/d2-$OS-$ARCH-$VERSION.tar.gz"

  if [ -e "$ARCHIVE" -a -z "${REBUILD-}" ]; then
    log "skipping as already built at $ARCHIVE"
    return 0
  fi

  sh_c mkdir -p "$HW_BUILD_DIR"
  sh_c rsync --recursive --perms --delete \
    --human-readable --copy-links ./ci/release/template/ "$HW_BUILD_DIR/"
  VERSION=$VERSION sh_c eval "'$HW_BUILD_DIR/README.md.sh'" \> "'$HW_BUILD_DIR/README.md'"
  sh_c rm -f "$HW_BUILD_DIR/README.md.sh"
  sh_c find "$HW_BUILD_DIR" -exec touch {} \;

  export GOOS=$(goos "$OS")
  export GOARCH="$ARCH"
  sh_c mkdir -p "$HW_BUILD_DIR/bin"
  sh_c go build -ldflags "-X oss.terrastruct.com/d2/lib/version.Version=$VERSION" \
    -o "$HW_BUILD_DIR/bin/d2" ./cmd/d2

  sh_c tar czf "$ARCHIVE" "$HW_BUILD_DIR"
}

main() {
  unset FLAG \
    FLAGRAW \
    FLAGARG \
    FLAGSHIFT \
    VERSION \
    BUILD_DIR \
    HW_BUILD_DIR \
    REBUILD \
    LOCAL \
    DRYRUN \
    ARCHIVE
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

main "$@"
