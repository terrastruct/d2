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
usage: $arg0 [-d|--dry-run] [--version vX.X.X] [--edge] [--method detect] [--prefix path]
  [--tala latest] [--force] [--uninstall] [-x|--trace]

install.sh automates the installation of D2 onto your system. It currently only supports
the installation of standalone releases from GitHub and via Homebrew on macOS. See the
docs for --detect below for more information

If you pass --edge, it will clone the source, build a release and install from it.
--edge is incompatible with --tala and currently unimplemented.

\$PREFIX in the docs below refers to the path set by --prefix. See docs on the --prefix
flag below for the default.

Flags:

-d, --dry-run
  Pass to have install.sh show the install method and flags that will be used to install
  without executing them. Very useful to understand what changes it will make to your system.

--version vX.X.X
  Pass to have install.sh install the given version instead of the latest version.
  warn: The version may not be obeyed with package manager installations. Use
        --method=standalone to enforce the version.

--edge
  Pass to build and install D2 from source. This will still use --method if set to detect
  to install the release archive for your OS, whether it's apt, yum, brew or standalone
  if an unsupported package manager is used.

  To install from source like a dev would, use go install oss.terrastruct.com/d2. There's
  also ./ci/release/build.sh --install to build and install a proper standalone release
  including manpages. The proper release will also ensure d2 --version shows the correct
  version by embedding the commit hash into the binary.

  note: currently unimplemented.
  warn: incompatible with --tala as TALA is closed source.

--method [detect | standalone | homebrew ]
  Pass to control the method by which to install. Right now we only support standalone
  releases from GitHub but later we'll add support for brew, rpm, deb and more.

  - detect will use your OS's package manager automatically.
    So far it only detects macOS and automatically uses homebrew.
  - homebrew uses https://brew.sh/ which is a macOS and Linux package manager.
  - standalone installs a standalone release archive into the unix hierarchy path
     specified by --prefix

--prefix path
  Controls the unix hierarchy path into which standalone releases are installed.
  Defaults to /usr/local or ~/.local if /usr/local is not writable by the current user.
  Remember that whatever you use, you must have the bin directory of your prefix path in
  \$PATH to execute the d2 binary. For example, if my prefix directory is /usr/local then
  my \$PATH must contain /usr/local/bin.
  You may also need to include \$PREFIX/share/man into \$MANPATH.
  install.sh will tell you whether \$PATH or \$MANPATH need to be updated after successful
  installation.

--tala [latest]
  Install Terrastruct's closed source TALA for improved layouts.
  See https://github.com/terrastruct/tala
  It optionally takes an argument of the TALA version to install.
  Installation obeys all other flags, just like the installation of d2. For example,
  the d2plugin-tala binary will be installed into \$PREFIX/bin/d2plugin-tala
  warn: The version may not be obeyed with package manager installations. Use
        --method=standalone to enforce the version.

--force:
  Force installation over the existing version even if they match. It will attempt a
  uninstall first before installing the new version. The installed release tree
  will be deleted from \$PREFIX/lib/d2/d2-<VERSION> but the release archive in
  ~/.cache/d2/release will remain.

--uninstall:
  Uninstall the installed version of d2. The --method and --prefix flags must be the same
  as for installation. i.e if you used --method standalone you must again use --method
  standalone for uninstallation. With detect, the install script will try to use the OS
  package manager to uninstall instead.
  note: tala will also be uninstalled if installed.

-x, --trace:
  Run script with set -x.

All downloaded archives are cached into ~/.cache/d2/release. use \$XDG_CACHE_HOME to change
path of the cached assets. Release archives are unarchived into \$PREFIX/lib/d2/d2-<VERSION>

note: Deleting the unarchived releases will cause --uninstall to stop working.

You can rerun install.sh to update your version of D2. install.sh will avoid reinstalling
if the installed version is the latest unless --force is passed.

See https://github.com/terrastruct/d2/blob/master/docs/INSTALL.md#security for
documentation on its security.
EOF
}

