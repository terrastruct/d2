#!/bin/sh
set -eu
. "$(dirname "$0")/lib.sh"

main() {
  if [ ! -e "${PREFIX:-}" ]; then
    echoerr "\$PREFIX must be set to a unix prefix directory in which to install d2 like /usr/local"
    exit 1
  fi

  mkdir -p "$PREFIX"

  sh_c install ./bin/d2 "$PREFIX/bin/d2"
}

main "$@"
