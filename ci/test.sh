#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

if [ "$*" = "" ]; then
  set ./...
fi

go test --timeout=30m "$@"