main() {
  while flag_parse "$@"; do
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      d|dry-run)
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
      x|trace)
        flag_noarg && shift "$FLAGSHIFT"
        set -x
        export TRACE=1
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done
  shift "$FLAGSHIFT"

  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
  fi

  REPO=${REPO:-terrastruct/d2}
  ensure_os
  ensure_arch
  ensure_prefix
  CACHE_DIR=$(cache_dir)
  mkdir -p "$CACHE_DIR"
  METHOD=${METHOD:-detect}
  INSTALL_DIR=$PREFIX/lib/d2

  case $METHOD in
    detect)
      case "$OS" in
        macos)
          if command -v brew >/dev/null; then
            log "detected macOS with homebrew, using homebrew for installation"
            METHOD=homebrew
          else
            warn "detected macOS without homebrew, falling back to --method=standalone"
            METHOD=standalone
          fi
          ;;
        linux|windows)
          METHOD=standalone
          ;;
        *)
          warn "unrecognized OS $OS, falling back to --method=standalone"
          METHOD=standalone
          ;;
      esac
      ;;
    standalone) ;;
    homebrew) ;;
    *)
      echoerr "unknown installation method $METHOD"
      return 1
      ;;
  esac

  if [ -n "${UNINSTALL-}" ]; then
    uninstall
    if [ -n "${DRY_RUN-}" ]; then
      bigheader "Rerun without --dry-run to execute printed commands and perform install."
    fi
  else
    install
    if [ -n "${DRY_RUN-}" ]; then
      bigheader "Rerun without --dry-run to execute printed commands and perform install."
    fi
  fi
}

install() {
  case $METHOD in
    standalone)
      install_d2_standalone
      if [ -n "${TALA-}" ]; then
        # Run in subshell to avoid overwriting VERSION.
        TALA_VERSION="$( RELEASE_INFO= install_tala_standalone && echo "$VERSION" )"
      fi
      ;;
    homebrew)
      install_d2_brew
      if [ -n "${TALA-}" ]; then install_tala_brew; fi
      ;;
  esac

  FGCOLOR=2 bigheader 'next steps'
  case $METHOD in
    standalone) install_post_standalone ;;
    homebrew) install_post_brew ;;
  esac
  install_post_warn
}

install_post_standalone() {
  log "d2-$VERSION-$OS-$ARCH has been successfully installed into $PREFIX"
  if [ -n "${TALA-}" ]; then
    log "tala-$TALA_VERSION-$OS-$ARCH has been successfully installed into $PREFIX"
  fi
  log "Rerun this install script with --uninstall to uninstall."
  log
  if ! echo "$PATH" | grep -qF "$PREFIX/bin"; then
    logcat >&2 <<EOF
Extend your \$PATH to use d2:
  export PATH=$PREFIX/bin:\$PATH
Then run:
  ${TALA:+D2_LAYOUT=tala }d2 --help
EOF
  else
    log "Run ${TALA:+D2_LAYOUT=tala }d2 --help for usage."
  fi
  if ! manpath 2>/dev/null | grep -qF "$PREFIX/share/man"; then
    logcat >&2 <<EOF
Extend your \$MANPATH to view d2's manpages:
  export MANPATH=$PREFIX/share/man:\$MANPATH
Then run:
  man d2
EOF
  if [ -n "${TALA-}" ]; then
    log "  man d2plugin-tala"
  fi
  else
    log "Run man d2 for detailed docs."
    if [ -n "${TALA-}" ]; then
      log "Run man d2plugin-tala for detailed TALA docs."
    fi
  fi
}

install_post_brew() {
  log "d2 has been successfully installed with homebrew."
  if [ -n "${TALA-}" ]; then
    log "tala has been successfully installed with homebrew."
  fi
  log "Rerun this install script with --uninstall to uninstall."
  log
  log "Run ${TALA:+D2_LAYOUT=tala }d2 --help for usage."
  log "Run man d2 for detailed docs."
  if [ -n "${TALA-}" ]; then
    log "Run man d2plugin-tala for detailed TALA docs."
  fi

  VERSION=$(brew info d2 | head -n1 | cut -d' ' -f4)
  VERSION=${VERSION%,}
  if [ -n "${TALA-}" ]; then
    TALA_VERSION=$(brew info tala | head -n1 | cut -d' ' -f4)
    TALA_VERSION=${TALA_VERSION%,}
  fi
}

