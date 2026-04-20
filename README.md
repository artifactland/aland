# aland

Command-line companion for [artifact.land](https://artifact.land). Fork
someone else's artifact, edit it in your agent of choice, preview in a
production-identical sandbox, publish as a draft.

## Install

Coming soon — Homebrew tap + signed binaries on GitHub Releases.

While the CLI is still being built:

```sh
git clone https://github.com/artifactland/aland.git
cd aland
make build
./bin/aland version
```

## Why a CLI at all?

People make artifacts inside AI coding agents (Claude Code, Cursor). Today
the bridge from "the artifact exists in my agent" to "the artifact lives at
a URL" is a manual download + drag + drop. `aland` lets agents hand their
output directly to the platform — `aland fork @alice/thing` pulls the
source into a directory, the agent edits, `aland push` creates a draft,
and the human clicks Publish in their browser.

## Design notes

- **Go + stdlib + Cobra + Lip Gloss.** Single signed binary, no runtime.
- **Draft-only writes.** `aland push` always creates a draft. Publishing
  live is a web-session action; there's no flag, subcommand, or API path
  that bypasses this.
- **Agent-friendly output.** Human output goes to stderr, structured data
  to stdout, `--json` available on every data-emitting command, no
  interactive prompts when stdin isn't a TTY.

## Development

```sh
make build    # compile to bin/aland
make test     # run go test ./...
make lint     # go vet
```

Override the API base with `ALAND_API=http://localhost:3000` or the
`--api-url` flag for local Rails development.
