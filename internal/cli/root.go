// Package cli assembles the cobra command tree for the aland binary.
// Keeping commands here (rather than in cmd/aland) lets tests instantiate
// NewRoot() without dragging in the main-function side effects.
package cli

import (
	"github.com/artifactland/aland/internal/config"
	"github.com/spf13/cobra"
)

// GlobalFlags holds values set by top-level persistent flags. Populated on
// PersistentPreRun and read by subcommands.
type GlobalFlags struct {
	Profile string
	APIBase string
	JSON    bool
}

// NewRoot returns a configured root cobra command. Version is passed in so
// the binary baked by goreleaser can set it at link time without changing
// this file.
func NewRoot(version string) *cobra.Command {
	globals := &GlobalFlags{}

	cmd := &cobra.Command{
		Use:   "aland",
		Short: "Publish and fork artifacts from the terminal",
		Long: `aland is the command-line companion for artifact.land.

Fork an artifact from someone's profile, edit it in your agent of choice,
preview the result in a production-identical sandbox, and publish it as a
draft. Publishing live always happens in your browser — that's a deliberate
safety property, not a limitation.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&globals.Profile, "profile", config.DefaultProfile,
		"credential profile to use (lets one install talk to prod and staging)")
	cmd.PersistentFlags().StringVar(&globals.APIBase, "api-url", "",
		"override the API base URL (defaults to https://artifact.land or $ALAND_API)")
	cmd.PersistentFlags().BoolVar(&globals.JSON, "json", false,
		"emit machine-readable JSON instead of pretty output")

	cmd.AddCommand(newVersionCommand(version))
	cmd.AddCommand(newLoginCommand())
	cmd.AddCommand(newLogoutCommand())
	cmd.AddCommand(newWhoamiCommand())
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newForkCommand())
	cmd.AddCommand(newPullCommand())
	cmd.AddCommand(newPreviewCommand())
	cmd.AddCommand(newPushCommand())
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newOpenCommand())
	cmd.AddCommand(newLinkCommand())
	cmd.AddCommand(newContextCommand())
	cmd.AddCommand(newSkillCommand())

	// Wire globals into context so subcommand Run funcs can reach them
	// without depending on package-level state.
	cmd.PersistentPreRun = func(c *cobra.Command, _ []string) {
		c.SetContext(WithGlobals(c.Context(), globals))
	}

	return cmd
}
