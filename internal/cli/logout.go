package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/artifactland/aland/internal/config"
	"github.com/artifactland/aland/internal/oauth"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and revoke your access token",
		Long: `Revoke the current profile's tokens at the server and remove the local
credentials. Subsequent commands will prompt you to ` + "`" + `aland login` + "`" + ` again.

Server revocation is best-effort — if the server is unreachable, we still
delete the local credentials so the token can't be used from this machine.`,
		RunE: runLogout,
	}
}

func runLogout(cmd *cobra.Command, _ []string) error {
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
		return fmt.Errorf("not signed in on profile %q", profileName)
	}

	apiBase, err := effectiveAPIBase(profile.APIBase, globals.APIBase)
	if err != nil {
		return err
	}

	client := &oauth.Client{APIBase: apiBase, ClientID: config.DefaultClientID}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	// Revoke the access token (best-effort) and the refresh token (if we
	// have one). Either way we delete the local creds at the end.
	if err := client.Revoke(ctx, profile.AccessToken); err != nil {
		ui.Warn("Couldn't reach the server to revoke the access token: %v", err)
	}
	if profile.RefreshToken != "" {
		_ = client.Revoke(ctx, profile.RefreshToken)
	}

	delete(creds.Profiles, profileName)
	if err := config.Save(creds); err != nil {
		return fmt.Errorf("removing local credentials: %w", err)
	}

	if profile.Username != "" {
		ui.Success("Signed out @%s (profile %q).", profile.Username, profileName)
	} else {
		ui.Success("Signed out (profile %q).", profileName)
	}
	return nil
}
