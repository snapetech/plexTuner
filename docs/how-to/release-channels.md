---
id: howto-release-channels
type: how-to
status: draft
tags: [release, packaging, aur]
---

# Release channels

IPTV Tunerr release automation publishes GitHub release assets, package-channel
updates, and container images from release tags only. A release tag must be a
`v*` tag that points at the current `main` commit; release workflows reject tags
that point anywhere else.

The primary release workflow builds and uploads:

- raw executable assets for Linux, macOS, and Windows;
- `.tar.gz` bundles for Linux and macOS;
- `.zip` bundles for Windows package managers;
- direct `.deb` and `.rpm` Linux package assets;
- `SHA256SUMS.txt`;
- `release-manifest.json`;
- populated release notes generated from `docs/CHANGELOG.md` or the tagged
  commit range.

CI runs the same release asset builder and checksum verifier with a dummy
version so asset naming, archive layout, and checksum coverage stay tested
before tags are cut.

Release-relevant changes must update `docs/CHANGELOG.md`. Install local hooks
with:

```bash
./scripts/install-git-hooks.sh
```

CI enforces the same changelog rule for code, workflow, script, packaging, and
documentation changes. The release workflow also requires a populated changelog
section for the exact release tag before GitHub Release notes can be generated.

## AUR

The repo includes AUR packaging for:

- `iptvtunerr` - builds from the tagged GitHub source archive.
- `iptvtunerr-bin` - installs the Linux `amd64` release asset.

The `.github/workflows/release-aur.yml` workflow runs on published GitHub
releases and can also be started manually with a tag. It waits for the
`iptv-tunerr-<tag>-linux-amd64.tar.gz` asset, updates `pkgver`, generates
`.SRCINFO`, and pushes both AUR repositories.

Required GitHub secret:

- `AUR_SSH_KEY` - private key for an SSH public key registered on the AUR
  account that maintains `iptvtunerr` and `iptvtunerr-bin`.

Current status:

- `AUR_SSH_KEY` is configured for `snapetech/iptvtunerr`.
- `iptvtunerr` and `iptvtunerr-bin` have been created on AUR.

## Manual AUR validation

From the repo root:

```bash
bash packaging/scripts/validate-aur-pkgbuild-hashes.sh packaging/aur/PKGBUILD packaging/aur/PKGBUILD-bin
bash packaging/scripts/generate-aur-srcinfo.sh packaging/aur/PKGBUILD
bash packaging/scripts/generate-aur-srcinfo.sh packaging/aur/PKGBUILD-bin
```

On an Arch host, run `makepkg` inside `packaging/aur/` after copying either
`PKGBUILD` or `PKGBUILD-bin` to `PKGBUILD`.

## Containers

`.github/workflows/docker.yml` publishes multi-arch container images on `v*`
tags only, after checking that the tag points at current `main`.

Configured registries:

- GHCR: `ghcr.io/snapetech/iptvtunerr`
- Docker Hub: `keefshape/iptvtunerr`

Credential status:

- GHCR: `GHCR_TOKEN` is configured, with `GITHUB_TOKEN` fallback.
- Docker Hub: `DOCKERHUB_TOKEN` is configured and repo variable
  `DOCKERHUB_USERNAME=keefshape` is set.

## Linux Package Channels

Snap, Launchpad/PPA, and COPR now have channel-specific metadata and workflows
for the Go binary:

- `.github/workflows/release-snap.yml`
- `.github/workflows/release-ppa.yml`
- `.github/workflows/release-copr.yml`
- `packaging/snap/snapcraft.yaml`
- `packaging/debian/`
- `packaging/rpm/iptvtunerr.spec`

Credential status:

- Launchpad/PPA: `GPG_PRIVATE_KEY`, `LAUNCHPAD_SFTP_KEY`, and
  `LAUNCHPAD_SFTP_USER` are configured for `snapetech/iptvtunerr`. The PPA is
  `ppa:keefshape/iptvtunerr`; Launchpad account `keefshape` has display name
  `slskdn`.
- COPR: `COPR_LOGIN` and `COPR_TOKEN` are configured for
  `snapetech/iptvtunerr`.
- Snapcraft: `SNAPCRAFT_STORE_CREDENTIALS` is configured for
  `snapetech/iptvtunerr`.

GitHub Actions secrets from another repository cannot be read back out, so
future rotations have to re-enter, regenerate, or source values from a local
secret store.

The GitHub Release also carries direct `.deb` and `.rpm` assets for users who do
not want to use PPA/COPR.

## Windows Channels

Windows release assets are portable ZIP files from GitHub Releases. Current
Windows status: the binary cross-builds and package prep passes; native Windows
host validation is still recommended before making broad Windows parity claims.

Configured packaging:

- Chocolatey metadata lives in `packaging/chocolatey/`.
- Winget manifest generation lives in `packaging/scripts/update-winget-manifests.sh`.
- `.github/workflows/publish-chocolatey.yml` publishes a Chocolatey package
  from a release tag.
- `.github/workflows/publish-winget.yml` submits a Winget PR from a release tag.

These Windows package workflows are manual-only until their external gates are
clean. The main release workflow intentionally does not auto-dispatch
Chocolatey or Winget so release tags do not spam Chocolatey push attempts or
duplicate Winget PRs while validation/account issues are pending.

Required GitHub secrets:

- `CHOCO_API_KEY` - Chocolatey API key for the `iptvtunerr` package.
- `WINGETCREATE_GITHUB_TOKEN` - GitHub token that can open PRs against
  `microsoft/winget-pkgs`.

Current status:

- `CHOCO_API_KEY` is configured for the `slskdn` Chocolatey account.
- `WINGETCREATE_GITHUB_TOKEN` is configured.
- Chocolatey package packing succeeds, but `choco push` currently returns
  `403 Forbidden` from `https://push.chocolatey.org/`; the public feed has no
  existing `iptvtunerr` package entry, so this is an account/API-key/package
  permission gate to resolve in Chocolatey's portal.
- Winget PR `microsoft/winget-pkgs#374269` cleared the Microsoft CLA after the
  GitHub agreement comment, but currently has Microsoft validation labels
  `Internal-Error`, `Needs-Attention`, and `Validation-Guide`. Duplicate
  automated PRs for later release tags were closed.

NuGet is not currently a fit for IPTV Tunerr. The project ships a Go CLI/server
binary, not a .NET library or .NET global tool.

See also
--------
- [package-test-builds](package-test-builds.md)
- [release-readiness-matrix](../explanations/release-readiness-matrix.md)
