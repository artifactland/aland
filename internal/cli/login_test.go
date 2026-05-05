package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/artifactland/aland/internal/config"
)

// loginNoOpsWhenTokenIsFresh: aland login on an existing fresh profile
// returns cleanly without rotating tokens — no PKCE flow, no browser, no
// "already signed in" hard error.
func TestLoginNoOpsWhenTokenIsFresh(t *testing.T) {
	// Loopback URL keeps the api-base validator happy; no server needed
	// because the fresh-token branch doesn't make any HTTP call.
	const apiBase = "http://127.0.0.1:1"

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:      apiBase,
		AccessToken:  "still-good",
		RefreshToken: "rt",
		Username:     "scott",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"login"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	creds, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := creds.GetProfile(config.DefaultProfile)
	if got.AccessToken != "still-good" {
		t.Errorf("token rotated unexpectedly: got %q", got.AccessToken)
	}
	if got.RefreshToken != "rt" {
		t.Errorf("refresh token rotated unexpectedly: got %q", got.RefreshToken)
	}
}

// loginRefreshesExpiredToken: when the access token is past expiry but a
// refresh token exists and is valid, login silently refreshes against the
// profile's stored APIBase and persists the new tokens — no browser flow.
func TestLoginRefreshesExpiredToken(t *testing.T) {
	var receivedGrant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedGrant = r.PostForm.Get("grant_type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
			"token_type":    "Bearer"
		}`))
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:      srv.URL,
		AccessToken:  "expired",
		RefreshToken: "rt-valid",
		Username:     "scott",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"login"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if receivedGrant != "refresh_token" {
		t.Errorf("expected refresh_token grant; got %q", receivedGrant)
	}

	creds, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := creds.GetProfile(config.DefaultProfile)
	if got.AccessToken != "new-access" {
		t.Errorf("access token not rotated: got %q", got.AccessToken)
	}
	if got.RefreshToken != "new-refresh" {
		t.Errorf("refresh token not rotated: got %q", got.RefreshToken)
	}
}

// reuseOrRefresh returns errReauthNeeded when the refresh token is dead.
// This is the key recovery path: previously the user was told "already
// signed in" and had to `aland logout` first. Now login falls through to
// PKCE on its own.
func TestReuseOrRefreshSignalsReauthOnDeadRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token revoked"}`))
	}))
	defer srv.Close()

	withTempConfigDir(t)
	existing := &config.Profile{
		APIBase:      srv.URL,
		AccessToken:  "expired",
		RefreshToken: "rt-revoked",
		Username:     "scott",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}
	if err := config.SetProfile(config.DefaultProfile, existing); err != nil {
		t.Fatal(err)
	}

	err := reuseOrRefresh(nil, srv.URL, config.DefaultProfile, existing)
	if err == nil {
		t.Fatal("expected errReauthNeeded; got nil")
	}
	if !errors.Is(err, errReauthNeeded) {
		t.Errorf("expected errReauthNeeded, got: %v", err)
	}
}

// reuseOrRefresh on a fresh token is a no-op (no server contact, no token
// rotation).
func TestReuseOrRefreshOnFreshTokenIsNoOp(t *testing.T) {
	withTempConfigDir(t)
	p := &config.Profile{
		APIBase:      "http://127.0.0.1:1",
		AccessToken:  "good",
		RefreshToken: "rt",
		Username:     "scott",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}
	if err := config.SetProfile(config.DefaultProfile, p); err != nil {
		t.Fatal(err)
	}
	if err := reuseOrRefresh(nil, "http://127.0.0.1:1", config.DefaultProfile, p); err != nil {
		t.Errorf("reuseOrRefresh on fresh token returned %v; want nil", err)
	}

	// And the stored profile is untouched.
	creds, _ := config.Load()
	if got := creds.GetProfile(config.DefaultProfile).AccessToken; got != "good" {
		t.Errorf("access token rotated unexpectedly: got %q", got)
	}
}
