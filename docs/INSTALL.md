# install

You may install `d2` through any of the following methods.

<!-- toc -->
- <a href="#installsh" id="toc-installsh">install.sh</a>
  - <a href="#security" id="toc-security">Security</a>
- <a href="#macos-homebrew" id="toc-macos-homebrew">macOS (Homebrew)</a>
- <a href="#standalone" id="toc-standalone">Standalone</a>
  - <a href="#manual" id="toc-manual">Manual</a>
  - <a href="#prefix" id="toc-prefix">PREFIX</a>
- <a href="#from-source" id="toc-from-source">From source</a>
  - <a href="#source-release" id="toc-source-release">Source Release</a>
- <a href="#coming-soon" id="toc-coming-soon">Coming soon</a>

## install.sh

The recommended and easiest way to install is with our install script, which will detect
the OS and architecture you're on and use the best method:

```sh
# With --dry-run the install script will print the commands it will use
# to install without actually installing so you know what it's going to do.
curl -fsSL https://d2lang.com/install.sh | sh -s -- --dry-run
# If things look good, install for real.
curl -fsSL https://d2lang.com/install.sh | sh -s --
```

For help on the terminal run, including the supported package managers and detection
methods:

```sh
curl -fsSL https://d2lang.com/install.sh | sh -s -- --help
```

### Security

The install script is not the most secure way to install d2. We recommend that if
possible, you use your OS's package manager directly or install from source with `go` as
described below.

But this does not mean the install script is insecure. There is no major flaw that
the install script is more vulnerable to than any other method of manual installation.
The most secure installation method involves a second independent entity, i.e your OS
package repos or Go's proxy server.

We're careful shell programmers and are aware of the many footguns of the Unix shell. Our
script was written carefully and with detail. For example, it is not vulnerable to partial
execution and the entire script runs with `set -eu` and very meticulous quoting.

It follows the XDG standards, installs `d2` properly into a Unix hierarchy path
(`/usr/local` unless `/usr/local` requires sudo in which case `~/.local` is used) and
allows for easy uninstall. You can easily adjust the used path with `--prefix`.

Some other niceties are that it'll tell you if you need to adjust `$PATH` or `$MANPATH` to
access `d2` and its manpages. It can also install
[TALA](https://github.com/terrastruct/tala) for you with `--tala`. You can also use it to
install a specific version of `d2` with `--version`. Run it with `--help` for more more
detailed docs on its various options and features.

If you're still concerned, remember you can run with `--dry-run` to avoid writing anything.

The install script does not yet verify any signature on the downloaded release
but that is coming soon. [#315](https://github.com/terrastruct/d2/issues/315)

## macOS (Homebrew)

If you're on macOS, you can install with `brew`.

```sh
brew tap terrastruct/d2
brew install d2
```

> The install script above does this automatically if you have `brew` installed and
> are running it on macOS.

## Standalone

We publish standalone release archives for every release on Github.

Here's a minimal example script that downloads a standalone release, extracts it into the
current directory and then installs it.
Adjust VERSION, OS, and ARCH as needed.

```sh
VERSION=v0.0.13 OS=macos ARCH=amd64 curl -fsSLO \
    "https://github.com/terrastruct/d2/releases/download/$VERSION/d2-$VERSION-$OS-$ARCH.tar.gz" \
    && tar -xzf "d2-$VERSION-$OS-$ARCH.tar.gz" \
    && make -sC "d2-$VERSION" install
```

To uninstall:

```sh
VERSION=v0.0.13 make -sC "d2-$VERSION" uninstall
```

### Manual

You can also manually download the `.tar.gz` release for your OS/ARCH combination and then
run the following inside the extracted directory to install:

```sh
make install
```

Run the following to uninstall:

```sh
make uninstall
```

### PREFIX

You can control the Unix hierarchy installation path with `PREFIX=`. For example:

```sh
# Install under ~/.local.
# Binaries will be at ~/.local/bin
# And manpages will be under ~/.local/share/man
# And supporting data like icons and fonts at ~/.local/share/d2
make install PREFIX=$HOME/.local
```

The install script places the standalone release into `$PREFIX/lib/d2/d2-<version>`
and we recommend doing the same with manually installed releases so that you
know where the release directory is for easy uninstall.

## From source

You can always install from source:

```sh
go install oss.terrastruct.com/d2@latest
```

### Source Release

To install a proper release from source clone the repository and then:

```sh
./ci/release/build.sh --install
# To uninstall:
# ./ci/release/build.sh --uninstall
```

Installing a real release will also install manpages and in the future other assets like
fonts and icons. Furthermore, when installing a non versioned commit, installing a proper
release will ensure that `d2 --version` works correctly by embedding the commit hash into
the `d2` binary.

## Coming soon

- Docker image
- Windows install
- rpm and deb packages
    - with repositories and standalone
- homebrew core
