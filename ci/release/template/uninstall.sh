#!/bin/sh
set -eu
. "$(dirname "$0")/lib.sh"

main() {
  if [ ! -e "${PREFIX:-}" ]; then
    echoerr "\$PREFIX must be set to a unix prefix directory from which to uninstall d2 like /usr/local"
    exit 1
  fi

  sh_c rm -f "$PREFIX/bin/d2"
}

main "$@"
