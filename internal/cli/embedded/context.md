# artifact.land — agent context

This doc is what `aland context` dumps to stdout. Any agent that can exec
a shell command can orient itself to artifact.land with one call:

```sh
aland context
```

Read the whole thing before starting work on an artifact. It's short.

## What artifact.land is

A place to publish and share **self-contained interactive artifacts** —
an entry HTML or JSX file (plus optional images bundled alongside it)
that runs in a sandboxed iframe. Think: React widgets, data
visualizations, prototypes, toys, zines. One artifact per URL, one URL
per page.

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
allow-popups allow-popups-to-escape-sandbox allow-same-origin">` served
from a **per-artifact subdomain** of `artifactlandcdn.com` (each
artifact gets its own browser origin, isolated from the main app and
from every other artifact). The production CSP also sets
`connect-src 'none'` and loads scripts only from a fixed CDN allowlist.

What this means in practice:

- ❌ **`fetch()` to external origins fails.** `connect-src 'none'`. Work
  with what you can inline.
- ❌ **External `<img src="https://…">` fails.** `img-src 'self' data: blob:`.
  Bundle the image (see Bundles below) or inline as base64.
- ❌ **Service workers don't register.** `worker-src 'none'`.
- ❌ **Loading arbitrary scripts (random unpkg paths, etc.) is blocked**
  outside the CDN allowlist.
- ✅ `localStorage`, `sessionStorage`, `IndexedDB`, and cookies all
  work — each artifact has a unique origin, so storage is scoped to
  that one artifact. No cloud sync; storage is per-device, per-browser.
- ✅ `<canvas>`, `<svg>`, `WebGL` all work.
- ✅ `Web Audio API` works. `tone` is in the runtime list for convenience.
- ✅ Keyboard, mouse, touch input work as expected.
- ✅ Big inline data ships with the entry file — artifacts can carry
  their own datasets up to the bundle cap (see below).

When you build an artifact, test these assumptions locally with
`aland preview`. If it works there, it works after publishing.

## Bundles (entry file + assets)

An artifact is one **entry file** (`index.html` or `index.jsx`) plus
any images it references, packaged together. The two shapes:

- **Single file** — just the entry file, no separate assets. All
  content inline (base64 images, embedded SVG, CDN scripts).
- **Bundle** — entry file at the root plus images under `assets/`:

  ```
  my-artifact/
  ├── index.html
  └── assets/
      ├── hero.jpg
      ├── thumbnail.png
      └── icons/
          └── star.webp
  ```

Reference bundled assets with normal relative URLs in the entry file —
they're same-origin with the entry, so no CORS, no signed URLs in your
markup:

```html
<img src="assets/hero.jpg">
<source srcset="assets/photo-2x.webp 2x, assets/photo.webp 1x">
```

CSS `background: url(assets/...)` and `<a href="assets/file.png">` work
the same way. Nested subdirectories under `assets/` are fine.

**Allowed bundled file types (v1):** raster images only — `.png`,
`.jpg` / `.jpeg`, `.gif`, `.webp`, `.avif`. Fonts, audio, data, SVG, and
video are deferred to follow-up epics; inline them in the entry file for
now (`@font-face` with base64, inline `<svg>`, inline JSON in a
`<script type="application/json">`).

`aland push <directory>` zips the directory and uploads it as a bundle.
`aland push <file>` is the unchanged single-file path. The CLI runs
`aland validate` automatically as the first step (see below).

## `aland validate <directory>`

Offline bundle linter. No network calls. Checks:

1. **Structure** — entry file at the root, every other file under
   `assets/`. **Blocking** if violated.
2. **File types** — every file matches the allowlist. **Blocking** on
   disallowed extensions.
