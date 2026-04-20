package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newPullCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "pull <@user/slug> [dir]",
		Short: "Pull an existing artifact's source into a local directory",
		Long: `Downloads the source of one of your artifacts — draft or published — and
writes .aland.json so ` + "`" + `aland push` + "`" + ` will patch this specific post on the
next run. Useful for fix-in-place loops: pull your published artifact,
patch the bug, republish.

The post must be one you own or one that's publicly readable. Publishing
other people's work requires ` + "`" + `aland fork` + "`" + ` instead — that's the API boundary,
not a CLI behavior.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, args, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite dir if it already contains files")
	return cmd
}

func runPull(cmd *cobra.Command, args []string, force bool) error {
	ref := args[0]
	username, slug, err := parseRef(ref)
	if err != nil {
		return err
	}

	dir := slug
	if len(args) == 2 {
		dir = args[1]
	}
	absDir, _ := filepath.Abs(dir)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	if !force {
		if project.Exists(dir) {
			return fmt.Errorf("%s already contains a .aland.json (use --force to replace)", absDir)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) > 0 {
			return fmt.Errorf("%s isn't empty (use --force to proceed anyway)", absDir)
		}
	}

	globals := Globals(cmd.Context())
	client, profile, err := authedClient(globals)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	ui.Info("Fetching @%s/%s from %s...", username, slug, profile.APIBase)
	post, err := client.GetPostByRef(ctx, username, slug)
	if err != nil {
		return fmt.Errorf("fetching post: %w", err)
	}

	src, err := client.GetSource(ctx, post.ID)
	if err != nil {
		return fmt.Errorf("downloading source: %w", err)
	}

	sourceFile := "index." + extensionFor(src.FileType)
	sourcePath := filepath.Join(dir, sourceFile)
	if err := os.WriteFile(sourcePath, []byte(src.Source), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", sourcePath, err)
	}

	p := project.New(dir)
	if err := p.Add(post.Slug, &project.Artifact{
		PostID:     post.ID,
		SourceFile: sourceFile,
		Title:      post.Title,
		Tags:       post.Tags,
		Visibility: post.Visibility,
	}); err != nil {
		return err
	}
	if err := p.Save(); err != nil {
		return err
	}

	isLive := post.PublishedAt != nil && *post.PublishedAt != ""
	state := "draft"
	if isLive {
		state = "published"
	}

	ui.Success("Pulled @%s/%s (%s) into %s", post.User.Username, post.Slug, state, absDir)
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render(fmt.Sprintf("  source: %s", sourcePath)))
	if post.URLs.Web != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  web:    "+post.URLs.Web))
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "")
	if isLive {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  Edit the file and run `aland push` to update the live artifact in place."))
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  Edit the file and run `aland push` to update the draft."))
	}
	return nil
}
