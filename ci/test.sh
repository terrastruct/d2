#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

if [ "$*" = "" ]; then
	set ./...
fi

go test -tags plugins_embed,plugins_embed_dagre,plugins_embed_elk --timeout=30m "$@"
