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
  [--tala latest] [--force] [--uninstall]

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

--tala [latest]
  Install Terrastruct's closed source TALA for improved layouts.
  See https://github.com/terrastruct/TALA
  It optionally takes an argument of the TALA version to install.
  Installation obeys all other flags, just like the installation of d2. For example,
  the d2plugin-tala binary will be installed into /usr/local/bin/d2plugin-tala

--force:
  Force installation over the existing version even if they match. It will attempt a
  uninstall first before installing the new version. The installed release tree
  will be deleted from /usr/local/lib/d2/d2-<VERSION> but the release archive in
  ~/.cache/d2/release will remain.

--uninstall:
  Uninstall the installed version of d2. The --method and --prefix flags must be the same
  as for installation. i.e if you used --method standalone you must again use --method
  standalone for uninstallation. With detect, the install script will try to use the OS
  package manager to uninstall instead.

All downloaded archives are cached into ~/.cache/d2/release. use \$XDG_CACHE_HOME to change
path of the cached assets. Release archives are unarchived into /usr/local/lib/d2/d2-<VERSION>

note: Deleting the unarchived releases will cause --uninstall to stop working.

You can rerun install.sh to update your version of D2. install.sh will avoid reinstalling
if the installed version is the latest unless --force is passed.
EOF
}

main() {
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
      tala)
        shift "$FLAGSHIFT"
        TALA=${FLAGARG:-latest}
        ;;
      edge)
        flag_noarg && shift "$FLAGSHIFT"
        EDGE=1
        echoerr "$FLAGRAW is currently unimplemented"
        return 1
        ;;
      method)
        flag_nonemptyarg && shift "$FLAGSHIFT"
        METHOD=$FLAGARG
        echoerr "$FLAGRAW is currently unimplemented"
        return 1
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

  REPO=${REPO:-terrastruct/d2}
  OS=$(os)
  ARCH=$(arch)
  if [ -z "${PREFIX-}" -a "$OS" = macos -a "$ARCH" = arm64 ]; then
    # M1 Mac's do not allow modifications to /usr/local even with sudo.
    PREFIX=$HOME/.local
  fi
  PREFIX=${PREFIX:-/usr/local}
  CACHE_DIR=$(cache_dir)
  mkdir -p "$CACHE_DIR"
  INSTALL_DIR=$PREFIX/lib/d2

  if [ -n "${UNINSTALL-}" ]; then
    uninstall
    return 0
  fi

  VERSION=${VERSION:-latest}
  if [ "$VERSION" = latest ]; then
    header "fetching latest release info"
    fetch_release_info
  fi

  install
}

install() {
  install_d2
  if [ -n "${TALA-}" ]; then
    # Run in subshell to avoid overwriting VERSION.
    TALA_VERSION="$( install_tala && echo "$VERSION" )"
  fi

  COLOR=2 header success
  log "d2-$VERSION-$OS-$ARCH has been successfully installed into $PREFIX"
  if [ -n "${TALA-}" ]; then
    log "tala-$TALA_VERSION-$OS-$ARCH has been successfully installed into $PREFIX"
  fi
  if ! echo "$PATH" | grep -qF "$PREFIX/bin"; then
    logcat >&2 <<EOF
Extend your \$PATH to use d2:
  export PATH=$PREFIX/bin:\$PATH
Then run:
  ${TALA+D2_LAYOUT=tala }d2 --help
EOF
  else
    log "  Run ${TALA+D2_LAYOUT=tala }d2 --help for usage."
  fi
  if ! manpath | grep -qF "$PREFIX/share/man"; then
    logcat >&2 <<EOF
Extend your \$MANPATH to view d2's manpages:
  export MANPATH=$PREFIX/share/man\${MANPATH+:\$MANPATH}
Then run:
  man d2
EOF
  if [ -n "${TALA-}" ]; then
    log "  man d2plugin-tala"
  fi
  else
    log "  Run man d2 for detailed docs."
    if [ -n "${TALA-}" ]; then
      log "  Run man d2plugin-tala for detailed docs."
    fi
  fi

  log "Rerun the install script with --uninstall to uninstall"
}

install_d2() {
  if command -v d2 >/dev/null; then
    INSTALLED_VERSION="$(d2 version)"
    if [ ! "${FORCE-}" -a "$VERSION" = "$INSTALLED_VERSION" ]; then
      log "skipping installation as version $VERSION is already installed."
      return 0
    fi
    log "uninstalling $INSTALLED_VERSION to install $VERSION"
    if ! uninstall_d2; then
      warn "failed to uninstall $INSTALLED_VERSION"
    fi
  fi

  header "installing d2-$VERSION"
  install_standalone_d2
}

install_standalone_d2() {
  ARCHIVE="d2-$VERSION-$OS-$ARCH.tar.gz"
  log "installing standalone release $ARCHIVE from github"

  fetch_release_info
  asset_line=$(sh_c 'cat "$RELEASE_INFO" | grep -n "$ARCHIVE" | cut -d: -f1 | head -n1')
  asset_url=$(sh_c 'sed -n $((asset_line-3))p "$RELEASE_INFO" | sed "s/^.*: \"\(.*\)\",$/\1/g"')
  fetch_gh "$asset_url" "$CACHE_DIR/$ARCHIVE" 'application/octet-stream'

  sh_c="sh_c"
  if ! is_prefix_writable; then
    sh_c="sudo_sh_c"
  fi

  "$sh_c" mkdir -p "'$INSTALL_DIR'"
  "$sh_c" tar -C "$INSTALL_DIR" -xzf "$CACHE_DIR/$ARCHIVE"
  "$sh_c" sh -c "'cd \"$INSTALL_DIR/d2-$VERSION\" && make install PREFIX=\"$PREFIX\"'"
}