3. **Size + count** — reports against both free and Pro caps (the CLI
   can't know your tier offline). **Warning** if over the free cap,
   **blocking** if over the Pro cap.
4. **Reference integrity** — parses the entry file, finds every
   `src=`, `href=`, `url(...)`, `<source srcset="…">`, and CSS
   `background: url(…)`. **Blocking** if a relative reference points
   at a file that doesn't exist in the bundle. **Warning** on external
   URLs (they'll be blocked at runtime by `connect-src 'none'` and
   `img-src 'self' data: blob:`).
5. **Image weight** — **Warning** on any image > 500 KB. Suggests
   compression (`cwebp -q 80`, ImageOptim, squoosh.app).

`aland push` calls validate as step zero. Blocking issues fail the
push; warnings print but don't block.

## File size + shape

- **Bundle caps:** Free tier — **5 MB / 50 files**. Pro tier —
  **25 MB / 200 files**. Cap applies to the post owner.
- **Entry file extensions:** `.html`, `.htm`, `.jsx`.
- JSX files must have a default export (`export default App`); the
  auto-mount wrapper renders it. Single-file JSX only — no local-file
  imports (can't resolve them), just the runtime libraries above.
- HTML files must contain at least one of `<!DOCTYPE>`, `<html>`,
  `<head>`, or `<body>` to pass basic validation.
- **Private posts are single-file only for v1.** Bundles on private
  posts are refused with `bundles_not_allowed_for_private_posts` — the
  Worker serves asset paths unsigned, so a private bundle would leak via
  guessable asset URLs. Set visibility to public or link-only to use a
  bundle, or stay single-file for private artifacts.

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

| Code                                      | What to do                                                                |
|-------------------------------------------|---------------------------------------------------------------------------|
| `unknown_library`                         | Remove the import or bundle as HTML (see above).                          |
| `unsupported_file_type`                   | Entry must be `.html` or `.jsx`. Bundled files must be raster images.     |
| `invalid_html`                            | Entry HTML must include `<!DOCTYPE>`, `<html>`, `<head>`, or `<body>`.    |
| `file_too_large`                          | Single-file path exceeded the per-file cap.                               |
| `bundle_too_large`                        | Bundle exceeded the size cap. Free 5 MB / Pro 25 MB. Trim or upgrade.     |
| `bundle_too_large_for_tier`               | Forking a bundle that's larger than the forker's tier cap allows.         |
| `too_many_files`                          | Bundle has more than the file-count cap. Free 50 / Pro 200.               |
| `too_many_files_for_tier`                 | Forking a bundle with more files than the forker's tier allows.           |
| `invalid_bundle_structure`                | Entry must be at the root; every other file must live under `assets/`.    |
| `invalid_bundle`                          | Zip rejected (path traversal, symlink, or other shape problem).           |
| `invalid_zip`                             | The uploaded file isn't a valid zip.                                      |
| `missing_entry_file`                      | No `index.html` or `index.jsx` at the bundle root.                        |
| `invalid_file_content`                    | A bundled file's magic bytes don't match its extension.                   |
| `bundles_not_allowed_for_private_posts`   | v1 carve-out. Use a single file for private posts, or switch visibility.  |
| `compilation_failed`                      | Server's SWC didn't accept the JSX. Error message has the details.        |
| `rate_limited`                            | You hit `push` too fast. Defaults: 5 pushes/min.                          |
| `unauthorized`                            | Run `aland login` (or `aland login --force` to re-auth).                  |
| `not_found`                               | The post doesn't exist or you can't see it.                               |

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
- **Lint a bundle before uploading:** `aland validate <directory>` —
  offline structure / file-type / reference / image-weight checks.
  `aland push` runs this automatically as the first step.
- **Save progress to the server:** `aland push` — creates a draft the
  user will review, or patches an existing draft / live artifact in place.
  Pass a directory to upload a bundle, a single file for the single-file
  path. The CLI builds the zip when given a directory.
- **Bind a local file to an existing post:** `aland link <file>
  <@user/slug|url|post-id>` — writes the binding into `.aland.json`
  without fetching. Useful after cloning on a new machine, renaming a
  file, or adopting a draft made elsewhere.
- **Open the draft in a browser for the user:** `aland open` or
  `aland open --preview`

The first draft-to-live transition always ends in a browser.
