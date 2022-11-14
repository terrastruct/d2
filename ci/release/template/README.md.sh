#!/bin/sh
set -eu

cat <<EOF
# d2

For docs, more installation options and the source code see https://oss.terrastruct.com/d2

version: $VERSION

## Install

\`\`\`sh
make install PREFIX=/usr/local DRY_RUN=1
# If it looks right, run:
make install PREFIX=/usr/local
\`\`\`

## Uninstall

\`\`\`sh
make uninstall PREFIX=/usr/local DRY_RUN=1
# If it looks right, run:
make uninstall PREFIX=/usr/local
\`\`\`
EOF
