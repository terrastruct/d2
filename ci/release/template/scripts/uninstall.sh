#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./scripts/lib.sh

main() {
  ensure_prefix_sh_c

  ensure_os
  if [ "$OS" = windows ]; then
    "$sh_c" rm -f "$PREFIX/bin/d2.exe"
  else
    "$sh_c" rm -f "$PREFIX/bin/d2"
  fi

  "$sh_c" rm -f "$PREFIX/share/man/man1/d2.1"
}

main "$@"
