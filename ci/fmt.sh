#!/bin/sh
set -eu
. "$(dirname "$0")/sub/lib.sh"
cd -- "$(dirname "$0")/.."

if is_changed README.md; then
  sh_c tocsubst --skip 1 README.md
fi
if is_changed docs/INSTALL.md; then
  sh_c tocsubst --skip 1 docs/INSTALL.md
fi
./ci/sub/fmt/make.sh
