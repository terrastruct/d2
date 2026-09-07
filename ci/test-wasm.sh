#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

GOOS=js GOARCH=wasm ./ci/test.sh \
  -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
  ./d2js/d2wasm ./d2layouts/d2elklayout "$@"
