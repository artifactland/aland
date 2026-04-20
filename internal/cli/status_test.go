package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/artifactland/aland/internal/project"
)

func TestStatusWithoutDraft(t *testing.T) {
	dir := setupProject(t, "http://unused", "index.html", "<!DOCTYPE html><html></html>", nil)
	runFrom(t, dir, []string{"status"})
}

func TestStatusWithDraftFetchesServerState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/posts/draft-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "draft-1",
				"slug": "my-slug",
				"title": "t",
				"file_type": "html",
				"visibility": "public_visibility",
				"tags": [],
				"urls": { "web": "https://artifact.land/drafts/draft-1" }
			},
			"meta": { "request_id": "r" }
		}`))
	}))
	defer srv.Close()

	dir := setupProject(t, srv.URL, "index.html", "<!DOCTYPE html><html></html>", &project.Artifact{
		PostID:     "draft-1",
		SourceFile: "index.html",
	})

	out := runFrom(t, dir, []string{"--json", "status"})
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("expected JSON output, got: %s", out)
	}
	srvData, ok := report["server"].(map[string]any)
	if !ok {
		t.Fatalf("server data missing from report: %v", report)
	}
	if srvData["state"] != "draft" {
		t.Errorf("state = %v, want draft", srvData["state"])
	}
}
