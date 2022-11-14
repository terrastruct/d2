# release

## _install.sh

The template for the install script in the root of the repository.

### gen_install.sh

Generates the install.sh script in the root of the repository by prepending the libraries
it depends on from ../sub/lib.

## release.sh

- ./release.sh is the top level script to generate a new release.
  Run with --help for usage.

## build.sh

- ./build.sh builds the release archives for each platform into ./build/<VERSION>/*.tar.gz
  Run with --help for usage.

> note: Remember for production releases you need to set the $TSTRUCT_OS_ARCH_BUILDER
> variables as we must compile d2 directly on each release target to include dagre.
> See https://github.com/terrastruct/d2/issues/31

### build_docker.sh

Helper script called by build.sh to build D2 on each linux runner inside Docker.
The Dockerfile is in ./builders/Dockerfile

### _build.sh

Called by build.sh (with --local or macOS) or build_docker.sh (on linux) to create the
release archive.

Do not invoke directly. If you want to produce a build for a single platform run build.sh
as so:

```sh
 # To only build the linux-amd64 release.
./build.sh --run=linux-amd64
```

```sh
 # To only build the linux-amd64 release locally.
./build.sh --local --run=linux-amd64
```
