package preview

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerServesHTMLWithSandboxHeaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.html")
	if err := os.WriteFile(path, []byte("<!DOCTYPE html><html><body>hi</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := Start(ctx, path, 0)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond) // serve loop starts in a goroutine

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hi") {
		t.Errorf("body missing source content: %s", body)
	}

	// All prod-parity headers must be present.
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "Permissions-Policy"} {
		if resp.Header.Get(h) == "" {
			t.Errorf("missing header %s", h)
		}
	}

	// connect-src 'none' is the most critical CSP directive; without it, an
	// artifact could phone home. Spot-check it here.
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'none'") {
		t.Errorf("CSP missing `connect-src 'none'`: %s", csp)
	}
	// frame-src allowlist mirrors prod — YouTube and Vimeo embeds must load.
	if !strings.Contains(csp, "frame-src https://www.youtube.com https://www.youtube-nocookie.com https://player.vimeo.com") {
		t.Errorf("CSP missing YouTube/Vimeo frame-src allowlist: %s", csp)
	}
}

func TestServerRejectsJSXWithHelpfulError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.jsx")
	if err := os.WriteFile(path, []byte("function App(){return <div/>;}"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := Start(ctx, path, 0)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "aland publish") {
		t.Errorf("error should suggest aland publish; got: %s", body)
	}
}
