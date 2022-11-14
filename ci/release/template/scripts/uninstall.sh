#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./scripts/lib.sh

main() {
  if [ ! -e "${PREFIX-}" ]; then
    echoerr "\$PREFIX must be set to a unix prefix directory from which to uninstall d2 like /usr/local"
    return 1
  fi

  sh_c rm -f "$PREFIX/bin/d2"
  sh_c rm -f "$PREFIX/share/man/man1/d2.1"
}

main "$@"
