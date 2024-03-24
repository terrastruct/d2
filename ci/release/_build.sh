#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

sh_c rm -Rf "$HW_BUILD_DIR"
sh_c mkdir -p "$HW_BUILD_DIR"
sh_c cp ./ci/release/template/LICENSE.txt "$HW_BUILD_DIR"
sh_c cp ./ci/release/template/Makefile "$HW_BUILD_DIR"
sh_c cp -R ./ci/release/template/man "$HW_BUILD_DIR"
sh_c cp -R ./ci/release/template/scripts "$HW_BUILD_DIR"
sh_c VERSION="$VERSION" ./ci/release/template/README.md.sh \> "'$HW_BUILD_DIR/README.md'"

ensure_goos
ensure_goarch
sh_c mkdir -p "$HW_BUILD_DIR/bin"
sh_c GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 go build -trimpath \
	-tags plugins_embed,plugins_embed_dagre,plugins_embed_elk \
	-ldflags "'-X oss.terrastruct.com/d2/lib/version.Version=$VERSION'" \
	-o "$HW_BUILD_DIR/bin/d2" .

if [ "$GOOS" = windows ]; then
	sh_c mv "$HW_BUILD_DIR/bin/d2" "$HW_BUILD_DIR/bin/d2.exe"
fi

ARCHIVE=$PWD/$ARCHIVE
cd "$(dirname "$HW_BUILD_DIR")"
sh_c tar -czf "$ARCHIVE" "$(basename "$HW_BUILD_DIR")"
cd - >/dev/null
