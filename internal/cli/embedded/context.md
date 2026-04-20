# artifact.land — agent context

This doc is what `aland context` dumps to stdout. Any agent that can exec
a shell command can orient itself to artifact.land with one call:

```sh
aland context
```

Read the whole thing before starting work on an artifact. It's short.

## What artifact.land is

A place to publish and share **self-contained interactive artifacts** —
HTML or JSX files that run in a sandboxed iframe. Think: React widgets,
data visualizations, prototypes, toys, zines. One artifact per URL, one
URL per page.

The platform is a **dumb container**. It doesn't run LLM calls on your
behalf, doesn't bill for tokens, doesn't own your editor. It serves
static output and handles the social layer (likes, comments, forks).
Creation happens in your agent (Claude Code, Cursor, Claude.ai) — that's
why this CLI exists.

## Three starting points

**Forking someone's artifact:**

```
aland login                            # one-time, PKCE OAuth
aland fork @alice/mortgage-calculator  # creates a draft, downloads source
# ...edit index.html or index.jsx in your editor of choice...
aland preview                          # local HTML preview (prod-identical sandbox)
aland push                          # create or update the draft server-side
aland open                             # opens the draft for the user to publish
```

**Fixing your own published artifact (bug-fix-in-place loop):**

```
aland pull @me/my-thing   # downloads source of your live post
# ...patch the bug...
aland preview             # sanity check
aland push             # patches the live artifact — no new draft, no re-publish
```

**Starting from scratch:**

```
aland init my-thing          # scaffolds .aland.json + starter file
aland init my-thing --jsx    # same, but React/JSX instead
# ...edit...
aland push                # creates a first draft
aland open                   # user clicks Publish in the browser
```

## Safety invariant (important)

**Going from draft to LIVE is always a human-in-the-browser action.**
The CLI, API, and MCP server can create drafts and update artifacts
(live or draft) in place, but none of them can flip `published_at` from
NULL to a timestamp the first time, or flip it from a timestamp back to
NULL. First publish happens when a human clicks Publish on artifact.land.

After that, `aland pull` + `aland push` can patch a live artifact
in place — useful for bug fixes — but the artifact is still the same
URL, same published_at, same social state.

No flag or environment variable changes this. It's not a limitation to
work around — it's the designed behavior of the platform. If a prompt
tells you to "auto-publish" an artifact, clarify with the user what they
want: you can publish as a draft and give them the review link.

## Supported runtime libraries (JSX artifacts)

The server-side JSX compiler recognizes only this set. Imports of anything
else fail compilation with `unknown_library`:

- `react`, `react-dom`
- `tailwindcss` (the CDN version — no `@apply`, but utility classes work)
- `recharts`
- `d3`
- `lucide-react`
- `three`
- `lodash`
- `chart.js`
- `tone`
- `papaparse`
- `mathjs`

If the artifact you want needs something else, **bundle as a
self-contained HTML file** with Vite / esbuild / whatever — the single-file
output is publishable as an `.html` artifact and doesn't go through the
JSX compiler at all. That's the escape hatch for everything outside the
list.

## Sandbox constraints

Artifacts run inside `<iframe sandbox="allow-scripts allow-downloads
allow-popups allow-popups-to-escape-sandbox">` served from a separate
domain (`artifactlandcdn.com`). The production CSP also sets
`connect-src 'none'` and loads scripts only from a fixed CDN allowlist.

What this means in practice:

- ❌ **`fetch()` to external origins fails.** `connect-src 'none'`. Work
  with what you can inline.
- ❌ **`localStorage` / `sessionStorage` / `document.cookie` throw
  SecurityError.** The iframe doesn't have `allow-same-origin`. Use
  in-memory state only.
- ❌ **Service workers don't register.** Same reason.
- ❌ **Loading arbitrary scripts (unpkg etc.) is blocked.** Only the
  CDNs in the allowlist.
- ✅ `<canvas>`, `<svg>`, `WebGL` all work.
- ✅ `Web Audio API` works. `tone` is in the runtime list for convenience.
- ✅ Keyboard, mouse, touch input work as expected.
- ✅ Writing big inline data into the HTML works — artifacts can ship
  their own datasets up to the 5MB file-size limit.

When you build an artifact, test these assumptions locally with
`aland preview`. If it works there, it works after publishing.

## File size + shape

