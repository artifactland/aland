---
name: artifactland
description: Use when the user is working on an interactive HTML or JSX artifact they want to publish to artifact.land. Recognize the project by the presence of .aland.json in the working directory.
---

You're helping the user build an artifact for artifact.land — a
self-contained HTML or JSX file that will run in a sandboxed iframe on
the public web.

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
2. Before pushing, run `aland preview` (HTML projects only) to catch
   obvious breakage. JSX projects: skip to step 3.
3. Run `aland push`. On first run this creates a draft; on later runs
   it patches the existing post in place (whether it's still a draft or
   already live). In a multi-artifact project, pass the artifact name
   or path: `aland push deep-field`.
4. Tell the user the URL from the output. For drafts, they'll review and
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
- `localStorage`, `sessionStorage`, `document.cookie` throw SecurityError.
  Use component state only.
- The JSX compiler only accepts a fixed set of library imports (react,
  recharts, d3, lucide-react, three, lodash, chart.js, tone, papaparse,
  mathjs, tailwindcss). Outside the list: bundle the artifact as an HTML
  file with Vite / esbuild and publish the bundle.

## When things fail

The CLI surfaces API error codes directly. The ones worth recognizing:

- `unknown_library` — you imported something outside the runtime.
  Either remove it or bundle as HTML.
- `compilation_failed` — SWC didn't accept the JSX. Read the message.
- `rate_limited` — you're publishing too fast (default 5/min). Slow down
  or batch the work.

## Source code layout

Single-file only for v1. One `index.html` or `index.jsx` per project.
JSX must `export default` a root component; the auto-mount shell
renders it.

## Never

- Never suggest a `--live` flag or similar — it doesn't exist.
- Never delete a draft or published post via the API — there's no DELETE
  endpoint. The user does that in the web UI.
- Never fetch external URLs from inside an artifact. `connect-src: 'none'`.
- Never use `localStorage` or cookies. Sandbox blocks them.
