---
name: artifactland
description: Use when the user is working on an interactive HTML or JSX artifact (single file or a bundle with images under assets/) they want to publish to artifact.land. Recognize the project by the presence of .aland.json in the working directory.
---

You're helping the user build an artifact for artifact.land — a
self-contained interactive thing (HTML or JSX entry file, optionally
with bundled images under `assets/`) that will run in a sandboxed
iframe on the public web.

## Recognize the project

If `.aland.json` exists in the working directory, you're in an
artifactland project. Read it. Shape:

```json
{
  "version": "1",
  "artifacts": {
    "<name>": {
      "source_file": "path/relative/to/project.html",
      "post_id": "set after first push",
      "fork_of": { "user": "...", "slug": "..." }
    }
  }
}
```

One `.aland.json` can hold multiple artifacts. In a monorepo of posts,
each key names one and the commands take the name (or the path) to pick
which to act on.

Key fields:

- `source_file` — the file you're editing for that artifact
- `post_id` — set after the first push; subsequent `aland push` runs
  PATCH the same post (draft or live)
- `fork_of` — set when the artifact was started via `aland fork`

## The workflow

1. Read the source file. Make the changes the user asked for.
2. If the project ships with a bundle (entry file + an `assets/`
   directory), run `aland validate <directory>` to lint the bundle
   structure, file types, and reference integrity before push. Skip on
   single-file projects.
3. Before pushing, run `aland preview` (HTML projects only) to catch
   obvious breakage. JSX projects: skip to step 4.
4. Run `aland push`. On first run this creates a draft; on later runs
   it patches the existing post in place (whether it's still a draft or
   already live). In a multi-artifact project, pass the artifact name
   or path: `aland push deep-field`. `aland push` runs `validate`
   automatically as step zero, so step 2 is "do it earlier and read the
   warnings" rather than a separate gate.
5. Tell the user the URL from the output. For drafts, they'll review and
   click Publish. For live posts, the fix is already in production.

If `.aland.json` doesn't exist yet and the user wants to publish a file
you just wrote, `aland push <file>` scaffolds the project around it —
no separate `aland init` step needed.

**You can never take a draft live.** `aland push` creates drafts on
first run; the user flips them live by clicking Publish on artifact.land.
After that, you can pull and patch in place — but you can't cause the
first publish, and you can't un-publish. Those are deliberate safety
properties, not limitations.

## Before suggesting code

Run `aland context` once at the start of a session. It prints the full
runtime and sandbox constraints. The most common footguns:

- `fetch()` to external origins fails — `connect-src 'none'` in
  production. Don't write artifacts that phone home.
- External `<img src="https://…">` is blocked — `img-src 'self' data:
  blob:`. Bundle the image under `assets/` and reference it relatively
  (`<img src="assets/hero.jpg">`), or inline as base64.
- The JSX compiler only accepts a fixed set of library imports (react,
  recharts, d3, lucide-react, three, lodash, chart.js, tone, papaparse,
  mathjs, tailwindcss). Outside the list: build a self-contained HTML
  file with Vite / esbuild and publish that.

`localStorage`, `sessionStorage`, `IndexedDB`, and cookies all work,
scoped to the artifact's own subdomain origin. Storage is per-device and
per-browser — no cloud sync.

## When things fail

The CLI surfaces API error codes directly. The ones worth recognizing:

- `unknown_library` — you imported something outside the runtime.
  Either remove it or bundle as HTML.
- `compilation_failed` — SWC didn't accept the JSX. Read the message.
- `rate_limited` — you're publishing too fast (default 5/min). Slow down
  or batch the work.

## Source code layout

An artifact is either a single entry file or a small bundle:

```
my-artifact/
├── index.html         ← entry (HTML or JSX) at the root
└── assets/            ← optional, raster images only for v1
    ├── hero.jpg
    └── thumbnail.png
```

- One entry file per project: `index.html` or `index.jsx` at the root.
- JSX must `export default` a root component; the auto-mount shell
  renders it. Single-file JSX only — no local-file imports, just the
  runtime libraries.
- Bundled assets live under `assets/` (subdirectories fine). Reference
  them with normal relative URLs: `<img src="assets/hero.jpg">`. They're
  same-origin with the entry, so no CORS, no signed URLs in markup.
- v1 allows raster images only in `assets/` (`.png`, `.jpg` / `.jpeg`,
  `.gif`, `.webp`, `.avif`). Fonts, audio, data, SVG → inline in the
  entry file for now.
- Caps: Free 5 MB / 50 files. Pro 25 MB / 200 files. Cap applies to the
  post owner.
- Private posts are single-file only for v1.

## Never

- Never suggest a `--live` flag or similar — it doesn't exist.
- Never delete a draft or published post via the API — there's no DELETE
  endpoint. The user does that in the web UI.
- Never fetch external URLs from inside an artifact. `connect-src: 'none'`.
- Never hotlink external images. Bundle them under `assets/` and
  reference relatively. `img-src 'self' data: blob:` blocks remote URLs.
- Never bundle a private artifact. v1 carve-out — private posts are
  single-file only. The API returns `bundles_not_allowed_for_private_posts`.
