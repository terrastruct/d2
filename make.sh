#!/bin/sh
set -eu

REQUIRED_GO_MINOR=$(sed -En 's/^go[[:space:]]+([[:digit:].]+)$/\1/p' go.mod)
ACTUAL_GO_VERSION=$(go version | sed -n 's/^go version go\([0-9]*\.[0-9]*\.[0-9]*\)\(.*\)/\1/p')

# We use 'case' instead of '[' to match values against complex patterns, because POSIX
# does not guarantee that '[' supports advanced features like globs and regex.
if case "$ACTUAL_GO_VERSION" in "$REQUIRED_GO_MINOR".*) false ;; *) true ;; esac then
  red="\e[0;91m"
  reset="\e[0m"

  printf "${red}PROBLEM: You need go %s to build d2, but you have %s installed.${reset}\n" "$REQUIRED_GO_MINOR" "$ACTUAL_GO_VERSION"
  exit 1
fi

if [ ! -e "$(dirname "$0")/ci/sub/.git" ]; then
  set -x
  git submodule update --init
  set +x
fi
. "$(dirname "$0")/ci/sub/lib.sh"
PATH="$(cd -- "$(dirname "$0")" && pwd)/ci/sub/bin:$PATH"
cd -- "$(dirname "$0")"

_make "$@"
