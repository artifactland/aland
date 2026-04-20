package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artifactland/aland/internal/api"
	"github.com/artifactland/aland/internal/config"
	"github.com/artifactland/aland/internal/oauth"
	"github.com/artifactland/aland/internal/project"
	"github.com/artifactland/aland/internal/ui"
	"github.com/spf13/cobra"
)

func newForkCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "fork <@user/slug> [dir]",
		Short: "Fork an artifact into a local directory",
		Long: `Creates a draft on artifact.land with fork_of set to the source, downloads
its source into dir, and writes a .aland.json. After forking, iterate in
your agent and run ` + "`" + `aland push` + "`" + ` to update the draft.

The source must be visible to you — public or unlisted works for anyone;
private (future) requires an invitation.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFork(cmd, args, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite dir if it already contains files")
	return cmd
}

func runFork(cmd *cobra.Command, args []string, force bool) error {
	ref := args[0]
	username, slug, err := parseRef(ref)
	if err != nil {
		return err
	}

	dir := slug
	if len(args) == 2 {
		dir = args[1]
	}
	absDir, _ := filepath.Abs(dir)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	// Refuse to trample an existing project unless explicitly told to.
	if !force {
		if project.Exists(dir) {
			return fmt.Errorf("%s already contains a .aland.json (use --force to replace)", absDir)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) > 0 {
			return fmt.Errorf("%s isn't empty (use --force to proceed anyway)", absDir)
		}
	}

	globals := Globals(cmd.Context())
	client, profile, err := authedClient(globals)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	// Resolve the source post first so we fail fast on a bad ref before
	// calling the fork endpoint (which would also 404 but less clearly).
	ui.Info("Looking up @%s/%s on %s...", username, slug, profile.APIBase)
	source, err := client.GetPostByRef(ctx, username, slug)
	if err != nil {
		return fmt.Errorf("fetching source: %w", err)
	}

	ui.Info("Creating a fork draft server-side...")
	draft, err := client.ForkPost(ctx, source.ID, nil)
	if err != nil {
		return fmt.Errorf("forking: %w", err)
	}

	// Pull down the draft's source so the caller has a file to edit. The
	// server already copied it off the original, but we fetch from the
	// draft's URL so future refetches reflect what the server has for this
	// specific draft (in case a later preprocessor kicks in).
	src, err := client.GetSource(ctx, draft.ID)
	if err != nil {
		return fmt.Errorf("downloading source: %w", err)
	}

	sourceFile := "index." + extensionFor(src.FileType)
	sourcePath := filepath.Join(dir, sourceFile)
	if err := os.WriteFile(sourcePath, []byte(src.Source), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", sourcePath, err)
	}

	p := project.New(dir)
	if err := p.Add(draft.Slug, &project.Artifact{
		PostID:     draft.ID,
		SourceFile: sourceFile,
		Title:      draft.Title,
		Tags:       draft.Tags,
		Visibility: draft.Visibility,
		ForkOf: &project.ForkOf{
			PostID:   source.ID,
			Username: source.User.Username,
			Slug:     source.Slug,
			Title:    source.Title,
		},
	}); err != nil {
		return err
	}
	if err := p.Save(); err != nil {
		return err
	}

	ui.Success("Forked @%s/%s into %s", source.User.Username, source.Slug, absDir)
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render(fmt.Sprintf("  source: %s", sourcePath)))
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render(fmt.Sprintf("  draft:  %s", draft.URLs.Web)))
	fmt.Fprintln(cmd.ErrOrStderr(), "")
	fmt.Fprintln(cmd.ErrOrStderr(), ui.MutedStyle.Render("  Next: edit the file, then run `aland push` to update the draft."))
	return nil
}

// parseRef accepts "@alice/thing" or "alice/thing" — both refer to the same
// artifact. Returns (username, slug) after stripping the @.
func parseRef(ref string) (string, string, error) {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "@")
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("ref %q isn't in @user/slug form", ref)
	}
	return parts[0], parts[1], nil
}

func extensionFor(fileType string) string {
	switch fileType {
	case "jsx":
		return "jsx"
	default:
		return "html"
	}
}

// refreshLeeway is how close to expiry we treat a token as "needs refresh."
// Small enough that we don't refresh eagerly on every command, large enough
// that we don't race against the server clock.
const refreshLeeway = 60 * time.Second

// refreshTimeout bounds the blocking /oauth/token call from inside authedClient
// so a flaky auth server can't hang the CLI.
const refreshTimeout = 15 * time.Second

// authedClient returns an api.Client configured for the active profile,
// plus that profile so callers can read metadata from it.
//
// If the stored access token is expired (or within refreshLeeway of it) and
// a refresh token is present, authedClient silently exchanges the refresh
// token for a new pair before returning. The rotated tokens are persisted
// so subsequent commands don't each re-refresh. Refresh failures are
// swallowed intentionally — the stale access token is returned and the
// server will 401 with a clear message if it really is dead.
func authedClient(globals *GlobalFlags) (*api.Client, *config.Profile, error) {
	profileName := globals.Profile
	if profileName == "" {
		profileName = config.DefaultProfile
	}
	creds, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	profile := creds.GetProfile(profileName)
	if profile == nil {
		return nil, nil, fmt.Errorf("not signed in on profile %q (run `aland login` first)", profileName)
	}
	apiBase, err := effectiveAPIBase(profile.APIBase, globals.APIBase)
	if err != nil {
		return nil, nil, err
	}

	if profile.RefreshToken != "" && tokenNeedsRefresh(profile) {
		if refreshed, rerr := refreshProfile(apiBase, profile); rerr == nil {
			if serr := config.SetProfile(profileName, refreshed); serr == nil {
				profile = refreshed
			} else {
				// Couldn't persist — still use the fresh in-memory tokens
				// for this command. Next command will try to refresh again.
				profile = refreshed
			}
		}
	}

	return &api.Client{APIBase: apiBase, Token: profile.AccessToken}, profile, nil
}

// tokenNeedsRefresh returns true when the access token is past expiry or
// within refreshLeeway of expiring. Profiles that predate ExpiresAt (zero
// value) are treated as needing refresh — cheaper than a failed command.
func tokenNeedsRefresh(p *config.Profile) bool {
	if p.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(refreshLeeway).After(p.ExpiresAt)
}

// refreshProfile exchanges the profile's refresh_token for a fresh access
// token against apiBase. Returns a new Profile carrying the rotated pair
// plus the identity fields copied from the original.
func refreshProfile(apiBase string, p *config.Profile) (*config.Profile, error) {
	client := &oauth.Client{
		APIBase:  apiBase,
		ClientID: config.DefaultClientID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	tok, err := client.Refresh(ctx, p.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Doorkeeper rotates refresh_token on success, but defend against a
	// provider that omits it so we never drop our only copy.
	newRefresh := tok.RefreshToken
	if newRefresh == "" {
		newRefresh = p.RefreshToken
	}

	return &config.Profile{
		APIBase:      p.APIBase,
		AccessToken:  tok.AccessToken,
		RefreshToken: newRefresh,
		ExpiresAt:    time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC(),
		UserID:       p.UserID,
		Username:     p.Username,
	}, nil
}
