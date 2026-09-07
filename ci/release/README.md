# release

## npm publishing

`@d2lang/d2` is the canonical JavaScript package. `@terrastruct/d2` is published from the
same built files as a compatibility package during the namespace transition. The
`npm-stage.yml` workflow stages both package names for review with npm trusted publishing;
each staged package has its own stage ID and must be reviewed and approved separately. If
one stage succeeds and the other fails, use GitHub's **Re-run failed jobs** action on the
same workflow run. That preserves the original commit and reuses the same immutable
package artifact; do not use **Re-run all jobs** or start a fresh workflow dispatch to
recover only one identity.

Before staging an official version, bump `d2js/js/package.json` and `package-lock.json` to
that exact version in a signed commit. Nightly stages derive their prerelease version from
the checked-in version automatically. `@d2lang/d2` must already exist and both packages
must separately trust the `d2lang/d2` `npm-stage.yml` workflow for stage-only publishing.
Each trusted publisher is restricted to the `npm-release` GitHub environment, whose
deployment branch policy accepts only protected branches. The workflow itself also
refuses to pack from any ref other than `refs/heads/master`.
The canonical package was bootstrapped once from the existing `@terrastruct/d2@0.1.33`
built artifacts because npm does not allow a brand-new package to use staged publishing.

Direct publishing is retained only for local bootstrap or recovery. It requires a
short-lived `NPM_TOKEN` that can publish every selected package and is not stored in GitHub.
The script pre-packs and preflights every selected package before publishing; a retry skips
an existing version only when its registry tarball integrity exactly matches the local
artifact.

After the workflow finishes, confirm both matrix stage jobs are green and inspect the two
pending records with `npm stage list` and `npm stage view <stage-id>`. Approve each only
after their package versions and artifacts from `npm stage download <stage-id>` match.
Use `npm stage approve <stage-id>` for each identity. If the release is abandoned, reject
every pending half with
`npm stage reject <stage-id>` before starting another run. A pending stage reserves its
package/version, so list pending stages before retrying any failed job.

## _install.sh

The template for the install script in the root of the d2 repository.

### gen_install.sh

Generates the install.sh script in the root of the repository by prepending the libraries
it depends on from ../sub/lib.

## release.sh

- ./release.sh is the top level script to generate a new release.
  Run with --help for usage.

Run from a clean checkout with Python 3, Git, and an authenticated GitHub CLI. The script
prepares the release branch, changelog, PR, and tag, then returns. It leaves existing PR
descriptions unchanged and uses the Human/AI template for new PRs. Prereleases must have
a semantic-version suffix, for example `--version=v0.9.0-rc.1`; `--prerelease` with a
stable-looking version is rejected.

A version is assigned once. The script never amends a release commit, force-pushes a branch,
or replaces a tag, including tags for draft releases. It checks local and remote release
refs before making changes. For an unfinished release, a matching remote tag makes
preparation a no-op with guidance to follow the existing Actions run. Published releases
and drafts with an MSI reject preparation. Conflicting tags or release branches require a
new version. A local tag left by a failed push can be pushed only when it points to the
unchanged prepared commit, without recreating the tag object. An untagged partial preparation
reuses its existing changelog commit and pushes the branch normally; it does not create another
empty commit.
If a remote preparation commit is unavailable locally, fetch origin before retrying.

Enable **Release immutability** in the repository's release settings to enforce the lock
on GitHub. Draft assets remain editable until publication; the uploader still accepts only
identical existing assets. Publishing freezes the release assets and associated tag, so
attach and verify every asset before publishing. Release notes/title and prerelease/latest
status remain editable. Enabling this setting applies to future published releases; it does
not retroactively lock already published releases. See [GitHub's release immutability
settings](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/establish-provenance-and-integrity/prevent-release-changes).

Pushing the tag starts one `Release archives` dependency graph:

1. The existing CI workflow tests the exact tag commit. In parallel, the archive job builds
   all six archives twice using the pinned Go toolchain, compares SHA-256 digests, verifies
   stripped binaries and normalized archives, enforces size budgets, and generates an SBOM.
