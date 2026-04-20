package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/artifactland/aland/internal/oauth"
	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newOpenCommand() *cobra.Command {
	var preview bool

	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open this project's draft or published URL in your browser",
		Long: `Reads .aland.json, fetches the draft's current web URL from the API, and
opens it. Falls back to printing the URL when no browser is available.

With --preview, opens /drafts/:id/preview instead (the full-screen
sandbox view) — useful for JSX where local preview isn't supported yet.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOpen(cmd, preview)
		},
	}

	cmd.Flags().BoolVar(&preview, "preview", false, "open the full-screen preview instead of the edit page")
	return cmd
}

func runOpen(cmd *cobra.Command, preview bool) error {
	p, err := project.Load(".")
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}
	a, err := p.Only()
	if err != nil {
		return err
	}
	if a.PostID == "" {
		return fmt.Errorf("no draft yet — run `aland push` first")
	}

	globals := Globals(cmd.Context())
	client, profile, err := authedClient(globals)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()

	post, err := client.GetPost(ctx, a.PostID)
	if err != nil {
		return fmt.Errorf("fetching draft: %w", err)
	}

	url := post.URLs.Web
	if preview {
		// The preview URL isn't returned explicitly; it's a known path.
		url = fmt.Sprintf("%s/drafts/%s/preview", profile.APIBase, post.ID)
	}
	if url == "" {
		return fmt.Errorf("no URL available for this draft yet")
	}

	if err := oauth.OpenURL(url); err != nil {
		ui.Warn("Couldn't open the browser automatically: %v", err)
		fmt.Fprintln(cmd.OutOrStdout(), url)
		return nil
	}
	ui.Success("Opened %s", url)
	fmt.Fprintln(cmd.OutOrStdout(), url)
	return nil
}
