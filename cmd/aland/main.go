package main

import (
	"fmt"
	"os"

	"github.com/artifactland/aland/internal/cli"
	"github.com/artifactland/aland/internal/ui"
)

// Version is overridden at build time by goreleaser's -ldflags.
var Version = "dev"

func main() {
	ui.Init()

	root := cli.NewRoot(Version)
	if err := root.Execute(); err != nil {
		// Root was configured with SilenceErrors, so we print the failure
		// ourselves — goes to stderr with the error styling.
		fmt.Fprintln(os.Stderr, ui.ErrorStyle.Render("✗ "+err.Error()))
		os.Exit(1)
	}
}
