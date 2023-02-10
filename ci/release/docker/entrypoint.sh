#!/bin/sh
set -eu

eval "$(fixuid -q)"

exec dumb-init /usr/local/bin/d2 "$@"
