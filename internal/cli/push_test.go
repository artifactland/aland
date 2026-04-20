package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func TestPublishCreatesDraftOnFirstRun(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "source") {
			t.Errorf("body missing source field: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "draft-abc",
				"slug": "my-thing",
				"title": "My Thing",
				"file_type": "html",
				"visibility": "public_visibility",
				"tags": [],
				"urls": { "web": "https://artifact.land/drafts/draft-abc" }
			},
			"meta": { "request_id": "req_Z" }
		}`))
	}))
	defer srv.Close()

	dir := setupProject(t, srv.URL, "index.html", "<!DOCTYPE html><html></html>", nil)

	runFrom(t, dir, []string{"push"})

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/v1/posts" {
		t.Errorf("path = %s, want /api/v1/posts", gotPath)
	}

	// post_id should be written back.
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := p.Only()
	if err != nil {
		t.Fatalf("Only: %v", err)
	}
	if a.PostID != "draft-abc" {
		t.Errorf("post_id = %q, want draft-abc", a.PostID)
	}
}

func TestPublishUpdatesExistingDraft(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "draft-xyz",
				"slug": "whatever",
				"title": "t",
				"file_type": "html",
				"visibility": "public_visibility",
				"tags": [],
				"urls": { "web": "https://artifact.land/drafts/draft-xyz" }
			},
			"meta": { "request_id": "r" }
		}`))
	}))
	defer srv.Close()

	dir := setupProject(t, srv.URL, "index.html", "<!DOCTYPE html><html></html>", &project.Artifact{
		PostID:     "draft-xyz",
		SourceFile: "index.html",
	})

	runFrom(t, dir, []string{"push"})

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/posts/draft-xyz" {
		t.Errorf("path = %s, want /api/v1/posts/draft-xyz", gotPath)
	}
}

func TestPublishFormatsUnknownLibraryError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": "unknown_library",
				"message": "Unknown import(s): framer-motion.",
				"details": { "libraries": ["framer-motion"] }
			},
			"meta": { "request_id": "r" }
		}`))
	}))
	defer srv.Close()

	dir := setupProject(t, srv.URL, "index.jsx", "import { motion } from 'framer-motion';\n", nil)

	err := runFromExpectingError(t, dir, []string{"push"})
	if !strings.Contains(err.Error(), "framer-motion") {
		t.Errorf("error should surface the library name; got: %v", err)
	}
	if !strings.Contains(err.Error(), "supported runtime") {
		t.Errorf("error should explain the situation; got: %v", err)
	}
}

func TestPublishMultiArtifactRequiresSelector(t *testing.T) {
	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase: "http://unused", AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := project.New(dir)
	_ = p.Add("alpha", &project.Artifact{SourceFile: "a.html"})
	_ = p.Add("beta", &project.Artifact{SourceFile: "b.html"})
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}

	err := runFromExpectingError(t, dir, []string{"push"})
	if !strings.Contains(err.Error(), "2 artifacts") {
		t.Errorf("expected multi-artifact guard, got: %v", err)
	}
	if !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Errorf("expected candidate list in error; got: %v", err)
	}
}

func TestPublishSelectsArtifactByName(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "d1", "slug": "beta-slug", "title": "Beta",
				"file_type": "html", "visibility": "public_visibility", "tags": [],
				"urls": { "web": "https://artifact.land/drafts/d1" }
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
	if err := os.WriteFile(filepath.Join(dir, "a.html"), []byte("<html>alpha</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.html"), []byte("<html>beta</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := project.New(dir)
	_ = p.Add("alpha", &project.Artifact{SourceFile: "a.html"})
	_ = p.Add("beta", &project.Artifact{SourceFile: "b.html"})
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}

	runFrom(t, dir, []string{"push", "beta"})

	// Body is JSON with a base64'd source; checking the filename field is
	// enough to confirm the right artifact was picked.
	if !strings.Contains(gotBody, `"filename":"b.html"`) {
		t.Errorf("expected filename b.html in upload; body: %s", gotBody)
	}
	if strings.Contains(gotBody, `"filename":"a.html"`) {
		t.Errorf("alpha filename leaked into upload; body: %s", gotBody)
	}
}

func TestPublishBootstrapsProjectOnFirstRun(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "d1", "slug": "light-cones", "title": "Light Cones",
				"file_type": "html", "visibility": "public_visibility", "tags": [],
				"urls": { "web": "https://artifact.land/drafts/d1" }
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
	source := `<!DOCTYPE html><html><head><title>Light Cones</title></head><body></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	runFrom(t, dir, []string{"push", "index.html"})

	// Project file should have been auto-created with the inferred title.
	p, err := project.Load(dir)
	if err != nil {
		t.Fatalf("project.Load after bootstrap: %v", err)
	}
	a, err := p.Only()
	if err != nil {
		t.Fatalf("Only: %v", err)
	}
	if a.Title != "Light Cones" {
		t.Errorf("title not inferred from <title>: got %q", a.Title)
	}
	if a.PostID != "d1" {
		t.Errorf("post_id not persisted after bootstrap+publish: got %q", a.PostID)
	}

	// Title should have been sent to the server as part of the metadata.
	if !strings.Contains(receivedBody, `"title":"Light Cones"`) {
		t.Errorf("inferred title not included in publish body: %s", receivedBody)
	}
}

