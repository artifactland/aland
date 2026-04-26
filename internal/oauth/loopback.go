package oauth

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"time"
)

// CallbackResult is what the browser hands back to us: either an OAuth code
// (success) or an error parameter (user denied, bad client config, etc.).
type CallbackResult struct {
	Code  string
	State string
	Err   error
}

// LoopbackServer binds a random port on 127.0.0.1 and serves exactly one
// /callback request before shutting down. The RedirectURI method returns
// the URL to hand to the authorize endpoint.
type LoopbackServer struct {
	listener net.Listener
	server   *http.Server
	result   chan CallbackResult
	state    string
}

// StartLoopback grabs a random port, starts an HTTP server, and returns it
// ready to serve the OAuth callback. The expected `state` value is compared
// against what the authorize endpoint returns, so a tampered redirect fails
// fast instead of exchanging a forged code.
func StartLoopback(expectedState string) (*LoopbackServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("binding loopback port: %w", err)
	}

	s := &LoopbackServer{
		listener: ln,
		result:   make(chan CallbackResult, 1),
		state:    expectedState,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		// Serve returns http.ErrServerClosed on Shutdown — expected, not logged.
		_ = s.server.Serve(ln)
	}()

	return s, nil
}

// RedirectURI is the one to include in the authorize URL. It matches the
// client's registered loopback redirect (per RFC 8252, any port is allowed
// when the registered host is 127.0.0.1).
func (s *LoopbackServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", s.port())
}

func (s *LoopbackServer) port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Await blocks until either the browser returns a callback or ctx is
// canceled. Returns the code (or an error); shuts the server down either
// way so the port gets freed.
func (s *LoopbackServer) Await(ctx context.Context) (string, error) {
	defer s.shutdown()

	select {
	case res := <-s.result:
		if res.Err != nil {
			return "", res.Err
		}
		return res.Code, nil
	case <-ctx.Done():
		return "", fmt.Errorf("timed out waiting for browser to return (%w)", ctx.Err())
	}
}

func (s *LoopbackServer) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}

func (s *LoopbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Some authorization pages (e.g. Rails apps using Turbo) submit the
	// authorize form via fetch rather than a full browser navigation.  Chrome
	// then applies its Private Network Access (PNA) policy and sends an OPTIONS
	// preflight before allowing a cross-origin fetch to a loopback address.
	// Respond with the required headers on every response so both the preflight
	// and the real GET succeed without needing a full-page redirect.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	q := r.URL.Query()

	if e := q.Get("error"); e != "" {
		desc := q.Get("error_description")
		s.respondFailure(w, e, desc)
		s.send(CallbackResult{Err: fmt.Errorf("authorize failed: %s%s", e, optional(": ", desc))})
		return
	}

	gotState := q.Get("state")
	if gotState != s.state {
		s.respondFailure(w, "state_mismatch", "The returned state did not match. Reload your terminal's login command and try again.")
		s.send(CallbackResult{Err: errors.New("state mismatch on OAuth callback — aborted to guard against a tampered redirect")})
		return
	}

	code := q.Get("code")
	if code == "" {
		s.respondFailure(w, "missing_code", "Authorization succeeded but no code was returned.")
		s.send(CallbackResult{Err: errors.New("OAuth callback missing `code` parameter")})
		return
	}

	s.respondSuccess(w)
	s.send(CallbackResult{Code: code, State: gotState})
}

// send is non-blocking because the channel is buffered and we only send once
// per server lifetime. Defensive in case of double-callbacks.
func (s *LoopbackServer) send(r CallbackResult) {
	select {
	case s.result <- r:
	default:
	}
}

func (s *LoopbackServer) respondSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(successHTML))
}

func (s *LoopbackServer) respondFailure(w http.ResponseWriter, code, description string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(w, failureHTML, html.EscapeString(code), html.EscapeString(description))
}

func optional(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

const successHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Signed in — artifact.land</title>
  <style>
    body { font-family: system-ui, -apple-system, sans-serif; display: grid; place-items: center; min-height: 100vh; margin: 0; color: #1b1b1b; background: #fafaf7; }
    .card { text-align: center; padding: 2rem 3rem; border: 1px solid #e5e5df; border-radius: 16px; background: #fff; }
    .check { color: #3aa063; font-size: 2rem; margin-bottom: 0.5rem; }
    h1 { font-size: 1.25rem; margin: 0 0 0.5rem; }
    p { margin: 0; color: #666; font-size: 0.95rem; }
  </style>
</head>
<body>
  <div class="card">
    <div class="check">✓</div>
    <h1>Signed in to artifact.land</h1>
    <p>You can close this tab and return to your terminal.</p>
  </div>
</body>
</html>`

const failureHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Sign-in failed — artifact.land</title>
  <style>
    body { font-family: system-ui, -apple-system, sans-serif; display: grid; place-items: center; min-height: 100vh; margin: 0; color: #1b1b1b; background: #fafaf7; }
    .card { text-align: center; padding: 2rem 3rem; border: 1px solid #e5e5df; border-radius: 16px; background: #fff; max-width: 480px; }
    .x { color: #c04040; font-size: 2rem; margin-bottom: 0.5rem; }
    h1 { font-size: 1.25rem; margin: 0 0 0.5rem; }
    p { margin: 0; color: #666; font-size: 0.95rem; }
    code { font-family: ui-monospace, monospace; background: #f0f0ea; padding: 0.1em 0.3em; border-radius: 4px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="x">✗</div>
    <h1>Sign-in failed</h1>
    <p><code>%s</code></p>
    <p style="margin-top: 0.5rem">%s</p>
  </div>
</body>
</html>`
