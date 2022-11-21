#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./ci/sub/lib.sh

./ci/release/gen_install.sh
./ci/release/gen_template_lib.sh

if [ -n "${CI-}" ]; then
  git_assert_clean
fi
