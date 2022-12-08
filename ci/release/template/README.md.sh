#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

cat <<EOF
# d2

For docs, more installation options and the source code see https://oss.terrastruct.com/d2

version: $VERSION
os: $OS
arch: $ARCH

Built with $(go version | grep -o 'go[^ ]\+').
EOF

ensure_os
if [ "$OS" = windows ]; then
  cat <<EOF

We currently do not have a .msi for automatic installation on Windows so this release is
structured the same as our Unix releases.

Easiest way to use d2 on Windows is to just cd into the bin directory of this release
and invoke d2 like ./d2.exe <full-input-file-path>

You can install on Windows with [MSYS2](https://www.msys2.org/) which emulates a Linux
shell for Windows. It also enables d2 to show colors in its output.

But if you must install on Windows without MSYS2, for now you'll have to add the d2 binary
in \`./bin/d2.exe\` to your \`\$PATH\` manually. Or you can add the \`./bin\` directory to
your \`\$PATH\`.

See https://www.wikihow.com/Change-the-PATH-Environment-Variable-on-Windows
See https://github.com/terrastruct/d2/blob/master/docs/INSTALL.md#windows
EOF
fi

cat <<EOF

## Install

\`\`\`sh
make install DRY_RUN=1
# If it looks right, run:
# make install
\`\`\`

Pass \`PREFIX=somepath\` to change the installation prefix from the default which is
\`/usr/local\` or \`~/.local\` if \`/usr/local\` is not writable with the current user.

## Uninstall

\`\`\`sh
make uninstall DRY_RUN=1
# If it looks right, run:
# make uninstall
\`\`\`
EOF
