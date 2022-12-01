#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

sh_c mkdir -p "$HW_BUILD_DIR"
sh_c rsync --recursive --perms --delete \
  --human-readable --copy-links ./ci/release/template/ "$HW_BUILD_DIR/"
VERSION=$VERSION sh_c eval "'$HW_BUILD_DIR/README.md.sh'" \> "'$HW_BUILD_DIR/README.md'"
sh_c rm -f "$HW_BUILD_DIR/README.md.sh"
sh_c find "$HW_BUILD_DIR" -exec touch {} \\\;

export GOOS=$(goos "$OS")
export GOARCH="$ARCH"
sh_c mkdir -p "$HW_BUILD_DIR/bin"
sh_c go build -ldflags "'-X oss.terrastruct.com/d2/lib/version.Version=$VERSION'" \
  -o "$HW_BUILD_DIR/bin/d2" .

ARCHIVE=$PWD/$ARCHIVE
cd "$(dirname "$HW_BUILD_DIR")"
sh_c tar -czf "$ARCHIVE" "$(basename "$HW_BUILD_DIR")"
cd ->/dev/null
