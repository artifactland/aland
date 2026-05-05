# aland CLI changelog

Notable user-visible changes. Follows [semver](https://semver.org).

## v0.1.2 — 2026-05-05

### Fixed

- `.aland.json` no longer stamps a default `visibility: public_visibility`
  on `aland init` and on the auto-bootstrap path of `aland push <file>`.
  That stale value used to ride along on every subsequent update and
  silently revert a post to Public after the user had bumped it to
  Private via the web. The field is now omitted; the server picks the
  default on create and preserves whatever the post currently has on
  every update.
- `aland login` is now self-healing on an existing profile:
  - **Fresh token** → no-op, prints a friendly "Already signed in as @…"
  - **Expired but refreshable** → silent refresh + persist new pair
  - **Refresh token also dead** → falls through to the full PKCE browser
    flow, no `aland logout` required first
  - `--force` still skips all shortcuts and runs full PKCE
- 401 from any API call now surfaces as a single, actionable message
  pointing at `aland login` — including a "in another terminal if you're
  inside an agent session" hint so a Claude Code (or other agent) caller
  knows to ask the user to re-auth without trying it itself.
- A hard OAuth error during silent token refresh (`invalid_grant`,
  `invalid_token`) short-circuits with the same clear message instead of
  letting a known-dead access token go to the API and cause a confusing
  downstream 401.
- Surface the new server-side `visibility_downgrade_blocked` error from
  `aland push` with a message that points at the likely cause (stale
  `.aland.json`) and the fix.

## v0.1.0

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
