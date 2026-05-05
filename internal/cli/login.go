package cli

import (
	"context"
	"errors"
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

If you're already signed in, ` + "`" + `aland login` + "`" + ` is a no-op when the token is
fresh, silently refreshes when it's expired but the refresh token still
works, and falls through to a full browser sign-in when the refresh token
is dead. Pass --force to skip those shortcuts and always re-auth.`,
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

	// If a profile already exists, try the cheapest path that gets the user
	// to a working session: no-op when fresh, refresh when expired-but-
	// refreshable, full re-auth when the refresh token is also dead. --force
	// always skips this and runs the full PKCE flow.
	if existing := loadProfile(profile); existing != nil && !force {
		// Refresh against the profile's recorded API base (overridden by
		// --api-url), so a `--profile staging` session refreshes on staging
		// rather than production.
		refreshBase, err := effectiveAPIBase(existing.APIBase, globals.APIBase)
		if err != nil {
			return err
		}
		rerr := reuseOrRefresh(cmd.Context(), refreshBase, profile, existing)
		if rerr == nil {
			return nil // success, no PKCE needed
		}
		if !errors.Is(rerr, errReauthNeeded) {
			return rerr
		}
		// errReauthNeeded → fall through to the full browser flow without
		// requiring `aland logout` first.
		ui.Info("Refreshing your session needs a new browser sign-in.")
	}

	apiBase, err := resolveAPIBase(globals.APIBase)
	if err != nil {
		return err
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

// errReauthNeeded signals that the existing profile can't be salvaged and
// the caller should fall through to the full PKCE browser flow. Any other
// error returned by reuseOrRefresh is a hard failure (e.g. couldn't write
// to disk) — caller should bail.
var errReauthNeeded = errors.New("re-auth needed")

// reuseOrRefresh tries to satisfy `aland login` without opening the browser.
// Returns nil on success (token was either still good or got refreshed) and
// errReauthNeeded when the only path forward is a fresh PKCE sign-in.
func reuseOrRefresh(ctx context.Context, apiBase, profileName string, existing *config.Profile) error {
	if !tokenNeedsRefresh(existing) {
		ui.Success("Already signed in as @%s.", firstNonEmpty(existing.Username, "<unknown>"))
		ui.Info("Use --force to start a fresh sign-in.")
		return nil
	}

	if existing.RefreshToken == "" {
		return errReauthNeeded
	}

	refreshed, rerr := refreshProfile(apiBase, existing)
	if rerr != nil {
		// Hard OAuth errors (revoked / expired refresh token) → re-auth.
		// Soft errors (network) → also fall through to browser flow; the
		// user is asking us to fix things, not paper over a transient blip.
		return errReauthNeeded
	}

	if err := config.SetProfile(profileName, refreshed); err != nil {
		return fmt.Errorf("saving refreshed credentials: %w", err)
	}
	ui.Success("Refreshed session for @%s.", firstNonEmpty(refreshed.Username, "<unknown>"))
	_ = ctx // reserved for a future `Me()` re-check; not needed for now
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
