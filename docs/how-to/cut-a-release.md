# Cut a release

Publish a new Northwatch release. Pushing a `vX.Y.Z` tag triggers the release
workflow, which builds the multi-architecture binaries and `.deb`s, signs
everything with cosign, generates SBOMs, and creates the GitHub release. Your job
is to prepare the changelog and push a clean, signed tag; the workflow does the
rest.

Pick the next version from the existing tags (`git tag -l --sort=version:refname`)
and the released sections in `CHANGELOG.md`. Substitute your target version for
`X.Y.Z` throughout.

## Prepare the changelog

`CHANGELOG.md` keeps unreleased work under an `## [Unreleased]` heading. Promote
it to the new version:

1. Rename `## [Unreleased]` to `## [X.Y.Z] - <date>` (ISO date, e.g.
   `## [0.6.0] - 2026-07-13`).
2. Add a fresh, empty `## [Unreleased]` section above it for the next cycle.
3. Skim the entries for accuracy and wording.

Commit the changelog on its own:

```bash
git add CHANGELOG.md
git commit -m "release: v0.6.0"
```

## Tag and push

Create an annotated, signed tag on the release commit, then push it:

```bash
git tag -s v0.6.0 -m "v0.6.0"
git push origin v0.6.0
```

The `v*` tag push is what triggers the release workflow
(`.github/workflows/release.yml`). Pushing the tag is the point of no return, so
make sure the commit is the one you want released and CI is green first.

## What the workflow produces

Once the tag lands, the workflow runs automatically and attaches everything to a
new GitHub release:

- Multi-architecture binaries: `northwatch-linux-amd64`,
  `northwatch-linux-arm64`, and `northwatch-darwin-arm64`, each built with the
  release tag baked in via `-ldflags "-X main.version=vX.Y.Z"`.
- Debian packages: `northwatch_<version>_amd64.deb` and
  `northwatch_<version>_arm64.deb`, built with `nfpm` (Linux targets only).
- cosign signatures: every binary, every `.deb`, and `checksums.txt` are
  keyless-signed with cosign, producing a `.sig` and a `.pem` per file. The
  identity is the release workflow itself (OIDC via GitHub Actions).
- SBOMs: a Software Bill of Materials per binary in two formats, SPDX
  (`<name>.spdx.json`) and CycloneDX (`<name>.cdx.json`), generated with syft.
- `checksums.txt`: SHA-256 sums covering the binaries and `.deb`s.
- A GitHub release, created with `gh release create --generate-notes`, so
  the release notes are auto-generated from the merged pull requests since the
  previous tag.

Watch the run under the repository's Actions tab; when it finishes, the release
appears on the [releases
page](https://github.com/B42Labs/northwatch/releases).

## Verify the artifacts

The release is verifiable end to end. After downloading an artifact
and its `.sig`/`.pem`, confirm the checksum and the cosign signature.

Check the checksum:

```bash
sha256sum -c checksums.txt --ignore-missing
```

Verify a cosign signature (keyless; the certificate identity is the workflow):

```bash
cosign verify-blob \
  --certificate northwatch-linux-amd64.pem \
  --signature   northwatch-linux-amd64.sig \
  --certificate-identity-regexp '^https://github.com/B42Labs/northwatch/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  northwatch-linux-amd64
```

The same command verifies the `.deb`s and `checksums.txt`; swap the filenames.
The [Install on Debian/Ubuntu](/how-to/install-debian) guide shows the `.deb`
form.

Inspect an SBOM to confirm it lists the expected dependencies:

```bash
# SPDX
jq '.packages[].name' northwatch-linux-amd64.spdx.json | head

# CycloneDX
jq '.components[].name' northwatch-linux-amd64.cdx.json | head
```

## Related

- [Install on Debian/Ubuntu](/how-to/install-debian)
- [Make targets](/reference/make-targets)
