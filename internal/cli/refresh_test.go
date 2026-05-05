package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/artifactland/aland/internal/api"
	"github.com/artifactland/aland/internal/config"
)

// TestAuthedClientRefreshesExpiredToken confirms that authedClient silently
// swaps a past-expiry access token for a fresh one via the refresh_token
// grant, and persists the rotated pair so subsequent commands don't each
// re-refresh.
func TestAuthedClientRefreshesExpiredToken(t *testing.T) {
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		tokenCalls++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("refresh_token"); got != "old-refresh" {
			t.Errorf("refresh_token = %q, want old-refresh", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "new-access",
			"refresh_token": "new-refresh",
			"token_type": "Bearer",
			"expires_in": 3600
		}`))
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:      srv.URL,
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired
		Username:     "scott",
	}); err != nil {
		t.Fatal(err)
	}

	client, profile, err := authedClient(&GlobalFlags{})
	if err != nil {
		t.Fatalf("authedClient: %v", err)
	}
	if client.Token != "new-access" {
		t.Errorf("client.Token = %q, want new-access", client.Token)
	}
	if profile.RefreshToken != "new-refresh" {
		t.Errorf("profile.RefreshToken = %q, want new-refresh", profile.RefreshToken)
	}
	if profile.Username != "scott" {
		t.Errorf("identity fields should survive refresh; username = %q", profile.Username)
	}
	if tokenCalls != 1 {
		t.Errorf("token endpoint hit %d times on first call, want 1", tokenCalls)
	}

	// Second call should reuse the now-fresh token and NOT hit /oauth/token.
	if _, _, err := authedClient(&GlobalFlags{}); err != nil {
		t.Fatalf("second authedClient: %v", err)
	}
	if tokenCalls != 1 {
		t.Errorf("token endpoint hit %d times after refresh; want 1 (no re-refresh)", tokenCalls)
	}

	// And the rotated tokens should be persisted to disk.
	creds, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	reloaded := creds.GetProfile(config.DefaultProfile)
	if reloaded == nil {
		t.Fatal("profile vanished after refresh")
	}
	if reloaded.AccessToken != "new-access" || reloaded.RefreshToken != "new-refresh" {
		t.Errorf("persisted tokens not rotated: %+v", reloaded)
	}
}

// TestAuthedClientReturnsErrUnauthenticatedOnDeadRefresh ensures a hard
// OAuth error from the token endpoint (invalid_grant — refresh token is
// revoked or expired) short-circuits authedClient with the typed
// ErrUnauthenticated sentinel rather than letting a stale access token
// proceed to the API and trigger a confusing downstream 401.
func TestAuthedClientReturnsErrUnauthenticatedOnDeadRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"revoked"}`))
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:      srv.URL,
		AccessToken:  "stale-access",
		RefreshToken: "revoked-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	_, _, err := authedClient(&GlobalFlags{})
	if err == nil {
		t.Fatal("expected ErrUnauthenticated, got nil")
	}
	if !errors.Is(err, api.ErrUnauthenticated) {
		t.Errorf("expected api.ErrUnauthenticated, got: %v", err)
	}
}

// TestAuthedClientPassesThroughOnSoftRefreshFailure: when the token
// endpoint is unreachable (network blip, 5xx), we don't want to lock the
// user out — keep the stale access token and let api.Client surface the
// downstream 401 if it really is dead. Only HARD OAuth errors short-circuit.
func TestAuthedClientPassesThroughOnSoftRefreshFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:      srv.URL,
		AccessToken:  "stale-access",
		RefreshToken: "still-good",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	client, _, err := authedClient(&GlobalFlags{})
	if err != nil {
		t.Fatalf("soft refresh failure should not block authedClient: %v", err)
	}
	if client.Token != "stale-access" {
		t.Errorf("soft failure should keep the stale token; got %q", client.Token)
	}
}

// TestAuthedClientSkipsRefreshForFreshToken verifies we don't burn a token
// endpoint call every time — a comfortably-unexpired token should hit the
// server zero times.
func TestAuthedClientSkipsRefreshForFreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Errorf("should not have hit /oauth/token with a fresh access token")
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:      srv.URL,
		AccessToken:  "fresh-access",
		RefreshToken: "still-valid",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	client, _, err := authedClient(&GlobalFlags{})
	if err != nil {
		t.Fatalf("authedClient: %v", err)
	}
	if client.Token != "fresh-access" {
		t.Errorf("client.Token = %q, want fresh-access", client.Token)
	}
}
