#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

if [ "$*" = "" ]; then
  set ./...
fi

if [ "${CI:-}" ]; then
  export FORCE_COLOR=1
  npx playwright@1.31.1 install --with-deps chromium
fi
go test --timeout=30m "$@"
