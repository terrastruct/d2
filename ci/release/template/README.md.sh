#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

ensure_os
ensure_arch
cat <<EOF
# d2

For docs, more installation options and the source code see https://oss.terrastruct.com/d2

version: $VERSION
os: $OS
arch: $ARCH

Built with $(go version | grep -o 'go[^ ]\+').
EOF

if [ "$OS" = windows ]; then
  cat <<EOF

We currently do not have an \`.msi\` for automatic installation on Windows so this release
is structured the same as our Unix releases.

Easiest way to use \`d2\` on Windows is to just \`chdir\` into the bin directory of this release
and invoke \`d2\` like \`./d2 <full-input-file-path>\`

For installation you'll have to add the \`./bin/d2.exe\` binary to your \`\$PATH\`. Or add
the \`./bin\` directory of this release to your \`\$PATH\`.

See https://www.wikihow.com/Change-the-PATH-Environment-Variable-on-Windows

Then you'll be able to call \`d2\` from the commandline in \`cmd.exe\` or \`pwsh.exe\`.

We intend to have a \`.msi\` release installer sometime soon that handles putting \`d2.exe\` into
your \`\$PATH\` for you.

You can also use \`make install\` to install on Windows after first installing
[MSYS2](https://www.msys2.org/) which emulates a Linux shell for Windows. Its terminal
also enables \`d2\` to show colors in its output. The manpage will also become accessible
with \`man d2\`.

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
