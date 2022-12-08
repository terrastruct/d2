#!/bin/sh
set -eu
. "$(dirname "$0")/sub/lib.sh"
cd -- "$(dirname "$0")/.."

./ci/sub/bin/fmt.sh
