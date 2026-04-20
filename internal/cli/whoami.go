package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/artifactland/aland/internal/api"
	"github.com/artifactland/aland/internal/config"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newWhoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the currently signed-in user",
		Long: `Print the username for the active profile. Hits /api/v1/users/me to confirm
the token is still valid; falls back to the cached username from the
credentials file when the server is unreachable.

Exit code 1 if not signed in, so scripts can branch on it cleanly.`,
		RunE: runWhoami,
	}
}

func runWhoami(cmd *cobra.Command, _ []string) error {
	globals := Globals(cmd.Context())
	profileName := globals.Profile
	if profileName == "" {
		profileName = config.DefaultProfile
	}

	creds, err := config.Load()
	if err != nil {
		return err
	}
	profile := creds.GetProfile(profileName)
	if profile == nil {
		return fmt.Errorf("not signed in on profile %q (run `aland login` first)", profileName)
	}

	apiBase, err := effectiveAPIBase(profile.APIBase, globals.APIBase)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	client := &api.Client{APIBase: apiBase, Token: profile.AccessToken}
	me, err := client.Me(ctx)
	if err != nil {
		// Cached username is better than nothing — useful when running
		// offline and just confirming "who am I logged in as locally."
		if profile.Username != "" {
			ui.Warn("Couldn't reach %s: %v", apiBase, err)
			renderWhoami(cmd, globals.JSON, profile, nil)
			return nil
		}
		return err
	}

	renderWhoami(cmd, globals.JSON, profile, me)
	return nil
}

func renderWhoami(cmd *cobra.Command, asJSON bool, profile *config.Profile, fresh *api.User) {
	username := profile.Username
	displayName := ""
	email := ""
	if fresh != nil {
		username = fresh.Username
		displayName = fresh.DisplayName
		email = fresh.Email
	}

	if asJSON {
		out := map[string]any{
			"username":     username,
			"display_name": displayName,
			"email":        email,
			"profile":      Globals(cmd.Context()).Profile,
			"api_base":     profile.APIBase,
		}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		return
	}

	fmt.Fprintln(cmd.OutOrStdout(), "@"+username)
	if displayName != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  "+displayName))
	}
	if email != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  "+email))
	}
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  "+profile.APIBase))
}
