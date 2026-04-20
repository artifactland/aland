package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/artifactland/aland/internal/config"
)

func TestWhoamiPrintsUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"u1","username":"scott","email":"s@example.com"},"meta":{"request_id":"r"}}`))
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:     srv.URL,
		AccessToken: "tok",
		Username:    "stale",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"whoami"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "@scott") {
		t.Errorf("expected @scott in output, got: %s", out.String())
	}
}

func TestWhoamiNotSignedInIsAnError(t *testing.T) {
	withTempConfigDir(t)

	root := NewRoot("test")
	root.SetArgs([]string{"whoami"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when not signed in, got nil")
	}
	if !strings.Contains(err.Error(), "not signed in") {
		t.Errorf("error should mention not signed in, got: %v", err)
	}
}

// withTempConfigDir is a copy of the helper from config_test; can't import
// across packages in Go, and the cli package needs the same setup.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ALAND_CONFIG_DIR", dir)
	return dir
}
