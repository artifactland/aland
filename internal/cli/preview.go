package cli

import (
	"fmt"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/artifactland/aland/internal/preview"
	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newPreviewCommand() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "preview [name-or-path]",
		Short: "Serve the artifact locally with a prod-identical sandbox",
		Long: `Starts a small HTTP server on localhost that serves the current project's
source file behind the same CSP, sandbox, and permissions policy as the
production content worker. If it works here, it'll work after publishing.

In a multi-artifact project, pass the artifact's name or source file path
to pick which one to preview. Bare ` + "`" + `aland preview` + "`" + ` works when there's
only one artifact.

HTML-only for v1. For JSX, run ` + "`" + `aland push` + "`" + ` and open /drafts/:id/preview
on artifact.land — the same prod sandbox, but with the server-side JSX
transpile already in the loop.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreview(cmd, args, port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 4321, "port to bind (falls back to random if taken)")
	return cmd
}

func runPreview(cmd *cobra.Command, args []string, port int) error {
	p, err := project.Load(".")
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}
	var query string
	if len(args) == 1 {
		query = args[0]
	}
	a, err := p.Resolve(query)
	if err != nil {
		return err
	}
	sourcePath := a.AbsSource(p.Dir())

	// JSX preview isn't wired locally yet — be honest about it rather than
	// serve something that drifts from production. Draft preview on the
	// server is the right route for JSX in v1.
	if strings.EqualFold(filepath.Ext(sourcePath), ".jsx") {
		return ui.Errorf(
			"JSX preview isn't supported locally yet. Run `aland push` and open the draft in your browser:\n  aland open --preview",
		)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv, err := preview.Start(ctx, sourcePath, port)
	if err != nil {
		return err
	}

	ui.Success("Preview running at %s", srv.URL())
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  serving "+sourcePath))
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  same CSP + sandbox as production"))
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  press Ctrl-C to stop"))

	<-ctx.Done()
	ui.Info("Preview stopped.")
	return nil
}
