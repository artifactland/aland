# Releasing the aland CLI

Semver + tag-driven. One `git tag && git push --tags` does everything.

## Prereqs (one-time)

- `artifactland/homebrew-tap` repo created with a default branch.
- GitHub Actions secret `HOMEBREW_TAP_TOKEN`: a PAT with `repo` scope on
  the tap repository so goreleaser can push formula updates.
- `cosign` keyless signing is wired through the `id-token: write`
  permission in the release workflow — no key management needed.

## Cutting a release

```sh
# Make sure main is green.
gh run list --workflow=ci.yml -L 1

# Bump CHANGELOG + version check.
vim CHANGELOG.md

# Tag + push.
git tag v0.1.0
git push origin v0.1.0
```

The `release` workflow will:

1. Check out at the tag.
2. Run `go test ./...` — hard blocker on failure.
3. Invoke goreleaser with the config at `.goreleaser.yaml`.
4. Cross-compile for darwin arm64+amd64, linux arm64+amd64, windows amd64.
5. Package archives + a `checksums.txt`, signed with cosign keyless.
6. Create a GitHub Release and attach everything.
7. Open (or update) the Homebrew formula in `artifactland/homebrew-tap`.

## Verifying after release

```sh
brew update
brew install artifactland/tap/aland
aland version
```

Or a one-liner download + checksum check:

```sh
curl -sSL https://github.com/artifactland/aland/releases/download/v0.1.0/checksums.txt
curl -sSL https://github.com/artifactland/aland/releases/download/v0.1.0/aland_0.1.0_Darwin_arm64.tar.gz | shasum -a 256
# Compare the second digest against checksums.txt.
```

## Yanking a release

Ideally: ship a follow-up release that fixes the issue and let Homebrew
users auto-upgrade. If we need to actually yank:

```sh
gh release delete v0.1.0 --cleanup-tag
# Then push a superseding tag, e.g. v0.1.1.
```

The Homebrew formula still references the yanked version until a new
release is cut — users who already installed won't lose the binary, but
fresh installs will fail until the tap updates. Avoid this situation.
