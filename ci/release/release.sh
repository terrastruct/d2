#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. "./ci/sub/lib.sh"

./ci/sub/release/release.sh "$@"
