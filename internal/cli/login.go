package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/artifactland/aland/internal/api"
	"github.com/artifactland/aland/internal/config"
	"github.com/artifactland/aland/internal/oauth"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

// authorizeScope is the full set of scopes the CLI ever needs. publish:live
// deliberately is not in here (and doesn't exist server-side).
const authorizeScope = "read publish:draft"

// loginTimeout gives the user time to sign in, authorize, and close the tab
// without making them redo the flow for taking a moment to think about it.
const loginTimeout = 5 * time.Minute

func newLoginCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to artifact.land",
		Long: `Open the artifact.land authorize page in your browser, hand back a token,
and remember it for subsequent commands. Uses loopback OAuth with PKCE.

Already signed in? ` + "`" + `aland whoami` + "`" + ` shows who; pass --force to re-auth.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "re-authenticate even if a profile already has a token")
	return cmd
}

func runLogin(cmd *cobra.Command, force bool) error {
	globals := Globals(cmd.Context())
	profile := globals.Profile
	if profile == "" {
		profile = config.DefaultProfile
	}

	apiBase, err := resolveAPIBase(globals.APIBase)
	if err != nil {
		return err
	}

	// Refuse to overwrite an existing profile unless the user asked for it.
	if existing := loadProfile(profile); existing != nil && !force {
		return fmt.Errorf(
			"already signed in as @%s on profile %q. Re-run with --force to replace the token, or `aland logout --profile %s` first",
			firstNonEmpty(existing.Username, "<unknown>"), profile, profile,
		)
	}

	pkce, err := oauth.NewPKCE()
	if err != nil {
		return err
	}
	state, err := oauth.RandomState()
	if err != nil {
		return err
	}

	server, err := oauth.StartLoopback(state)
	if err != nil {
		return err
	}

	client := &oauth.Client{
		APIBase:  apiBase,
		ClientID: config.DefaultClientID,
	}
	authorizeURL := client.AuthorizeURL(server.RedirectURI(), authorizeScope, state, pkce.Challenge)

	ui.Info("Opening %s", apiBase+"/oauth/authorize")
	if err := oauth.OpenURL(authorizeURL); err != nil {
		// Non-fatal — the user can still click the URL we print.
		ui.Warn("Couldn't open the browser automatically: %v", err)
		ui.Info("Open this URL manually:\n\n  %s\n", authorizeURL)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), loginTimeout)
	defer cancel()

	ui.Info("Waiting for you to authorize in the browser...")
	code, err := server.Await(ctx)
	if err != nil {
		return fmt.Errorf("sign-in interrupted: %w", err)
	}

	token, err := client.Exchange(ctx, code, server.RedirectURI(), pkce.Verifier)
	if err != nil {
		return fmt.Errorf("exchanging authorization code: %w", err)
	}

	// Stash the token immediately so even if the follow-up whoami call fails
	// (network blip, rate limit, etc.) the user doesn't have to redo login.
	p := &config.Profile{
		APIBase:      apiBase,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC(),
	}
	if err := config.SetProfile(profile, p); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	// Enrich the stored profile with identity so whoami is instant.
	if me, err := (&api.Client{APIBase: apiBase, Token: token.AccessToken}).Me(ctx); err == nil {
		p.UserID = me.ID
		p.Username = me.Username
		_ = config.SetProfile(profile, p)
		ui.Success("Signed in as @%s", me.Username)
	} else {
		// Still a success — the token is good; we just didn't get the name.
		ui.Success("Signed in.")
		ui.Info("Couldn't fetch profile: %v", err)
	}

	path, _ := config.CredentialsPath()
	fmt.Fprintln(os.Stderr, ui.MutedStyle.Render(fmt.Sprintf("  credentials stored at %s (chmod 600)", path)))

	return nil
}

func loadProfile(name string) *config.Profile {
	creds, err := config.Load()
	if err != nil {
		return nil
	}
	return creds.GetProfile(name)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
