# install

You may install D2 through any of the following methods.

<!-- toc -->
- <a href="#installsh" id="toc-installsh">install.sh</a>
  - <a href="#security" id="toc-security">Security</a>
- <a href="#macos-homebrew" id="toc-macos-homebrew">macOS (Homebrew)</a>
- <a href="#standalone" id="toc-standalone">Standalone</a>
- <a href="#from-source" id="toc-from-source">From source</a>
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

We're careful shell programmers and are aware of the many footguns of the unix shell. Our
script was written carefully and with detail. For example, it is not vulnerable to partial
execution and the entire script runs with `set -eu` and very meticulous quoting.

It follows the XDG standards, installs d2 properly into a unix hierarchy path (defaulting
to /usr/local though you can use ~/.local to avoid sudo if you'd like) and allows for easy
uninstall.

Some other niceties are that it'll tell you if you need to adjust `$PATH` or `$MANPATH` to
access d2 and its manpages. It can also install
[TALA](https://github.com/terrastruct/tala) for you with `--tala`. You can also use it to
install a specific version of `d2` with `--version`. Run it with `--help` for more more
detailed docs on its various options and features.

If you're still concerned, remember you can run with `--dry-run` to avoid executing
anything permanent.

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
Download the `.tar.gz` release for your OS/ARCH combination and then run the following
inside the extracted directory to install:

```sh
make install
```

Run the following to uninstall:

```sh
make uninstall
```

If root permissions are required for installation, you'll need to run `make` with `sudo`.
You can control the Unix hierarchy installation path with `PREFIX=`. For example:

```
# Install under ~/.local.
# Binaries will be at ~/.local/bin
# And manpages will be under ~/.local/share/man
# And supporting data like icons and fonts at ~/.local/share/d2
make install PREFIX=$HOME/.local
```

The install script places the standalone release into `$PREFIX/lib/d2/d2-<version>`
and we recommend doing the same with manually installed releases so that you
know where the release directory is for easy uninstall.

> warn: Our binary releases aren't fully static like normal Go binaries due to the C
> dependency on v8go for executing dagre. If you're on an older system with an old
> libc, you'll want to install from source.

## From source

You can always install from source:

```sh
go install oss.terrastruct.com/d2@latest
```

To install a proper release from source clone the repository and then:

```sh
./ci/release/build.sh --install
# To uninstall:
# ./ci/release/build.sh --uninstall
```

## Coming soon

- Docker image
- Windows install
- rpm and deb packages
    - with repositories and standalone
- homebrew core
