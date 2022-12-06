#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./scripts/lib.sh

main() {
  ensure_prefix_sh_c
  "$sh_c" rm -f "$PREFIX/bin/d2"
  "$sh_c" rm -f "$PREFIX/share/man/man1/d2.1"
}

main "$@"
