#!/bin/sh
set -eu

cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib/log.sh
. ./ci/sub/lib/flag.sh
cd -

help() {
  arg0="$0"
  if [ "$0" = sh ]; then
    arg0="curl -fsSL https://d2lang.com/install.sh | sh -s --"
  fi

  cat <<EOF
usage: $arg0 [--dryrun] [--version vX.X.X] [--edge] [--method detect] [--prefix ~/.local]
  [--tala] [--tala-version vX.X.X] [--force] [--uninstall]

install.sh automates the installation of D2 onto your system. It currently only supports
the installation of standalone releases from GitHub. If you pass --edge, it will clone the
source, build a release and install from it.

Flags:

--dryrun
  Pass to have install.sh show the install method and flags that will be used to install
  without executing them. Very useful to understand what changes it will make to your system.

--version vX.X.X
  Pass to have install.sh install the given version instead of the latest version.

--edge
  Pass to build and install D2 from source.

--method [detect | standalone]
  Pass to control the method by which to install. Right now we only support standalone
  releases from GitHub but later we'll add support for brew, rpm, deb and more.

  - detect is currently unimplemented but would use your OS's package manager
    automatically.
  - standalone installs a standalone release archive into ~/.local
     Add ~/.local/bin to your \$PATH to use it.
     Control the unix hierarchy path with --prefix

--prefix ~/.local
  Controls the unix hierarchy path into which standalone releases are installed.
  Defaults to ~/.local. You may also want to use /usr/local
  Remember that whatever you use, you must have the bin directory of your prefix
  path in \$PATH to execute the d2 binary. For example, if my prefix directory is
  /usr/local then my \$PATH must contain /usr/local/bin.

--tala
  Install Terrastruct's closed source TALA for improved layouts.
  See https://github.com/terrastruct/TALA

--tala-version vX.X.X
  Install the passed version of tala instead of latest.

--force:
  Force installation over the existing version even if they match. It will attempt a clean
  uninstall first before installing the new version. The install assets will not be deleted
  from ~/.cache/d2/install.

--uninstall:
  Uninstall the installed version of d2. The --method flag must be the same as for
  installation. i.e if you used --method standalone you must again use --method standalone
  for uninstallation. With detect, the install script will try to use the OS package manager
  to uninstall instead.

All downloaded assets are cached into ~/.cache/d2/install. use \$XDG_CACHE_HOME to change
path of the cached assets.

You can rerun install.sh to update your version of D2. install.sh will avoid reinstalling
if the installed version is the latest unless --force is passed.
EOF
}