func TestPublishWithoutProjectOrArgErrors(t *testing.T) {
	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase: "http://unused", AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	err := runFromExpectingError(t, dir, []string{"push"})
	if !strings.Contains(err.Error(), "no .aland.json") {
		t.Errorf("expected helpful no-project error; got: %v", err)
	}
	if !strings.Contains(err.Error(), "aland init") {
		t.Errorf("error should mention `aland init` as an option; got: %v", err)
	}
}

func TestPublishJSONMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "d1",
				"slug": "s",
				"title": "t",
				"file_type": "html",
				"visibility": "public_visibility",
				"tags": [],
				"urls": { "web": "https://artifact.land/drafts/d1" }
			},
			"meta": { "request_id": "r" }
		}`))
	}))
	defer srv.Close()

	dir := setupProject(t, srv.URL, "index.html", "<!DOCTYPE html><html></html>", nil)

	stdout := runFrom(t, dir, []string{"--json", "push"})

	// In --json mode, the structured payload lands on stdout and starts with
	// a {
	var payload map[string]any
	if err := json.Unmarshal([]byte(firstJSONLine(stdout)), &payload); err != nil {
		t.Fatalf("JSON mode should emit parseable JSON: %v\nbody: %s", err, stdout)
	}
	if payload["post_id"] != "d1" {
		t.Errorf("post_id = %v, want d1", payload["post_id"])
	}
	if payload["created"] != true {
		t.Errorf("created should be true on first publish")
	}
}

// --- helpers --------------------------------------------------------------

// setupProject builds a directory with a .aland.json + source file and
// writes credentials for the default profile pointing at srvURL. The
// overrides argument seeds the single artifact; SourceFile defaults to
// `filename` if not otherwise set.
func setupProject(t *testing.T, srvURL, filename, content string, overrides *project.Artifact) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a := overrides
	if a == nil {
		a = &project.Artifact{SourceFile: filename}
	}
	if a.SourceFile == "" {
		a.SourceFile = filename
	}
	p := project.New(dir)
	if err := p.Add(project.DefaultArtifactName, a); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}

	withTempConfigDir(t)
	if err := config.SetProfile(config.DefaultProfile, &config.Profile{
		APIBase:     srvURL,
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runFrom runs the CLI from `dir` — chdir + restore on cleanup. Returns
// captured stdout.
func runFrom(t *testing.T, dir string, args []string) string {
	t.Helper()
	restore := chdir(t, dir)
	defer restore()

	root := NewRoot("test")
	root.SetArgs(args)

	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr: %s", err, errBuf.String())
	}
	return outBuf.String()
}

func runFromExpectingError(t *testing.T, dir string, args []string) error {
	t.Helper()
	restore := chdir(t, dir)
	defer restore()

	root := NewRoot("test")
	root.SetArgs(args)

	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error; stdout=%q stderr=%q", outBuf.String(), errBuf.String())
	}
	return err
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(orig) }
}

// firstJSONLine returns the first line of s that looks like a JSON object,
// skipping any styled preamble.
func firstJSONLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") {
			return trimmed
		}
	}
	return s
}

// Compile check: ensure the test package has a dependency on fmt so Go
// doesn't complain about unused imports when no helper uses it.
var _ = fmt.Sprintf
