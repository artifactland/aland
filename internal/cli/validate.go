package cli

import (
	"fmt"

	"github.com/artifactland/aland/internal/bundle"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [directory]",
		Short: "Lint a bundle directory before pushing",
		Long: `Walks the directory and checks the bundle's shape against the rules
artifact.land enforces on upload — structure (entry at the root,
everything else under assets/), file-type allowlist (raster images
only for v1), size and file-count caps (Free 5MB / 50 files, Pro
25MB / 200 files), reference integrity (entry HTML pointing at
files that don't exist), and image weight.

Errors block publish. Warnings print but allow ` + "`aland push`" + ` to proceed.

` + "`aland push`" + ` runs validate automatically as its first step, so
you usually don't have to call this directly — but it's the right
tool for CI ("does my bundle still validate?") or for poking at a
broken push without the network round-trip.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runValidate,
	}
	return cmd
}

func runValidate(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	report, err := bundle.Validate(dir)
	if err != nil {
		return err
	}

	printReport(cmd, dir, report)

	if report.HasErrors() {
		return ui.Errorf("Bundle has %d blocking issue(s).", len(report.Errors()))
	}
	return nil
}

// printReport renders a validate Report to the user's terminal. Summary
// up top, then issues grouped by level. Stderr for the styled output;
// the bare report shape stays clean for piping if anyone wants it.
func printReport(cmd *cobra.Command, dir string, r *bundle.Report) {
	err := cmd.ErrOrStderr()

	fmt.Fprintln(err, "")
	fmt.Fprintln(err, ui.MutedStyle.Render(fmt.Sprintf("  Bundle: %s", dir)))
	fmt.Fprintln(err, ui.MutedStyle.Render(fmt.Sprintf("  %d files, %s",
		r.FileCount, bundle.HumanizeBytes(r.TotalBytes))))
	if r.EntryPath != "" {
		fmt.Fprintln(err, ui.MutedStyle.Render(fmt.Sprintf("  Entry:  %s", r.EntryPath)))
	}
	fmt.Fprintln(err, "")

	for _, issue := range r.Errors() {
		printIssue(err, "error", issue)
	}
	for _, issue := range r.Warnings() {
		printIssue(err, "warn", issue)
	}

	if !r.HasErrors() && len(r.Warnings()) == 0 {
		ui.Success("Bundle looks good.")
	} else if !r.HasErrors() {
		ui.Info("Bundle passes with %d warning(s).", len(r.Warnings()))
	}
}

func printIssue(out interface{ Write(p []byte) (int, error) }, label string, issue bundle.Issue) {
	prefix := "  ✗ "
	if label == "warn" {
		prefix = "  ⚠ "
	}
	if issue.Path != "" {
		fmt.Fprintf(out, "%s%s: %s\n    %s\n", prefix, issue.Code, issue.Path, issue.Message)
	} else {
		fmt.Fprintf(out, "%s%s\n    %s\n", prefix, issue.Code, issue.Message)
	}
}
