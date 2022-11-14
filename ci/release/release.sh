#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."

./ci/sub/bin/release.sh "$@"
