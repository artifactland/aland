package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/artifactland/aland/internal/config"
	"github.com/artifactland/aland/internal/project"
)

func TestPullDownloadsPublishedSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/posts/source"):
			// shouldn't hit this path
			t.Errorf("unexpected path: %s", r.URL.Path)
		case r.URL.Path == "/api/v1/users/scott/posts/thing":
			_, _ = w.Write([]byte(`{
				"data": {
					"id": "pst_1",
					"slug": "thing",
					"title": "Thing",
					"file_type": "html",
					"visibility": "public_visibility",
					"tags": [],
					"published_at": "2026-04-17T10:00:00Z",
					"user": { "id": "u1", "username": "scott" },
					"urls": { "canonical": "https://artifact.land/@scott/thing", "web": "https://artifact.land/@scott/thing" }
				},
				"meta": { "request_id": "r" }
			}`))
		case r.URL.Path == "/api/v1/posts/pst_1/source":
			_, _ = w.Write([]byte(`{
				"data": { "id": "pst_1", "file_type": "html", "source": "<!DOCTYPE html><html>live</html>" },
				"meta": { "request_id": "r" }
			}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:     srv.URL,
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	targetDir := filepath.Join(dir, "thing")

	root := NewRoot("test")
	root.SetArgs([]string{"pull", "@scott/thing", targetDir})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	p, err := project.Load(targetDir)
	if err != nil {
		t.Fatalf("project.Load: %v", err)
	}
	a := p.Get("thing")
	if a == nil {
		t.Fatalf("expected artifact 'thing' in project; got: %+v", p.Artifacts)
	}
	if a.PostID != "pst_1" {
		t.Errorf("post_id = %q, want pst_1", a.PostID)
	}

	body, err := os.ReadFile(filepath.Join(targetDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "live") {
		t.Errorf("expected 'live' in source, got: %s", body)
	}
}

func TestPullRefusesNonEmptyDir(t *testing.T) {
	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase: "http://example.invalid", AccessToken: "x", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"pull", "@scott/thing", dir})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "isn't empty") {
		t.Fatalf("expected empty-dir rejection; got: %v", err)
	}
}
