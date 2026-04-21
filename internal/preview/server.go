// Package preview runs a local HTTP server that serves an HTML artifact
// behind the same CSP + sandbox headers as the production content worker.
// If "works here" holds, "works in prod" also holds.
package preview

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// productionCSP mirrors the Content-Security-Policy set by the Rails
// ContentController (content_controller.rb). Keep this in sync when the
// server-side policy changes.
const productionCSP = "default-src 'self' https://cdnjs.cloudflare.com https://cdn.tailwindcss.com https://unpkg.com; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdnjs.cloudflare.com https://cdn.tailwindcss.com https://unpkg.com; " +
	"style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com https://cdn.tailwindcss.com https://unpkg.com https://fonts.googleapis.com; " +
	"img-src 'self' data: blob:; " +
	"connect-src 'none'; " +
	"font-src 'self' data: https://fonts.gstatic.com; " +
	"frame-src https://www.youtube.com https://www.youtube-nocookie.com https://player.vimeo.com; " +
	"object-src 'none'; " +
	"base-uri 'none'"

// Server is a one-shot local preview server. Start it, hand the URL to the
// user, wait for ctx cancellation (Ctrl-C) to shut down.
type Server struct {
	SourcePath string
	listener   net.Listener
	server     *http.Server
}

// Start binds to a random loopback port and begins serving. Use URL() to
// hand the user the address.
func Start(ctx context.Context, sourcePath string, preferredPort int) (*Server, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", preferredPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil && preferredPort != 0 {
		// Preferred port in use — fall back to a random one rather than
		// failing, so `aland preview` always works even if you have another
		// dev server on 4321.
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return nil, fmt.Errorf("binding preview port: %w", err)
	}

	s := &Server{SourcePath: sourcePath, listener: ln}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)

	s.server = &http.Server{
		Handler:           withSandboxHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() { _ = s.server.Serve(ln) }()

	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(sctx)
	}()

	return s, nil
}

// URL returns http://127.0.0.1:PORT/ for display.
func (s *Server) URL() string {
	return fmt.Sprintf("http://%s/", s.listener.Addr().String())
}

// Wait blocks until the server shuts down (context canceled). Useful for
// CLI commands that want to run in the foreground.
func (s *Server) Wait() {
	// Serve() already returned via the goroutine — we wait for the listener
	// to actually close.
	for {
		if s.listener.Addr() == nil {
			return
		}
		conn, err := net.DialTimeout("tcp", s.listener.Addr().String(), 200*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}

	content, err := os.ReadFile(s.SourcePath)
	if err != nil {
		http.Error(w, "Source file not found: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ext := strings.ToLower(filepath.Ext(s.SourcePath))
	if ext != ".html" && ext != ".htm" {
		http.Error(w, "preview currently supports only HTML files; for JSX, run `aland publish` and use /drafts/:id/preview", http.StatusUnsupportedMediaType)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

// withSandboxHeaders adds every security header the production content
// worker emits so the local preview behaves the same way.
func withSandboxHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", productionCSP)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.ServeHTTP(w, r)
	})
}