install_post_warn() {
  if command -v d2 >/dev/null; then
    INSTALLED_VERSION=$(d2 --version)
    if [ "$INSTALLED_VERSION" != "$VERSION" ]; then
      warn "newly installed d2 $VERSION is shadowed by d2 $INSTALLED_VERSION in \$PATH"
    fi
  fi
  if [ -n "${TALA-}" ] && command -v d2plugin-tala >/dev/null; then
    INSTALLED_TALA_VERSION=$(d2plugin-tala --version)
    if [ "$INSTALLED_TALA_VERSION" != "$TALA_VERSION" ]; then
      warn "newly installed d2plugin-tala $TALA_VERSION is shadowed by d2plugin-tala $INSTALLED_TALA_VERSION in \$PATH"
    fi
  fi
}

install_d2_standalone() {
  VERSION=${VERSION:-latest}
  header "installing d2-$VERSION"

  ensure_version

  if command -v d2 >/dev/null; then
    INSTALLED_VERSION="$(d2 --version)"
    if [ ! "${FORCE-}" -a "$VERSION" = "$INSTALLED_VERSION" ]; then
      log "skipping installation as d2 $VERSION is already installed."
      return 0
    fi
    log "uninstalling d2 $INSTALLED_VERSION to install $VERSION"
    if ! uninstall_d2_standalone; then
      warn "failed to uninstall d2 $INSTALLED_VERSION"
    fi
  fi

  ARCHIVE="d2-$VERSION-$OS-$ARCH.tar.gz"
  log "installing standalone release $ARCHIVE from github"

  ensure_version
  asset_url="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"
  fetch_gh "$asset_url" "$CACHE_DIR/$ARCHIVE" 'application/octet-stream'

  ensure_prefix_sh_c
  "$sh_c" mkdir -p "'$INSTALL_DIR'"
  "$sh_c" tar -C "$INSTALL_DIR" -xzf "$CACHE_DIR/$ARCHIVE"
  "$sh_c" sh -c "'cd \"$INSTALL_DIR/d2-$VERSION\" && make install PREFIX=\"$PREFIX\"'"
}

install_d2_brew() {
  header "installing d2 with homebrew"
  sh_c brew update
  sh_c brew install d2
}

install_tala_standalone() {
  REPO="${REPO_TALA:-terrastruct/tala}"
  VERSION=$TALA

  header "installing tala-$VERSION"

  ensure_version

  if command -v d2plugin-tala >/dev/null; then
    INSTALLED_VERSION="$(d2plugin-tala --version)"
    if [ ! "${FORCE-}" -a "$VERSION" = "$INSTALLED_VERSION" ]; then
      log "skipping installation as tala $VERSION is already installed."
      return 0
    fi
    log "uninstalling tala $INSTALLED_VERSION to install $VERSION"
    if ! uninstall_tala_standalone; then
      warn "failed to uninstall tala $INSTALLED_VERSION"
    fi
  fi

  ARCHIVE="tala-$VERSION-$OS-$ARCH.tar.gz"
  log "installing standalone release $ARCHIVE from github"

  ensure_version
  asset_url="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"
  fetch_gh "$asset_url" "$CACHE_DIR/$ARCHIVE" 'application/octet-stream'

  ensure_prefix_sh_c
  "$sh_c" mkdir -p "'$INSTALL_DIR'"
  "$sh_c" tar -C "$INSTALL_DIR" -xzf "$CACHE_DIR/$ARCHIVE"
  "$sh_c" sh -c "'cd \"$INSTALL_DIR/tala-$VERSION\" && make install PREFIX=\"$PREFIX\"'"
}

install_tala_brew() {
  header "installing tala with homebrew"
  sh_c brew update
  sh_c brew install terrastruct/tap/tala
}

uninstall() {
  # We uninstall tala first as package managers require that it be uninstalled before
  # uninstalling d2 as TALA depends on d2.
  if command -v d2plugin-tala >/dev/null; then
    INSTALLED_VERSION="$(d2plugin-tala --version)"
    header "uninstalling tala-$INSTALLED_VERSION"
    case $METHOD in
      standalone) uninstall_tala_standalone ;;
      homebrew) uninstall_tala_brew ;;
    esac
  elif [ "${TALA-}" ]; then
    warn "no version of tala installed"
  fi

  if ! command -v d2 >/dev/null; then
    warn "no version of d2 installed"
    return 0
  fi

  INSTALLED_VERSION="$(d2 --version)"
  header "uninstalling d2-$INSTALLED_VERSION"
  case $METHOD in
    standalone) uninstall_d2_standalone ;;
    homebrew) uninstall_d2_brew ;;
  esac
}

