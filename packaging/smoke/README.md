# Package Smoke Validation

`packaging/smoke/package-smoke` validates post-release package channels by installing from the public channel and writing evidence:

```bash
packaging/smoke/package-smoke iptvtunerr container-ghcr v0.1.63
```

The driver emits `evidence.json`, `junit.xml`, and logs under `artifacts/package-smoke/`. It is intended for internal GitLab post-release validation and for disabled GitHub workflow scaffolding.

Channels currently wired for this project: `github-archive`, `deb`, `rpm`, `container-ghcr`, `container-dockerhub`, `aur`, `aur-bin`, `copr`, `ppa`, `snap`, `chocolatey`, and `winget`.
