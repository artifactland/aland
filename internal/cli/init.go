package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

//go:embed templates/starter.html
var starterHTML string

//go:embed templates/starter.jsx
var starterJSX string

func newInitCommand() *cobra.Command {
	var jsx bool
	var force bool

	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a new artifact project in dir (or current directory)",
		Long: `Creates a fresh .aland.json alongside a starter index.html (or index.jsx
with --jsx) so you can start editing with your agent of choice and eventually
` + "`" + `aland push` + "`" + ` it as a draft.

Refuses to overwrite an existing project — pass --force to replace.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, args, jsx, force)
		},
	}

	cmd.Flags().BoolVar(&jsx, "jsx", false, "scaffold a JSX starter instead of HTML")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing .aland.json")
	return cmd
}

func runInit(cmd *cobra.Command, args []string, jsx, force bool) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	if project.Exists(dir) && !force {
		return fmt.Errorf("%s already contains a .aland.json (use --force to replace)", dir)
	}

	sourceFile := "index.html"
	content := starterHTML
	if jsx {
		sourceFile = "index.jsx"
		content = starterJSX
	}

	sourcePath := filepath.Join(dir, sourceFile)
	if _, err := os.Stat(sourcePath); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", sourcePath)
	}
	if err := os.WriteFile(sourcePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing starter: %w", err)
	}

	p := project.New(dir)
	if err := p.Add(project.DefaultArtifactName, &project.Artifact{
		SourceFile: sourceFile,
		Visibility: "public_visibility",
	}); err != nil {
		return err
	}
	if err := p.Save(); err != nil {
		return err
	}

	abs, _ := filepath.Abs(dir)
	ui.Success("Scaffolded artifact project at %s", abs)
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render(fmt.Sprintf("  %s", filepath.Join(dir, sourceFile))))
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render(fmt.Sprintf("  %s", filepath.Join(dir, project.Filename))))
	fmt.Fprintln(cmd.ErrOrStderr(), "")
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  Next: edit the file, then run `aland push` to create a draft."))
	return nil
}
