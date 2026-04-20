package cli

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed embedded/context.md
var contextMarkdown string

func newContextCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "context",
		Short: "Print everything an agent needs to know about artifact.land",
		Long: `Dumps a self-contained markdown briefing to stdout — workflow,
sandbox constraints, supported libraries, error codes, and exit codes.

Designed for agents that can exec shell commands: one ` + "`" + `aland context` + "`" + ` call
and the agent is oriented. Pipe it anywhere or redirect to a file.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), contextMarkdown)
			return err
		},
	}
}
