# install

You may install D2 through any of the following methods.

<!-- toc -->

- [install.sh](#installsh)
- [macOS (Homebrew)](#macos-homebrew)
- [Standalone](#standalone)
- [From source](#from-source)

<!-- tocstop -->

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
go install oss.terrastruct.com/d2/cmd/d2@latest
```

## Coming soon

- Docker image
- Windows install
- rpm and deb packages
    - with repositories and standalone
- homebrew core
