package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLoadReturnsEmptyWhenFileMissing(t *testing.T) {
	withTempConfigDir(t)

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(creds.Profiles) != 0 {
		t.Fatalf("expected empty profiles, got %d", len(creds.Profiles))
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	withTempConfigDir(t)

	p := &Profile{
		APIBase:      DefaultAPIBase,
		AccessToken:  "tok-123",
		RefreshToken: "ref-abc",
		ExpiresAt:    time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second),
		Username:     "scott",
	}
	if err := SetProfile(DefaultProfile, p); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}

	creds, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := creds.GetProfile(DefaultProfile)
	if got == nil {
		t.Fatalf("default profile missing after save/load")
	}
	if got.AccessToken != "tok-123" {
		t.Errorf("access_token = %q, want tok-123", got.AccessToken)
	}
	if got.Username != "scott" {
		t.Errorf("username = %q, want scott", got.Username)
	}
}

func TestSaveWritesModeSixHundred(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks don't apply on Windows")
	}
	withTempConfigDir(t)

	if err := SetProfile(DefaultProfile, &Profile{AccessToken: "x"}); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	path, _ := CredentialsPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %#o, want 0600", perm)
	}
}

func TestLoadRefusesLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks don't apply on Windows")
	}
	dir := withTempConfigDir(t)

	// Write a credentials file world-readable.
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{}}`), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail on loose permissions, got nil error")
	}
}

func TestMultipleProfilesAreIsolated(t *testing.T) {
	withTempConfigDir(t)

	if err := SetProfile("prod", &Profile{AccessToken: "prod-tok"}); err != nil {
		t.Fatal(err)
	}
	if err := SetProfile("staging", &Profile{AccessToken: "staging-tok"}); err != nil {
		t.Fatal(err)
	}

	creds, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if creds.GetProfile("prod").AccessToken != "prod-tok" {
		t.Errorf("prod token wrong")
	}
	if creds.GetProfile("staging").AccessToken != "staging-tok" {
		t.Errorf("staging token wrong")
	}
	if creds.GetProfile("does-not-exist") != nil {
		t.Errorf("unknown profile should return nil")
	}
}

// withTempConfigDir points ALAND_CONFIG_DIR at a fresh temp directory for
// the duration of the test. Returns the path in case the test needs it.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ALAND_CONFIG_DIR", dir)
	return dir
}

// TestDirDefaultsToDotArtifactlandInHome confirms the CLI settles on the
// branded $HOME/.artifactland path rather than os.UserConfigDir()'s
// per-platform default.
func TestDirDefaultsToDotArtifactlandInHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ALAND_CONFIG_DIR", "")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(home, ".artifactland")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}
