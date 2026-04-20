// Package config manages the CLI's on-disk configuration: the config
// directory, the per-profile credentials file, and settings like the
// default API base URL.
//
// Layout under $ALAND_CONFIG_DIR or ~/.artifactland:
//
//	credentials.json       — per-profile access/refresh tokens (chmod 600)
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// DefaultAPIBase is where the CLI talks to the Rails API. Overridable via
// ALAND_API env var or --api-url flag so staging is reachable without a
// rebuild.
const DefaultAPIBase = "https://artifact.land"

// DefaultClientID is the pre-registered OAuth client the CLI uses. Matches
// the seed in db/seeds.rb on the Rails side.
const DefaultClientID = "artifactland-cli"

// DefaultProfile is the credential bucket used when the user doesn't pass
// --profile. Separate buckets let one install talk to prod and staging
// without re-logging-in.
const DefaultProfile = "default"

// Dir returns the absolute path to the CLI's config directory. It is NOT
// created by this function — callers that write into it should call
// EnsureDir() first.
//
// The default is ~/.artifactland on every platform. We deliberately don't
// use os.UserConfigDir() — that gives three different paths across Linux,
// macOS, and Windows, which makes support docs three-versioned for no user
// benefit. CLI convention (`~/.aws`, `~/.docker`, `~/.claude`) is to put
// state under the tool's brand in $HOME, so we do the same.
func Dir() (string, error) {
	if override := os.Getenv("ALAND_CONFIG_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home dir: %w", err)
	}
	return filepath.Join(home, ".artifactland"), nil
}

// EnsureDir creates the config dir with 0700 perms if it doesn't exist.
func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	return dir, nil
}

// CredentialsPath is where all profiles' tokens are stored.
func CredentialsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// Credentials is the serializable shape of the credentials file. Profiles
// are keyed by name; DefaultProfile is always "default".
type Credentials struct {
	Profiles map[string]*Profile `json:"profiles"`
}

// Profile holds one OAuth session. The API URL is stored per-profile so
// `--profile staging` changes both the creds and where we point.
type Profile struct {
	APIBase      string    `json:"api_base"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id,omitempty"`
	Username     string    `json:"username,omitempty"`
}

// Load reads credentials.json. Returns an empty Credentials when the file is
// absent — callers treat "no file" and "no profiles" identically.
//
// Refuses to load a credentials file with loose permissions on unix systems
// so a misconfigured umask doesn't silently leak tokens to other users.
func Load() (*Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Credentials{Profiles: map[string]*Profile{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat credentials: %w", err)
	}

	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return nil, fmt.Errorf("credentials file %s has permissions %#o; expected 600 — run `chmod 600 %s`", path, perm, path)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open credentials: %w", err)
	}
	defer f.Close()

	creds := &Credentials{Profiles: map[string]*Profile{}}
	if err := json.NewDecoder(f).Decode(creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if creds.Profiles == nil {
		creds.Profiles = map[string]*Profile{}
	}
	return creds, nil
}

// Save writes credentials to disk with 0600 perms (atomic via rename). Any
// existing looser-permission file is replaced.
func Save(creds *Credentials) error {
	dir, err := EnsureDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "credentials.json")

	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod credentials: %w", err)
	}

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(creds); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing credentials: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("closing credentials temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("renaming credentials: %w", err)
	}
	return nil
}

// SetProfile inserts or replaces a profile and saves. Convenience wrapper
// around Load/Save for the common single-profile write.
func SetProfile(name string, p *Profile) error {
	creds, err := Load()
	if err != nil {
		return err
	}
	creds.Profiles[name] = p
	return Save(creds)
}

// GetProfile returns the named profile or nil if absent.
func (c *Credentials) GetProfile(name string) *Profile {
	if c == nil || c.Profiles == nil {
		return nil
	}
	return c.Profiles[name]
}
