#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./ci/sub/lib.sh

sh_c go build --tags=dev -o=bin/d2 .
sh_c ./bin/d2 "$@"