uninstall_d2_standalone() {
  log "uninstalling standalone release of d2-$INSTALLED_VERSION"

  if [ ! -e "$INSTALL_DIR/d2-$INSTALLED_VERSION" ]; then
    warn "missing standalone install release directory $INSTALL_DIR/d2-$INSTALLED_VERSION"
    warn "d2 must have been installed via some other installation method."
    return 1
  fi

  ensure_prefix_sh_c
  "$sh_c" sh -c "'cd \"$INSTALL_DIR/d2-$INSTALLED_VERSION\" && make uninstall PREFIX=\"$PREFIX\"'"
  "$sh_c" rm -rf "$INSTALL_DIR/d2-$INSTALLED_VERSION"
}

uninstall_d2_brew() {
  sh_c brew remove d2
}

uninstall_tala_standalone() {
  log "uninstalling standalone release tala-$INSTALLED_VERSION"

  if [ ! -e "$INSTALL_DIR/tala-$INSTALLED_VERSION" ]; then
    warn "missing standalone install release directory $INSTALL_DIR/tala-$INSTALLED_VERSION"
    warn "tala must have been installed via some other installation method."
    return 1
  fi

  ensure_prefix_sh_c
  "$sh_c" sh -c "'cd \"$INSTALL_DIR/tala-$INSTALLED_VERSION\" && make uninstall PREFIX=\"$PREFIX\"'"
  "$sh_c" rm -rf "$INSTALL_DIR/tala-$INSTALLED_VERSION"
}

uninstall_tala_brew() {
  sh_c brew remove tala
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

ensure_version() {
  if [ "$VERSION" = latest ]; then
    log "fetching latest version of $REPO"
    redirect_url=$(curl -fsSLI -o /dev/null -w "%{url_effective}" "https://github.com/$REPO/releases/latest")
    if [ $? -ne 0 ]; then
      warn "redirect method failed, falling back to API"
      release_info_url="https://api.github.com/repos/$REPO/releases/latest"
      release_info=$(mktempd)/release-info.json
      DRY_RUN= fetch_gh "$release_info_url" "$release_info" 'application/json'
      VERSION=$(cat "$release_info" | grep -m1 tag_name | sed 's/^.*: "\(.*\)",$/\1/g')
      log "detected latest version via API: $VERSION"
    else
      # Extract version from redirected URL
      VERSION="${redirect_url##*/}"
      log "detected latest version: $VERSION"
    fi
  fi
  
  # Ensure VERSION has 'v' prefix
  case "$VERSION" in
    v*) ;;
    *) VERSION="v$VERSION" ;;
  esac
}

curl_gh() {
  retry_curl_gh "$@"
}

retry_curl_gh() {
  local max_attempts=3
  local attempt=1
  local delay=1
  
  while [ $attempt -le $max_attempts ]; do
    set +e
    sh_c curl -fL ${GITHUB_TOKEN:+"-H \"Authorization: Bearer \$GITHUB_TOKEN\""} "$@"
    local exit_code=$?
    set -e
    
    # Success case
    if [ $exit_code -eq 0 ]; then
      return 0
    fi
    
    # Retry on HTTP errors (exit code 22) which includes 403, 429, 5xx
    if [ $exit_code -eq 22 ] && [ $attempt -lt $max_attempts ]; then
      warn "GitHub API request failed (attempt $attempt/$max_attempts), retrying in ${delay}s..."
      sleep $delay
      delay=$((delay * 2))
      attempt=$((attempt + 1))
    else
      # Log the reason for failure
      if [ $exit_code -eq 22 ]; then
        warn "GitHub API request failed after $max_attempts attempts, giving up"
      else
        warn "GitHub API request failed with non-retryable error (exit code: $exit_code)"
      fi
      return $exit_code
    fi
  done
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

# The main function does more than provide organization. It provides robustness in that if
# the install script was to only partial download into sh, sh will not execute it because
# main is not invoked until the very last byte.
main "$@"
