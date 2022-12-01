#!/bin/sh
set -eu
. "$(dirname "$0")/sub/lib.sh"
cd -- "$(dirname "$0")/.."

sh_c tocsubst --skip 1 README.md
./ci/sub/fmt/make.sh
