# release

## _install.sh

The template for the install script in the root of the d2 repository.

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
> See https://github.com/d2lang/d2/issues/31

Use `--host-only` to build only the release for the host's `$OS-$ARCH` pair.

### build_docker.sh

Helper script called by build.sh to build D2 on each linux runner inside Docker.
The Dockerfile is in ./linux/Dockerfile

### Docker continuity test

The manually dispatched `Docker continuity test` GitHub workflow proves that an existing,
published D2 release can be rebuilt for Docker Hub without the legacy AWS builders. It
downloads the release's exact Linux archives, builds on native GitHub-hosted amd64 and arm64
runners, verifies the images, and publishes only
`terrastruct/d2:continuity-test-<workflow-run-id>`. It never updates a release version tag or
`latest`.

Before dispatching it, configure the `docker-release` GitHub environment with a
`DOCKERHUB_USERNAME` variable and a `DOCKERHUB_TOKEN` secret that can write to that user's
`d2` repository. Dispatch the workflow from the protected `master` branch. The version input
must name a published, non-draft GitHub release with both Linux archives; `v0.7.1` is the
default continuity fixture. Delete the continuity-test tag in Docker Hub after reviewing the
workflow summary and manifest.

This test does not disable or replace the existing release script's Docker publishing path.

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
