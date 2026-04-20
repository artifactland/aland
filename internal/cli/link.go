package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newLinkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link <file> <@user/slug | url | post-id>",
		Short: "Bind a local file to an existing artifact without fetching",
		Long: `Records in .aland.json that <file> corresponds to an artifact that already
exists on the server. The local file is untouched; the server is only read
(to confirm the artifact exists and pick up its title/tags).

Useful when:
  - you cloned a repo on a new machine and need to rebind an individual file
  - you wrote a file locally and separately created a draft on the web
  - you renamed a source file and want to update the binding

This is the inverse of ` + "`" + `aland pull` + "`" + ` — same end state, but you bring the file.`,
		Args: cobra.ExactArgs(2),
		RunE: runLink,
	}
	return cmd
}

func runLink(cmd *cobra.Command, args []string) error {
	sourceFile := args[0]
	ref := args[1]

	cleaned := filepath.Clean(sourceFile)
	if _, err := os.Stat(cleaned); err != nil {
		return fmt.Errorf("%s: %w", sourceFile, err)
	}

	globals := Globals(cmd.Context())
	client, _, err := authedClient(globals)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()

	// Anything that parses as @user/slug goes through the name-based endpoint;
	// bare strings with no slash are treated as a post id. Pasted URLs work
	// too — we strip the host/scheme and parse the path.
	normalizedRef := stripRefURL(ref)
	var (
		postID string
		title  string
		tags   []string
		vis    string
		userN  string
		slug   string
	)
	if strings.Contains(normalizedRef, "/") {
		username, s, perr := parseRef(normalizedRef)
		if perr != nil {
			return perr
		}
		post, perr := client.GetPostByRef(ctx, username, s)
		if perr != nil {
			return fmt.Errorf("looking up @%s/%s: %w", username, s, perr)
		}
		postID, title, tags, vis = post.ID, post.Title, post.Tags, post.Visibility
		userN, slug = post.User.Username, post.Slug
	} else {
		post, perr := client.GetPost(ctx, normalizedRef)
		if perr != nil {
			return fmt.Errorf("looking up %s: %w", normalizedRef, perr)
		}
		postID, title, tags, vis = post.ID, post.Title, post.Tags, post.Visibility
		userN, slug = post.User.Username, post.Slug
	}

	// Load the existing project or create a fresh one if this is a new dir.
	p, err := project.Load(".")
	if err != nil {
		if !errors.Is(err, project.ErrNotAProject) {
			return fmt.Errorf("loading project: %w", err)
		}
		p = project.New(".")
	}

	name := uniqueArtifactName(p, deriveArtifactName(cleaned), filepath.ToSlash(cleaned))
	if err := p.Add(name, &project.Artifact{
		PostID:     postID,
		SourceFile: filepath.ToSlash(cleaned),
		Title:      title,
		Tags:       tags,
		Visibility: vis,
	}); err != nil {
		return err
	}
	if err := p.Save(); err != nil {
		return err
	}

	ui.Success("Linked %s → @%s/%s", cleaned, userN, slug)
	return nil
}

// stripRefURL turns "https://artifact.land/@scott/deep-field" into
// "@scott/deep-field" so parseRef can handle it. Anything non-URL passes
// through unchanged.
func stripRefURL(ref string) string {
	if !strings.HasPrefix(ref, "http://") && !strings.HasPrefix(ref, "https://") {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return strings.TrimPrefix(u.Path, "/")
}

// uniqueArtifactName picks a non-colliding key for a new artifact. If the
// desired key already points at the same file path, we reuse it (re-linking
// the same file is an update, not a duplicate). If it points at a different
// file, we append -2, -3, … so both survive.
func uniqueArtifactName(p *project.Project, wanted, sourceFile string) string {
	existing := p.Get(wanted)
	if existing == nil {
		return wanted
	}
	if filepath.ToSlash(filepath.Clean(existing.SourceFile)) == sourceFile {
		return wanted
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", wanted, i)
		if p.Get(candidate) == nil {
			return candidate
		}
	}
}