install_tala() {
  REPO="${REPO_TALA:-terrastruct/TALA}"
  VERSION=$TALA
  RELEASE_INFO=
  fetch_release_info
  header "installing tala-$VERSION"
  install_standalone_tala
}

install_standalone_tala() {
  ARCHIVE="tala-$VERSION-$OS-$ARCH.tar.gz"
  log "installing standalone release $ARCHIVE from github"

  asset_line=$(sh_c 'cat "$RELEASE_INFO" | grep -n "$ARCHIVE" | cut -d: -f1 | head -n1')
  asset_url=$(sh_c 'sed -n $((asset_line-3))p "$RELEASE_INFO" | sed "s/^.*: \"\(.*\)\",$/\1/g"')

  fetch_gh "$asset_url" "$CACHE_DIR/$ARCHIVE" 'application/octet-stream'

  sh_c="sh_c"
  if ! is_prefix_writable; then
    sh_c="sudo_sh_c"
  fi

  "$sh_c" mkdir -p "'$INSTALL_DIR'"
  "$sh_c" tar -C "$INSTALL_DIR" -xzf "$CACHE_DIR/$ARCHIVE"
  "$sh_c" sh -c "'cd \"$INSTALL_DIR/tala-$VERSION\" && make install PREFIX=\"$PREFIX\"'"
}

uninstall() {
  if ! command -v d2 >/dev/null; then
    warn "no version of d2 installed"
    return 0
  fi
  INSTALLED_VERSION="$(d2 --version)"
  if ! uninstall_d2; then
    echoerr "failed to uninstall $INSTALLED_VERSION"
    return 1
  fi
  if [ "${TALA-}" ]; then
    if ! command -v d2plugin-tala >/dev/null; then
      warn "no version of tala installed"
      return 0
    fi
    INSTALLED_VERSION="$(d2plugin-tala --version)"
    if ! uninstall_tala; then
      echoerr "failed to uninstall tala $INSTALLED_VERSION"
      return 1
    fi
  fi
  return 0
}

uninstall_d2() {
  header "uninstalling d2-$INSTALLED_VERSION"
  uninstall_standalone_d2
}

uninstall_standalone_d2() {
  log "uninstalling standalone release of d2-$INSTALLED_VERSION"

  if [ ! -e "$INSTALL_DIR/d2-$INSTALLED_VERSION" ]; then
    warn "missing standalone install release directory $INSTALL_DIR/d2-$INSTALLED_VERSION"
    warn "d2 have been installed via some other installation method."
    return 1
  fi

  sh_c="sh_c"
  if ! is_prefix_writable; then
    sh_c="sudo_sh_c"
  fi

  "$sh_c" sh -c "'cd \"$INSTALL_DIR/d2-$INSTALLED_VERSION\" && make uninstall PREFIX=\"$PREFIX\"'"
  "$sh_c" rm -rf "$INSTALL_DIR/d2-$INSTALLED_VERSION"
}

uninstall_tala() {
  header "uninstalling tala-$INSTALLED_VERSION"
  uninstall_standalone_tala
}

uninstall_standalone_tala() {
  log "uninstalling standalone release tala-$INSTALLED_VERSION"

  if [ ! -e "$INSTALL_DIR/tala-$INSTALLED_VERSION" ]; then
    warn "missing standalone install release directory $INSTALL_DIR/tala-$INSTALLED_VERSION"
    warn "tala have been installed via some other installation method."
    return 1
  fi

  sh_c="sh_c"
  if ! is_prefix_writable; then
    sh_c="sudo_sh_c"
  fi

  "$sh_c" sh -c "'cd \"$INSTALL_DIR/tala-$INSTALLED_VERSION\" && make uninstall PREFIX=\"$PREFIX\"'"
  "$sh_c" rm -rf "$INSTALL_DIR/tala-$INSTALLED_VERSION"
}

is_prefix_writable() {
  sh_c "mkdir -p '$INSTALL_DIR' 2>/dev/null" || true
  # The reason for checking whether $INSTALL_DIR is writable is that on macOS you have
  # /usr/local owned by root but you don't need root to write to its subdirectories which
  # is all we want to do.
  if [ ! -w "$INSTALL_DIR" ]; then
    return 1
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

fetch_release_info() {
  if [ -n "${RELEASE_INFO-}" ]; then
    return 0
  fi

  log "fetching info on $VERSION version of $REPO"
  RELEASE_INFO=$(mktemp -d)/release-info.json
  if [ "$VERSION" = latest ]; then
    release_info_url="https://api.github.com/repos/$REPO/releases/$VERSION"
  else
    release_info_url="https://api.github.com/repos/$REPO/releases/tags/$VERSION"
  fi
  fetch_gh "$release_info_url" "$RELEASE_INFO" \
    'application/json'
  VERSION=$(cat "$RELEASE_INFO" | grep -m1 tag_name | sed 's/^.*: "\(.*\)",$/\1/g')
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