2. Each archive runs natively on its supported platform, checking its version, SVG, and PNG.
3. The reusable `Windows MSI` workflow consumes that run's Windows amd64 archive, verifies
   its checksum, and builds and verifies the installer on `windows-2022`. It always packages
   notices from the new archive, so an old published release is no longer a PR test fixture.
4. After tests, native smoke, and MSI verification succeed, the workflow attests the archive
   provenance, SBOM, and checksum manifest. A separate write-scoped job creates or resumes a
   draft and uploads the same archives, `SHA256SUMS`, MSI, and `d2.spdx.json`.

The uploader rechecks the tag commit and draft identity/state before uploads and verifies
all final asset digests. Existing assets are accepted only when their bytes match; nothing
is overwritten. There is no workstation download/upload handoff or independent MSI poller.

After the workflow succeeds, review and merge the release PR, then publish the complete
draft manually on GitHub. `release.sh` rejects `--publish`, `--skip-build`, and `--rebuild`,
and never repurposes an existing version. To recover a failed run without changing code or
artifacts, use **Re-run failed jobs** on that run. Code or build changes require a new version,
even if the old version is still a draft. Its Actions artifacts are immutable: a full
rerun fails rather than replacing them. If draft upload was interrupted, the failed job
resumes by accepting identical existing assets and attaching only missing ones. Artifacts
are retained for 30 days; recover failures within that window.

Release-related PRs exercise the same archive and native smoke jobs and, when the WiX gate
below is enabled, the MSI build. PRs never attest or write release assets. CI is called
directly by the tag workflow, so it does not depend on another token-generated event.

Every release produced by this workflow includes `SHA256SUMS`. To verify archive
provenance and its checksum:

```sh
gh attestation verify d2-v0.0.0-linux-amd64.tar.gz --repo d2lang/d2
sha256sum --check --ignore-missing SHA256SUMS
```

The MSI remains unsigned. Authenticode signing is tracked separately in
[issue #1078](https://github.com/d2lang/d2/issues/1078).

The installer uses WiX 7. Before the workflow can install or run WiX 7, a repository owner
must review the [WiX Open Source Maintenance Fee and EULA terms](https://docs.firegiant.com/wix/osmf/),
confirm that any required sponsorship is in place, and set the `WIX7_EULA_ACCEPTED`
repository variable to exactly `true`. That explicit owner-controlled gate enables the
workflow to pass `-acceptEula wix7`. Pull requests still build and smoke-test their release
archives when the variable is absent, but skip MSI packaging. Tag-triggered builds fail
closed when the variable is absent; no draft is uploaded.

## build.sh

- ./build.sh builds the release archives for each platform into ./build/<VERSION>/*.tar.gz
  Run with --help for usage.

Use `--host-only` to build only the release for the host's `$OS-$ARCH` pair. Local
development invocations still build locally. Production builds and artifact uploads belong
to the tag-triggered `Release archives` workflow; `RELEASE=1` local builds are rejected.

### Docker image helper

`./docker/build.sh` is retained as a load-only local development helper. It rejects
`--push`, `--latest`, and inherited `RELEASE` publishing. The release script no longer
calls it or publishes Docker tags from a legacy AWS SSH builder.

### Production Docker publishing

The manually dispatched `Publish Docker release` GitHub workflow is the production path
for Docker Hub. It replaces the Docker portion of the legacy AWS release path; the Windows
MSI is built separately by the `Windows MSI` workflow described above.

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

The workflow uses a native amd64/arm64 matrix and passes per-platform digest artifacts to
its publication job. PR image smoke tests call the same build and smoke script. Release
orchestration scripts come from the protected dispatch commit; a separate checkout supplies
the Dockerfile and entrypoint from the exact release commit.

Before publishing a production tag, the workflow:

1. verifies the protected-master dispatch, published non-draft semver GitHub release, exact
   Linux amd64 and arm64 asset IDs, and their GitHub SHA-256 digests;
2. explicitly peels the Git tag to a commit, requires that commit to be an ancestor of the
   dispatched `master` commit, and uses that release commit's Dockerfile without modification;
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