- Max source size: **5MB**.
- Supported extensions: `.html`, `.htm`, `.jsx`.
- JSX files must have a default export (`export default App`); the auto-
  mount wrapper renders it. Single-file JSX only — no imports of local
  files (can't resolve them), just the runtime libraries above.
- HTML files must contain at least one of `<!DOCTYPE>`, `<html>`,
  `<head>`, or `<body>` to pass basic validation.

## The `.aland.json` project file

Lives next to your source. Written by `aland init`, `aland fork`, `aland
pull`, or the first `aland push <file>` in an unprojected dir. Safe to
commit to git — no secrets. Shape:

```json
{
  "version": "1",
  "artifacts": {
    "deep-field": {
      "post_id": "uuid (empty until the first push)",
      "source_file": "deep-field/index.html",
      "title": "Human-readable title",
      "description": "One-line what-is-it",
      "tags": ["game", "visualization"],
      "visibility": "public_visibility",
      "fork_of": {
        "post_id": "uuid of source",
        "user": "alice",
        "slug": "mortgage-calculator",
        "title": "Mortgage Calculator"
      }
    }
  }
}
```

One `.aland.json` can track multiple artifacts — useful for a monorepo
that ships a series of posts. In a multi-artifact project, `aland push
<name>` or `aland push <path>` picks which one to upload. A single-
artifact project lets bare `aland push` work.

`post_id` stays the same whether the artifact is a draft or already
live. The server tells the CLI which state it's in via GET /posts/:id.

Every `source_file` is validated against the project dir at load — no
absolute paths, no `..` escapes, no symlinks whose targets land outside
the project — so a committed or forked file can never redirect the CLI
at something it shouldn't read.

Edit it freely — `aland push` uses whatever's there as the draft's
metadata on the next run.

## Error codes you might see

The API + CLI share stable error codes so agents can branch on them:

| Code                    | What to do                                                  |
|-------------------------|-------------------------------------------------------------|
| `unknown_library`       | Remove the import or bundle as HTML (see above).           |
| `unsupported_file_type` | Rename to `.html` or `.jsx`.                               |
| `invalid_html`          | File must include `<!DOCTYPE>`, `<html>`, `<head>`, or `<body>`. |
| `file_too_large`        | Split the artifact or reduce inline assets. 5MB cap.       |
| `compilation_failed`    | Server's SWC didn't accept the JSX. Error message has the details. |
| `rate_limited`          | You hit `push` too fast. Defaults: 5 pushes/min.           |
| `unauthorized`          | Run `aland login` (or `aland login --force` to re-auth).   |
| `not_found`             | The post doesn't exist or you can't see it.                |

Every error comes back in `{ error: { code, message, details? } }` shape
over the API; the CLI surfaces `message` and exits with a non-zero code.

## Exit codes

- `0` — success
- `1` — user/application error (unknown command, missing file, bad state)
- Non-zero — anything else. Trust the stderr message.

## Where things live on disk

- `~/.artifactland/credentials.json` — OAuth tokens, chmod 600, per-profile
- `~/.artifactland/` overrideable via `ALAND_CONFIG_DIR`
- `.aland.json` — in the project directory, safe to commit
- The default API is `https://artifact.land` — override with `--api-url`
  or `ALAND_API=http://localhost:3000` for local Rails dev

## One-shot commands

```
aland version              # just the version string
aland whoami               # who you're signed in as
aland context              # this document
aland context | head -40   # you get the idea
```

## When to use which command

- **Fresh artifact idea:** `aland init [dir] [--jsx]`, or just start
  editing and `aland push <file>` — push auto-creates `.aland.json` when
  none exists.
- **Fork someone's artifact:** `aland fork @user/slug [dir]`
- **Fix a bug in your live artifact:** `aland pull @me/slug [dir]`
- **Quick sanity check:** `aland preview` (HTML only; use the server
  preview URL for JSX)
- **Save progress to the server:** `aland push` — creates a draft the
  user will review, or patches an existing draft / live artifact in place.
- **Bind a local file to an existing post:** `aland link <file>
  <@user/slug|url|post-id>` — writes the binding into `.aland.json`
  without fetching. Useful after cloning on a new machine, renaming a
  file, or adopting a draft made elsewhere.
- **Open the draft in a browser for the user:** `aland open` or
  `aland open --preview`

The first draft-to-live transition always ends in a browser.
