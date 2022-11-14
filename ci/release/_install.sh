#!/bin/sh
set -eu

cd -- "$(dirname "$0")/../sub/lib"
. ./log.sh
. ./flag.sh
. ./release.sh
cd - >/dev/null

help() {
  arg0="$0"
  if [ "$0" = sh ]; then
    arg0="curl -fsSL https://d2lang.com/install.sh | sh -s --"
  fi

  cat <<EOF
usage: $arg0 [--dry-run] [--version vX.X.X] [--edge] [--method detect] [--prefix /usr/local]
  [--tala] [--tala-version vX.X.X] [--force] [--uninstall]

install.sh automates the installation of D2 onto your system. It currently only supports
the installation of standalone releases from GitHub. If you pass --edge, it will clone the
source, build a release and install from it.

Flags:

--dry-run
  Pass to have install.sh show the install method and flags that will be used to install
  without executing them. Very useful to understand what changes it will make to your system.

--version vX.X.X
  Pass to have install.sh install the given version instead of the latest version.

--edge
  Pass to build and install D2 from source. This will still use --method if set to detect
  to install the release archive for your OS, whether it's apt, yum, brew or standalone
  if an unsupported package manager is used. To install from source like a dev would,
  use go install oss.terrastruct.com/d2
  note: currently unimplemented.

--method [detect | standalone]
  Pass to control the method by which to install. Right now we only support standalone
  releases from GitHub but later we'll add support for brew, rpm, deb and more.
  note: currently unimplemented.

  - detect is currently unimplemented but would use your OS's package manager
    automatically.
  - standalone installs a standalone release archive into the unix hierarchy path
     specified by --prefix which defaults to /usr/local
     Ensure /usr/local/bin is in your \$PATH to use it.

--prefix /usr/local
  Controls the unix hierarchy path into which standalone releases are installed.
  Defaults to /usr/local. You may also want to use ~/.local to avoid needing sudo.
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

All downloaded assets are cached into ~/.cache/d2/release. use \$XDG_CACHE_HOME to change
path of the cached assets.

You can rerun install.sh to update your version of D2. install.sh will avoid reinstalling
if the installed version is the latest unless --force is passed.
EOF
}

main() {
  if [ -n "${DEBUG-}" ]; then
    set -x
  fi

  METHOD=standalone
  while :; do
    flag_parse "$@"
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      dry-run)
        flag_noarg && shift "$FLAGSHIFT"
        DRY_RUN=1
        ;;
      version)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        VERSION=$FLAGARG
        ;;
      tala-version)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        TALA_VERSION=$FLAGARG
        ;;
      edge)
        flag_noarg && shift "$FLAGSHIFT"
        EDGE=1
        echoerr "$FLAGRAW is currently unimplemented"
        exit 1
        ;;
      method)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        METHOD=$FLAGARG
        echoerr "$FLAGRAW is currently unimplemented"
        exit 1
        ;;
      prefix)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        export PREFIX=$FLAGARG
        ;;
      force)
        flag_noarg && shift "$FLAGSHIFT"
        FORCE=1
        ;;
      uninstall)
        flag_noarg && shift "$FLAGSHIFT"
        UNINSTALL=1
        ;;
      '')
        shift "$FLAGSHIFT"
        break
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done

  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  if [ -n "${UNINSTALL-}" ]; then
    uninstall
    return 1
  fi

  REPO=${REPO:-terrastruct/d2}
  PREFIX=${PREFIX:-/usr/local}
  OS=$(os)
  ARCH=$(arch)
  CACHE_DIR=$(cache_dir)
  mkdir -p "$CACHE_DIR"
  VERSION=${VERSION:-latest}
  if [ "$VERSION" = latest ]; then
    VERSION=$(fetch_version_info)
  fi

  if command -v d2 >/dev/null; then
    INSTALLED_VERSION="$(d2 version)"
    if [ ! "${FORCE-}" -a "$VERSION" = "$INSTALLED_VERSION" ]; then
      log "skipping installation as version $VERSION is already installed."
      return 0
    fi
    log "uninstalling $INSTALLED_VERSION to install $VERSION"
    # uninstall
  fi

  install_standalone
}

install_standalone() {
  ARCHIVE="d2-$VERSION-$OS-$ARCH.tar.gz"
  log "installing standalone release $ARCHIVE from github"

  VERSION=$(fetch_version_info)
  asset_line=$(cat "$CACHE_DIR/$VERSION.json" | grep -n "$ARCHIVE" | cut -d: -f1 | head -n1)
  asset_url=$(sed -n $((asset_line-3))p "$CACHE_DIR/$VERSION.json" | sed 's/^.*: "\(.*\)",$/\1/g')
  fetch_gh "$asset_url" "$CACHE_DIR/$ARCHIVE" 'application/octet-stream'

  sh_c tar -C "$CACHE_DIR" -xzf "$CACHE_DIR/$ARCHIVE"
  sh_c cd "$CACHE_DIR/d2-$VERSION"

  sh_c="sh_c"
  if !is_prefix_writable; then
    sh_c="sudo_sh_c"
  fi
  "$sh_c" make install PREFIX="$PREFIX"
}

uninstall() {
  log "uninstalling standalone release d2-$VERSION"

  sh_c cd "$CACHE_DIR/d2-$VERSION"

  sh_c="sh_c"
  if !is_prefix_writable; then
    sh_c="sudo_sh_c"
  fi
  "$sh_c" make uninstall PREFIX="$PREFIX"
}

is_prefix_writable() {
  sh_c mkdir -p "$PREFIX" 2>/dev/null || true
  # The reason for checking whether bin is writable specifically is that on macOS you have
  # /usr/local owned by root but you don't need root to write to its subdirectories which
  # is all we want to do.
  if [ ! -w "$PREFIX/bin" ]; then
    return 0
  fi
}

cache_dir() {
  if [ -n "${XDG_CACHE_HOME-}" ]; then
    echo "$XDG_CACHE_HOME/d2/release"
  elif [ -n "${HOME-}" ]; then
    echo "$HOME/.cache/d2/release"
  else
    echo "/tmp/d2-cache/release"
  fi
}

fetch_version_info() {
  req_version=$VERSION
  log "fetching info on version $req_version"
  if [ -e "$CACHE_DIR/$req_version.json" ]; then
    log "reusing $CACHE_DIR/$req_version.json"
  fi

  rm -f "$CACHE_DIR/req_version.json"
  if [ "$req_version" = latest ]; then
    release_info_url="https://api.github.com/repos/$REPO/releases/$req_version"
  else
    release_info_url="https://api.github.com/repos/$REPO/releases/tags/$req_version"
  fi
  fetch_gh "$release_info_url" "$CACHE_DIR/$req_version.json" \
    'application/json'
  VERSION=$(cat "$CACHE_DIR/$req_version.json" | grep -m1 tag_name | sed 's/^.*: "\(.*\)",$/\1/g')
  if [ "$req_version" = latest ]; then
    mv "$CACHE_DIR/$req_version.json" "$CACHE_DIR/$VERSION.json"
  fi
  echo "$VERSION"
}

curl_gh() {
  sh_c curl -fL ${GITHUB_TOKEN+"-H \"Authorization: Bearer \$GITHUB_TOKEN\""} "$@"
}

fetch_gh() {
  url=$1
  file=$2
  accept=$3

  if [ -e "$file" ]; then
    log "reusing $file"
    return
  fi

  curl_gh -#o "$file.inprogress" -C- -H "'Accept: $accept'" "$url"
  sh_c mv "$file.inprogress" "$file"
}

main "$@"
