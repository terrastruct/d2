#!/bin/sh
set -eu
. "$(dirname "$0")/../../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/.."

cd ../..
sh_c "GOOS=js GOARCH=wasm go build -ldflags='-s -w' -trimpath -o main.wasm ./d2js"
sh_c "mv main.wasm ./d2js/js/wasm/d2.wasm"

if [ ! -f ./d2js/js/wasm/d2.wasm ]; then
  echoerr "Error: d2.wasm is missing"
  exit 1
else
  stat --printf="Size: %s bytes\n" ./d2js/js/wasm/d2.wasm || ls -lh ./d2js/js/wasm/d2.wasm
fi

cd d2js/js
sh_c bun run build
