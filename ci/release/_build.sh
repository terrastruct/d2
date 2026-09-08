#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

sh_c rm -Rf "$HW_BUILD_DIR"
sh_c mkdir -p "$HW_BUILD_DIR"
sh_c cp ./ci/release/template/LICENSE.txt "$HW_BUILD_DIR"
sh_c cp ./THIRD_PARTY_NOTICES.txt "$HW_BUILD_DIR"
sh_c cp ./ci/release/template/Makefile "$HW_BUILD_DIR"
sh_c cp -R ./ci/release/template/man "$HW_BUILD_DIR"
sh_c cp -R ./ci/release/template/scripts "$HW_BUILD_DIR"
sh_c VERSION="$VERSION" ./ci/release/template/README.md.sh \> "'$HW_BUILD_DIR/README.md'"

ensure_goos
ensure_goarch
PINNED_GO_VERSION=go$(awk '$1 == "go" { print $2; exit }' go.mod)
ACTUAL_GO_VERSION=$(GOTOOLCHAIN=local go env GOVERSION)
if { [ -n "${RELEASE-}" ] || [ -n "${RELEASE_BUILD_IN_CI-}" ]; } &&
  [ "$ACTUAL_GO_VERSION" != "$PINNED_GO_VERSION" ]; then
  echo >&2 "release builds require $PINNED_GO_VERSION, got $ACTUAL_GO_VERSION"
  exit 1
fi
EXPECTED_GO_VERSION=$ACTUAL_GO_VERSION
REVISION=$(git rev-parse HEAD)
SOURCE_DATE_EPOCH=$(git show -s --format=%ct HEAD)
VCS_MODIFIED=false
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  VCS_MODIFIED=true
fi
if [ -n "${RELEASE-}" ] && [ "$VCS_MODIFIED" = true ]; then
  echo >&2 "release builds require a clean tracked worktree"
  exit 1
fi
if [ -z "${DRY_RUN-}" ] && { [ -z "${RELEASE_TOOL-}" ] || [ ! -x "$RELEASE_TOOL" ]; }; then
  echo >&2 "RELEASE_TOOL must name the host release-tool binary"
  exit 1
fi

GOTOOLCHAIN=local
GOWORK=off
GOFLAGS=
GOEXPERIMENT=
CGO_ENABLED=0
unset GOAMD64 GOARM64
case "$GOARCH" in
  amd64) GOAMD64=v1; export GOAMD64 ;;
  arm64) GOARM64=v8.0; export GOARM64 ;;
esac
export GOTOOLCHAIN GOWORK GOFLAGS GOEXPERIMENT CGO_ENABLED

sh_c mkdir -p "$HW_BUILD_DIR/bin"
BINARY="$HW_BUILD_DIR/bin/d2"
if [ "$GOOS" = windows ]; then BINARY=$BINARY.exe; fi
sh_c GOOS="$GOOS" GOARCH="$GOARCH" go build -mod=readonly -pgo=off \
  -buildvcs=true -trimpath \
  -ldflags "'-s -w -X github.com/d2lang/d2/lib/version.Version=$VERSION'" \
  -o "$BINARY" .

# Allow the bundled TALA engine and native exporters on all six release targets.
MAX_RELEASE_BINARY_BYTES=${MAX_RELEASE_BINARY_BYTES:-45000000}
MAX_RELEASE_ARCHIVE_BYTES=${MAX_RELEASE_ARCHIVE_BYTES:-19000000}
sh_c "$RELEASE_TOOL" verify-binary \
  --path "'$BINARY'" \
  --goos "$GOOS" \
  --goarch "$GOARCH" \
  --go-version "$EXPECTED_GO_VERSION" \
  --revision "$REVISION" \
  --vcs-modified "$VCS_MODIFIED" \
  --max-size "$MAX_RELEASE_BINARY_BYTES"

ARCHIVE=$PWD/$ARCHIVE
sh_c "$RELEASE_TOOL" archive \
  --root "'$HW_BUILD_DIR'" \
  --output "'$ARCHIVE'" \
  --epoch "$SOURCE_DATE_EPOCH"
sh_c "$RELEASE_TOOL" verify-archive \
  --path "'$ARCHIVE'" \
  --root "$(basename "$HW_BUILD_DIR")" \
  --epoch "$SOURCE_DATE_EPOCH" \
  --max-size "$MAX_RELEASE_ARCHIVE_BYTES"
