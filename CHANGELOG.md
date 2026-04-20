# aland CLI changelog

Notable user-visible changes. Follows [semver](https://semver.org).

### Added

- `aland login` / `logout` / `whoami` — PKCE loopback OAuth against the
  artifact.land Doorkeeper provider. Credentials stored per-profile at
  `~/.artifactland/credentials.json` (chmod 600, refuses loose perms).
  Expired access tokens refresh automatically on next command when a
  refresh token is present.
- `aland init` — scaffold a fresh artifact project (HTML or `--jsx`).
  Optional: `aland push <file>` auto-creates `.aland.json` when none
  exists, so init isn't a required step before the first push.
- `aland fork @user/slug` — create a draft server-side with
  `remix_of_id` set, pull the source locally, write `.aland.json`.
- `aland pull @user/slug` — download your own artifact's source and
  bind it into `.aland.json`.
- `aland link <file> <@user/slug | url | post-id>` — bind a local file
  to an existing post without fetching. Recovery path for after clones,
  renames, and web-side draft creation.
- `aland preview [name-or-path]` — local HTTP server with
  prod-identical CSP, sandbox, and permissions-policy for HTML
  artifacts. JSX preview is server-side only for v1.
- `aland push [name-or-path]` — create on first run, PATCH thereafter.
  Multi-artifact projects take a name (key in `.aland.json`) or a path
  to pick which artifact to push. Structured error handling for
  `unknown_library`, `compilation_failed`, `unsupported_file_type`,
  `file_too_large`, `rate_limited`.
- `aland status` — local + server-side project state, `--json` mode.
- `aland open [--preview]` — open the draft's web URL in the browser.
- `aland context` — dump the agent-orientation markdown to stdout.
- `aland skill install` — copy the bundled SKILL.md into
  `~/.claude/skills/artifactland/` for Claude Code users.
- `--json` on every data-emitting command; TTY-aware styling via
  Lip Gloss (colors auto-disabled when piped or `NO_COLOR=1`).

`.aland.json` is a multi-artifact project file (`artifacts` keyed by
name). Every `source_file` is validated at load and save against the
project dir — relative paths only, no `..` escapes, no symlinks whose
targets land outside — so a committed or forked `.aland.json` can
never redirect the CLI at files it shouldn't read.

### Architecture

- Go + cobra + lipgloss + stdlib. No TUI libs, no Huh — agents don't
  like interactive prompts.
- Single signed binary distributed via Homebrew tap (primary) + GitHub
  Releases (secondary). goreleaser handles cross-compile, archive,
  checksum, and cosign keyless signing.
