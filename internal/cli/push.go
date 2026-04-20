package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/artifactland/aland/internal/api"
	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newPushCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push [name-or-path]",
		Short: "Create or update a draft on artifact.land",
		Long: `Reads the source file listed in .aland.json, uploads it to the API, and
creates or updates a draft. The server re-compiles on every run — if there
are unsupported imports or other compile errors, they surface here.

First run creates a draft and writes its id back into .aland.json; later
runs PATCH that same draft so you don't accumulate a graveyard of
one-per-iteration drafts.

In a multi-artifact project, pass the artifact's name (the key in the
.aland.json artifacts map) or its source file path to pick which one to
push. Bare ` + "`" + `aland push` + "`" + ` is fine for single-artifact projects.

With no .aland.json, ` + "`" + `aland push <file>` + "`" + ` scaffolds one around the file
and creates the draft — no separate ` + "`" + `aland init` + "`" + ` step required.

Publishing live always happens in the browser. The command prints the
draft URL; you review and click Publish there.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runPush,
	}
	return cmd
}

func runPush(cmd *cobra.Command, args []string) error {
	var query string
	if len(args) == 1 {
		query = args[0]
	}

	p, err := project.Load(".")
	if err != nil {
		if !errors.Is(err, project.ErrNotAProject) {
			return fmt.Errorf("loading project: %w", err)
		}
		// No .aland.json yet. If the user handed us a file path, treat
		// this as first-push and scaffold the project around it — no need
		// to run `aland init` separately. With no arg, we genuinely don't
		// know what to upload, so bail with a pointer.
		if query == "" {
			return fmt.Errorf("no .aland.json here — pass a source file path (e.g. `aland push index.html`), or run `aland init` first")
		}
		p, err = bootstrapProject(query)
		if err != nil {
			return err
		}
		ui.Info("Created .aland.json (%s)", project.Filename)
	}

	a, err := p.Resolve(query)
	if err != nil {
		return err
	}

	sourcePath := a.AbsSource(p.Dir())
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", sourcePath, err)
	}

	globals := Globals(cmd.Context())
	client, profile, err := authedClient(globals)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	attrs := artifactAttributes(a)
	var (
		post    *api.Post
		created bool
	)

	if a.PostID == "" {
		ui.Info("Creating a new draft on %s...", profile.APIBase)
		post, err = client.CreateDraft(ctx, a.SourceFile, content, attrs)
		if err != nil {
			return formatPushError(err)
		}
		created = true
	} else {
		ui.Info("Updating %s on %s...", a.PostID, profile.APIBase)
		post, err = client.UpdateDraft(ctx, a.PostID, a.SourceFile, content, attrs)
		if err != nil {
			return formatPushError(err)
		}
	}

	// Persist the post id back into .aland.json on first publish so repeat
	// runs PATCH the same post instead of creating new drafts.
	if a.PostID != post.ID {
		a.PostID = post.ID
		if err := p.Save(); err != nil {
			return fmt.Errorf("updating .aland.json: %w", err)
		}
	}

	isLive := post.PublishedAt != nil && *post.PublishedAt != ""

	if globals.JSON {
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"post_id":   post.ID,
			"url":       post.URLs.Web,
			"title":     post.Title,
			"created":   created,
			"published": isLive,
		})
		return nil
	}

	switch {
	case created:
		ui.Success("Draft created.")
	case isLive:
		ui.Success("Published artifact updated.")
	default:
		ui.Success("Draft updated.")
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "")
	if isLive {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  Live at:"))
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  Review and publish at:"))
	}
	fmt.Fprintln(cmd.OutOrStdout(), "  "+post.URLs.Web)
	return nil
}

// artifactAttributes pulls the metadata fields off the artifact in the
// shape the API expects under `post[...]`.
func artifactAttributes(a *project.Artifact) map[string]any {
	attrs := map[string]any{}
	if a.Title != "" {
		attrs["title"] = a.Title
	}
	if a.Description != "" {
		attrs["description"] = a.Description
	}
	if len(a.Tags) > 0 {
		attrs["tag_list"] = joinTags(a.Tags)
	}
	if a.Visibility != "" {
		attrs["visibility"] = a.Visibility
	}
	return attrs
}

func joinTags(tags []string) string {
	out := ""
	for i, t := range tags {
		if i > 0 {
			out += ", "
		}
		out += t
	}
	return out
}

// formatPushError turns a typed API error into a readable CLI message.
// unknown_library is the big one: point users at the specific libs they
// tried and how to resolve.
func formatPushError(err error) error {
	apiErr, ok := err.(*api.Err)
	if !ok {
		return err
	}
	switch apiErr.Code {
	case "unknown_library":
		libs, _ := apiErr.Details["libraries"].([]any)
		names := make([]string, 0, len(libs))
		for _, l := range libs {
			if s, ok := l.(string); ok {
				names = append(names, s)
			}
		}
		msg := fmt.Sprintf(
			"The artifact imports %s — not in the supported runtime.\n"+
				"  Either remove the import, or bundle your artifact as a self-contained HTML file.\n"+
				"  See `aland context` for the full supported library list.",
			formatList(names),
		)
		return ui.Errorf("%s", msg)
	case "compilation_failed":
		return ui.Errorf("Compilation failed:\n  %s", apiErr.Message)
	case "unsupported_file_type":
		return ui.Errorf("%s", apiErr.Message)
	case "file_too_large":
		return ui.Errorf("%s", apiErr.Message)
	case "rate_limited":
		return ui.Errorf("Rate limited. %s", apiErr.Message)
	default:
		return ui.Errorf("%s", apiErr.Error())
	}
}

// bootstrapProject creates a fresh .aland.json in cwd with a single artifact
// pointing at sourceFile. Called when `aland push <file>` runs in a dir
// that doesn't have a project yet — removes the need to `aland init` first
// when iterating with an agent.
func bootstrapProject(sourceFile string) (*project.Project, error) {
	cleaned := filepath.Clean(sourceFile)
	content, err := os.ReadFile(cleaned)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sourceFile, err)
	}
	p := project.New(".")
	if err := p.Add(deriveArtifactName(cleaned), &project.Artifact{
		SourceFile: filepath.ToSlash(cleaned),
		Title:      inferHTMLTitle(content),
		Visibility: "public_visibility",
	}); err != nil {
		return nil, fmt.Errorf("scaffolding project: %w", err)
	}
	if err := p.Save(); err != nil {
		return nil, fmt.Errorf("writing %s: %w", project.Filename, err)
	}
	return p, nil
}

// deriveArtifactName picks a reasonable key for a newly-bootstrapped
// artifact based on its source file. Filenames like "index.html" and
// "main.jsx" are generic, so we prefer the containing directory's name
// when available; otherwise the basename is fine.
func deriveArtifactName(sourceFile string) string {
	cleaned := filepath.Clean(sourceFile)
	base := strings.TrimSuffix(filepath.Base(cleaned), filepath.Ext(cleaned))
	dir := filepath.Base(filepath.Dir(cleaned))
	if base == "index" || base == "main" {
		if dir != "" && dir != "." && dir != string(filepath.Separator) {
			return dir
		}
		return project.DefaultArtifactName
	}
	if base == "" || base == "." {
		return project.DefaultArtifactName
	}
	return base
}

// htmlTitleRe pulls the text between the first <title>…</title>. Good enough
// for the common case of a hand-authored artifact; not a full parser.
var htmlTitleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func inferHTMLTitle(content []byte) string {
	m := htmlTitleRe.FindSubmatch(content)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}

func formatList(items []string) string {
	switch len(items) {
	case 0:
		return "an unsupported library"
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		head := items[:len(items)-1]
		out := ""
		for i, h := range head {
			if i > 0 {
				out += ", "
			}
			out += h
		}
		return out + ", and " + items[len(items)-1]
	}
}
