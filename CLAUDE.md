# CLAUDE.md

Guidance for Claude Code sessions working in this repo.

## Project overview

`aland` is the command-line companion for [artifact.land](https://artifact.land).
Users install via Homebrew, authenticate via PKCE loopback OAuth, and
then use the CLI to fork, edit, preview, and push artifacts as drafts.

**Publishing live is always a web-session action.** The CLI can create
drafts and PATCH existing posts (drafts or live), but it cannot flip
`published_at` from NULL to a timestamp on a first publish. That's a
deliberate safety property enforced both in the CLI and on the server.

This repo is CLI-only. The server, Worker, and infrastructure live in
a separate codebase that isn't open-source. Coordinated changes between
the two are uncommon; when they come up, it's two commits in two repos.

## Stack

- Go 1.25, stdlib-forward
- Cobra for the command tree, Lip Gloss for TTY output
- No TUI libs or interactive prompts — agents are a first-class caller
- goreleaser + GitHub Actions for cross-compile + Homebrew tap

## Common commands

```bash
make build       # compile to ./bin/aland
make test        # go test ./...
make vet         # go vet
make fmt         # gofmt -w -s .
```

Targeted runs:
```bash
go test ./internal/cli/                 # one package
go test ./... -run TestPush -v          # by test name
go test -race ./...                     # with race detector
```

Binary against staging or local dev:
```bash
./bin/aland --api-url https://staging.artifact.land whoami
ALAND_API=http://localhost:3000 ./bin/aland login
```

## Architecture

- `cmd/aland/main.go` — entry point; version / commit / build-date
  baked in via `-X main.X=...` ldflags at release time.
- `internal/cli/` — one file per command (`push.go`, `pull.go`, `fork.go`,
  `link.go`, `preview.go`, `login.go`, …); `root.go` wires them.
- `internal/project/` — `.aland.json` schema + loader + path validator.
  Every `source_file` is validated against the project dir on load
  and save.
- `internal/config/` — `~/.artifactland/credentials.json`, 0600, per
  profile; `authedClient` transparently refreshes expired tokens.
- `internal/oauth/` — PKCE loopback client against the Doorkeeper
  provider at artifact.land.
- `internal/api/` — typed REST client against `/api/v1/*`.
- `internal/preview/` — local HTTP server with prod-identical CSP /
  sandbox / permissions-policy.
- `internal/ui/` — Lip Gloss styles; TTY-aware, `NO_COLOR` respected.

Embedded assets:
- `internal/cli/templates/` — `starter.html` + `starter.jsx` for `aland init`
- `internal/cli/embedded/context.md` — agent orientation dumped by `aland context`
- `internal/cli/embedded/SKILL.md` — Claude Code skill installed by `aland skill install`

## Release process

Tag-driven. Everything else is automatic.

```bash
# 1. Rename ## Unreleased → ## vX.Y.Z — YYYY-MM-DD in CHANGELOG.md
# 2. Commit, push, confirm CI is green
# 3. Tag + push
git tag v0.1.0
git push origin v0.1.0
# 4. Watch
gh run watch -R artifactland/aland
```

On tag push `.github/workflows/release.yml` runs `go test`, then
goreleaser, which:
- Cross-compiles Darwin (amd64 + arm64), Linux (amd64 + arm64), Windows amd64
- Packages each archive with LICENSE + README
- Writes `checksums.txt`, signs with cosign keyless (sigstore OIDC via
  the workflow's `id-token: write` permission)
- Creates a GitHub Release
- Commits a Homebrew cask to `artifactland/homebrew-tap`

User install:
```
brew install artifactland/tap/aland
```

### Local dry-run

```bash
goreleaser release --snapshot --clean --skip=sign
```

`--skip=sign` is required locally — cosign needs an OIDC provider, and
only GitHub Actions has one via `id-token: write`. On your laptop cosign
falls back to an interactive device-code flow that times out after 5
minutes. The dry-run exercises build + archive + checksum + cask
generation; CI exercises signing for real.

Check `dist/` afterward — you should see five archives, `checksums.txt`,
per-archive metadata, and `dist/homebrew/Casks/aland.rb` with correct
GitHub URLs and SHA256s.

### Rollback

```bash
gh release delete v0.1.0 --cleanup-tag -R artifactland/aland
# Then cut a follow-up tag, e.g. v0.1.1. Never reuse a tag.
```

Don't tag v0.1.0 on a Friday afternoon.

## Invariants — don't regress

- **Draft-only writes.** No CLI flag, subcommand, or API path flips
  `published_at` from NULL on first publish. Enforced server-side too.
- **Strict `source_file` validation.** Untrusted `.aland.json` files
  must not redirect the CLI at `/etc/passwd`, `~/.ssh/id_rsa`, or any
  path outside the project dir. The validator in
  `internal/project/project.go` rejects absolute paths, `..` escapes,
  and symlinks whose targets escape root.
- **Credentials file refuses loose perms.** `config.Load` rejects
  anything looser than 0600 on Unix. Don't add a bypass flag.
- **Every data-emitting command has `--json`.** Agents parse output;
  keep the structured form stable.
- **Stderr for human chrome, stdout for data.** Don't let
  progress/info lines land on stdout — it breaks pipelines.
