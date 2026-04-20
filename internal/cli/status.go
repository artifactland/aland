package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/artifactland/aland/internal/api"
	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local project state and server-side post status",
		Long: `Prints what .aland.json knows (source file, title, fork source, post id)
and — when there's a post_id — fetches the server's current state so you
can see whether the post is a draft or published, the URL to open, etc.

Agent-friendly: pass --json to get a structured report.`,
		RunE: runStatus,
	}
}

type statusReport struct {
	Dir        string            `json:"dir"`
	SourceFile string            `json:"source_file"`
	Title      string            `json:"title,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	Visibility string            `json:"visibility,omitempty"`
	ForkOf     *project.ForkOf   `json:"fork_of,omitempty"`
	PostID     string            `json:"post_id,omitempty"`
	Server     *statusServerData `json:"server,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type statusServerData struct {
	ID          string  `json:"id"`
	State       string  `json:"state"`
	Visibility  string  `json:"visibility"`
	WebURL      string  `json:"web_url,omitempty"`
	PublishedAt *string `json:"published_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

func runStatus(cmd *cobra.Command, _ []string) error {
	absDir, _ := filepath.Abs(".")

	p, err := project.Load(".")
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}
	a, err := p.Only()
	if err != nil {
		return err
	}

	report := statusReport{
		Dir:        absDir,
		SourceFile: a.SourceFile,
		Title:      a.Title,
		Tags:       a.Tags,
		Visibility: a.Visibility,
		ForkOf:     a.ForkOf,
		PostID:     a.PostID,
	}

	if a.PostID != "" {
		globals := Globals(cmd.Context())
		client, _, err := authedClient(globals)
		if err == nil {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			if post, perr := client.GetPost(ctx, a.PostID); perr == nil {
				report.Server = &statusServerData{
					ID:          post.ID,
					State:       draftState(post),
					Visibility:  post.Visibility,
					WebURL:      post.URLs.Web,
					PublishedAt: post.PublishedAt,
					UpdatedAt:   post.UpdatedAt,
				}
			} else {
				report.Error = perr.Error()
			}
		}
	}

	if Globals(cmd.Context()).JSON {
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(report)
		return nil
	}

	renderStatus(cmd, &report)
	return nil
}

func draftState(p *api.Post) string {
	if p.PublishedAt != nil && *p.PublishedAt != "" {
		return "published"
	}
	return "draft"
}

func renderStatus(cmd *cobra.Command, r *statusReport) {
	w := cmd.ErrOrStderr()
	fmt.Fprintln(w, ui.BoldStyle.Render(r.Dir))
	fmt.Fprintln(w, ui.MutedStyle.Render("  source: "+r.SourceFile))
	if r.Title != "" {
		fmt.Fprintln(w, ui.MutedStyle.Render("  title:  "+r.Title))
	}
	if len(r.Tags) > 0 {
		fmt.Fprintln(w, ui.MutedStyle.Render(fmt.Sprintf("  tags:   %v", r.Tags)))
	}
	if r.Visibility != "" {
		fmt.Fprintln(w, ui.MutedStyle.Render("  visibility: "+r.Visibility))
	}
	if r.ForkOf != nil {
		fmt.Fprintln(w, ui.MutedStyle.Render(fmt.Sprintf("  forked from: @%s/%s", r.ForkOf.Username, r.ForkOf.Slug)))
	}
	fmt.Fprintln(w, "")

	if r.PostID == "" {
		fmt.Fprintln(w, ui.MutedStyle.Render("  Nothing on the server yet — run `aland push` to create a draft."))
		return
	}

	if r.Error != "" {
		fmt.Fprintln(w, ui.WarnStyle.Render("  couldn't reach the server: "+r.Error))
		fmt.Fprintln(w, ui.MutedStyle.Render("  local post id: "+r.PostID))
		return
	}

	if r.Server != nil {
		fmt.Fprintln(w, ui.AccentStyle.Render("  "+r.Server.State+" ("+r.Server.Visibility+")"))
		if r.Server.WebURL != "" {
			fmt.Fprintln(w, "  "+r.Server.WebURL)
		}
	}
}
