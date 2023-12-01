#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./ci/sub/lib.sh

sh_c go build --tags=dev,plugins_embed,plugins_embed_dagre,plugins_embed_elk -o=bin/d2 .
sh_c ./bin/d2 "$@"
