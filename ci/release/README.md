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

### Docker image helper

`./docker/build.sh` is retained as a load-only local development helper. It rejects
`--push`, `--latest`, and inherited `RELEASE` publishing. The release script no longer
calls it or publishes Docker tags from a legacy AWS SSH builder.

### Production Docker publishing

The manually dispatched `Publish Docker release` GitHub workflow is the production path
for Docker Hub. It replaces only the Docker portion of the legacy AWS release path; the
other release asset builders are unchanged.

Run it from the protected `master` branch after the GitHub release is published. Enter the
exact v-prefixed release version and leave `publish_latest` off unless this stable release
should become the default Docker image. The workflow rejects `publish_latest` for a GitHub
prerelease or a semver prerelease.

The `docker-release` GitHub environment must provide a `DOCKERHUB_USERNAME` variable set
to `d2lang` and a `DOCKERHUB_TOKEN` secret containing a personal access token for that
Docker ID. The Docker ID must have write access to both the canonical `d2lang/d2`
repository and the maintained `terrastruct/d2` compatibility mirror. The environment's
deployment branch policy must
allow only protected branches. Docker Hub tag immutability must remain enabled on both
repositories for version tags with
`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$`; `latest` remains outside
that rule and mutable. This repository-side rule is the final overwrite guard.

Before publishing a production tag, the workflow:

1. verifies the protected-master dispatch, published non-draft semver GitHub release, exact
   Linux amd64 and arm64 asset IDs, and their GitHub SHA-256 digests;
2. explicitly peels the Git tag to a commit, requires that commit to be an ancestor of the
   dispatched `master` commit, and uses that release commit's Dockerfile;
3. builds on native GitHub-hosted amd64 and arm64 runners and pushes only untagged digests
   with provenance;
4. runs native version, SVG, and PNG smoke tests for both digests; and
5. dry-runs and validates the exact two-platform manifest, including provenance
   attestations; then
6. immediately re-fetches the release and re-peels its tag, requiring the release ID,
   publication/prerelease state, tag commit, asset IDs, and asset digests to be unchanged.

Only then does it create the requested version tag in the canonical repository and its
compatibility mirror. Existing version tags are immutable, so the workflow refuses to
overwrite one. If `publish_latest` was explicitly selected, it updates `latest` in both
repositories from the immutable version-manifest digest only after the version manifests
are published and verified. If verification or `latest` promotion fails after a version
tag was created, use GitHub's **Re-run failed jobs** action: the same run accepts an
existing version tag only when its descriptors exactly match that run's verified candidate,
and the separate `latest` job can retry without touching the version tags. A new dispatch
still refuses any existing version tag during preflight.

### Docker continuity test

The manually dispatched `Docker continuity test` GitHub workflow proves that an existing,
published D2 release can be rebuilt for Docker Hub without the legacy AWS builders. It
downloads the release's exact Linux archives, builds on native GitHub-hosted amd64 and arm64
runners, verifies the images, and publishes only
`d2lang/d2:continuity-test-<workflow-run-id>-<attempt>` and the matching
`terrastruct/d2` compatibility tag. It never updates a release version tag or `latest`.

Dispatch the workflow from the protected `master` branch. The version input must name a
published, non-draft GitHub release with both Linux archives; `v0.7.1` is the default
continuity fixture. The workflow removes both test tags after verification. If Docker Hub
rejects the cleanup request, the workflow emits a warning identifying the two tags to
delete manually.

The `v0.7.1` fixture embeds playwright-go v0.4702.0, whose original driver CDN no longer
serves the required ZIP files. For that fixture only, the workflow reconstructs the same
Playwright 1.47.2 driver layout from a checksum-pinned official `playwright-core` npm
tarball and the image's Node runtime. Browser payloads use Playwright's current direct CDN.
The published D2 archive remains unchanged. Other release versions use their tagged
Dockerfile without this compatibility step.

This remains a non-production regression test. The separate `Publish Docker release`
workflow is the production publisher.

### _build.sh

Called by build.sh to create a release archive.

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
