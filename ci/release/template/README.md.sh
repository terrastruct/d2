#!/bin/sh
set -eu

cat <<EOF
# d2

version: $VERSION

## Install

```
PREFIX=/usr/local ./install.sh
```
EOF
