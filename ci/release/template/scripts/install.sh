#!/bin/sh
set -eu
cd -- "$(dirname "$0")/.."
. ./scripts/lib.sh

main() {
  if [ ! -e "${PREFIX-}" ]; then
    echoerr "\$PREFIX must be set to a unix prefix directory in which to install d2 like /usr/local"
    return 1
  fi

  sh_c mkdir -p "$PREFIX/bin"
  sh_c mkdir -p "$PREFIX/share/man/man1"
  sh_c install ./bin/d2 "$PREFIX/bin/d2"
  sh_c install ./man/d2.1 "$PREFIX/share/man/man1"
}

main "$@"
