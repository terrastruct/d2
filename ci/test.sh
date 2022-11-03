#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

if [ "$*" = "" ]; then
  set ./...
fi

if [ "${CI:-}" ]; then
  export FORCE_COLOR=1
fi
go test --timeout=30m "$@"
