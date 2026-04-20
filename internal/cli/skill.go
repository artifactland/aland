package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

//go:embed embedded/SKILL.md
var skillMarkdown string

func newSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage the Claude Code skill for artifactland projects",
	}

	cmd.AddCommand(newSkillInstallCommand())
	return cmd
}

func newSkillInstallCommand() *cobra.Command {
	var print bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the artifactland skill into ~/.claude/skills/",
		Long: `Copies the bundled SKILL.md into ~/.claude/skills/artifactland/ so Claude
Code picks up the workflow + sandbox constraints without the user pasting
anything. Safe to run repeatedly — overwrites in place.

With --print, prints the destination path and skill content without
writing anything. Useful for agents that want to read the skill via stdout.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSkillInstall(cmd, print)
		},
	}

	cmd.Flags().BoolVar(&print, "print", false, "print the destination path and content without writing")
	return cmd
}

func runSkillInstall(cmd *cobra.Command, printOnly bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locating home dir: %w", err)
	}
	dir := filepath.Join(home, ".claude", "skills", "artifactland")
	path := filepath.Join(dir, "SKILL.md")

	if printOnly {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("would write to "+path))
		fmt.Fprint(cmd.OutOrStdout(), skillMarkdown)
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(skillMarkdown), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	ui.Success("Installed artifactland skill at %s", path)
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  restart Claude Code to pick it up"))
	return nil
}
