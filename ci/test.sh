#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

if [ "$*" = "" ]; then
  set ./...
fi

CI=1 go test --timeout=30m "$@"
