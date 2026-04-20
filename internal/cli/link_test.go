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

func TestLinkBindsFileToPostByRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/scott/posts/deep-field" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "pst_42", "slug": "deep-field", "title": "Deep Field",
				"file_type": "html", "visibility": "public_visibility", "tags": ["space"],
				"user": { "id": "u1", "username": "scott" },
				"urls": { "web": "https://artifact.land/@scott/deep-field", "canonical": "https://artifact.land/@scott/deep-field" }
			},
			"meta": { "request_id": "r" }
		}`))
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase: srv.URL, AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "deep-field.html")
	if err := os.WriteFile(sourcePath, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	runFrom(t, dir, []string{"link", "deep-field.html", "@scott/deep-field"})

	p, err := project.Load(dir)
	if err != nil {
		t.Fatalf("project.Load: %v", err)
	}
	a, err := p.Only()
	if err != nil {
		t.Fatalf("Only: %v", err)
	}
	if a.PostID != "pst_42" {
		t.Errorf("post_id = %q, want pst_42", a.PostID)
	}
	if a.Title != "Deep Field" {
		t.Errorf("title = %q, want Deep Field", a.Title)
	}
	if a.SourceFile != "deep-field.html" {
		t.Errorf("source_file = %q, want deep-field.html", a.SourceFile)
	}
}

func TestLinkAcceptsWebURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/users/scott/posts/deep-field") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "pst_42", "slug": "deep-field", "title": "Deep Field",
				"file_type": "html", "visibility": "public_visibility", "tags": [],
				"user": { "id": "u1", "username": "scott" },
				"urls": { "web": "https://artifact.land/@scott/deep-field" }
			},
			"meta": { "request_id": "r" }
		}`))
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase: srv.URL, AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	runFrom(t, dir, []string{"link", "x.html", "https://artifact.land/@scott/deep-field"})

	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Only(); err != nil {
		t.Errorf("link should have bound exactly one artifact; got: %v", err)
	}
}

func TestLinkDisambiguatesDuplicateNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Both lookups should succeed; the key in .aland.json is what we're
		// testing here, not the server's response.
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "pst_a", "slug": "deep-field", "title": "t",
				"file_type": "html", "visibility": "public_visibility", "tags": [],
				"user": { "id": "u1", "username": "scott" },
				"urls": { "web": "https://artifact.land/@scott/deep-field" }
			},
			"meta": { "request_id": "r" }
		}`))
		_ = r
	}))
	defer srv.Close()

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase: srv.URL, AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	// Two files that both derive the same key ("deep-field") via the
	// containing-dir heuristic for generic index filenames.
	if err := os.MkdirAll(filepath.Join(dir, "deep-field"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deep-field", "index.html"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deep-field.html"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	runFrom(t, dir, []string{"link", "deep-field/index.html", "@scott/deep-field"})
	runFrom(t, dir, []string{"link", "deep-field.html", "@scott/deep-field"})

	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Artifacts) != 2 {
		t.Errorf("expected 2 artifacts after two distinct links, got %d: %v", len(p.Artifacts), p.Artifacts)
	}
	if p.Get("deep-field") == nil {
		t.Errorf("expected original 'deep-field' key retained")
	}
	if p.Get("deep-field-2") == nil {
		t.Errorf("expected suffixed 'deep-field-2' key for second link")
	}
}
