#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."

./ci/sub/release/release.sh "$@"
